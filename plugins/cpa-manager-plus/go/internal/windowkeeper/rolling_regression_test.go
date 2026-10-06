package windowkeeper

import (
	"context"
	"testing"
	"time"
)

func TestKeeperRollingStartsNextGeneration(t *testing.T) {
	ctx := context.Background()
	for _, mode := range []string{"always", "rolling"} {
		t.Run(mode, func(t *testing.T) {
			db := newMockStore()
			db.settings.Enabled = true
			db.settings.WindowMode = mode
			now := time.Date(2026, 9, 23, 8, 0, 0, 0, time.UTC)
			end := now.Add(time.Hour)
			old := Window{LimitID: "codex", Slot: "primary", Kind: KindFiveHour, PeriodSeconds: 18000, UsedPercent: 100, LimitReached: true, StartsAt: end.Add(-5 * time.Hour), EndsAt: end}
			anchored := old
			anchored.LimitReached = false
			anchored.UsedPercent = 1
			anchored.StartsAt = end.Add(3 * time.Second)
			anchored.EndsAt = anchored.StartsAt.Add(5 * time.Hour)
			prober := &mockProber{snaps: []Snapshot{{PlanType: "plus", Windows: []Window{old}}, {PlanType: "plus", Windows: []Window{old}}, {PlanType: "plus", Windows: []Window{anchored}}, {PlanType: "plus", Windows: []Window{anchored}}}}
			sender := &mockSender{}
			k := &Keeper{Store: db, Catalog: mockCatalog{refs: []AccountRef{{AuthID: "audit-account", Plan: "plus"}}}, Prober: prober, Sender: sender, Owner: "audit", Now: func() time.Time { return now }}
			if err := k.Process(ctx, now); err != nil {
				t.Fatal(err)
			}
			now = end.Add(3 * time.Second)
			if err := k.Process(ctx, now); err != nil {
				t.Fatal(err)
			}
			if sender.calls != 1 || len(db.attempts) != 1 || db.attempts[0].Status != "succeeded" {
				t.Fatalf("first send failed calls=%d attempts=%#v", sender.calls, db.attempts)
			}
			firstGeneration := db.attempts[0].GenerationKey
			first := db.windows["audit-account"][0]
			if first.Phase != PhaseAnchored || !first.StartsAt.Equal(anchored.StartsAt) || !first.EndsAt.Equal(anchored.EndsAt) || !first.LastBlockedEnd.Equal(end) {
				t.Fatalf("lost first anchor evidence: %#v", first)
			}
			now = anchored.EndsAt.Add(3 * time.Second)
			if err := k.Process(ctx, now); err != nil {
				t.Fatal(err)
			}
			states := db.windows["audit-account"]
			if sender.calls != 2 || len(db.attempts) != 2 || db.attempts[1].Status != "succeeded" || db.attempts[1].GenerationKey == firstGeneration {
				t.Fatalf("next generation suppressed: states=%#v calls=%d attempts=%#v", states, sender.calls, db.attempts)
			}
			if !states[0].StartsAt.Equal(anchored.StartsAt) || !states[0].EndsAt.Equal(anchored.EndsAt) {
				t.Fatalf("invented local reset: %#v", states[0])
			}
			// The second post-probe is stale: hours passing must not manufacture
			// another quota window, even after a keeper restart or manual probe.
			for _, elapsed := range []time.Duration{time.Minute, 5 * time.Hour, 24 * time.Hour} {
				now = anchored.EndsAt.Add(elapsed)
				restarted := *k
				if err := restarted.Recover(ctx); err != nil {
					t.Fatal(err)
				}
				if err := restarted.Probe(ctx, "audit-account"); err != nil {
					t.Fatal(err)
				}
				if err := restarted.Process(ctx, now); err != nil {
					t.Fatal(err)
				}
			}
			if sender.calls != 2 || len(db.attempts) != 2 {
				t.Fatalf("resent stale window: calls=%d attempts=%#v", sender.calls, db.attempts)
			}
		})
	}
}

func TestRollingGenerationBoundaries(t *testing.T) {
	end := time.Date(2026, 9, 23, 14, 0, 0, 0, time.UTC)
	oldEnd := end.Add(-5 * time.Hour)
	w := makeWindow(KindFiveHour, 18000, end, false, true)
	for _, mode := range []string{"always", "rolling"} {
		t.Run(mode, func(t *testing.T) {
			for _, phase := range []string{PhaseAnchored, PhaseDue} {
				t.Run(phase, func(t *testing.T) {
					// PhaseDue also covers state persisted by the old, stuck scheduler.
					prev := State{LimitID: w.LimitID, Slot: w.Slot, Kind: w.Kind, PeriodSeconds: w.PeriodSeconds,
						StartsAt: w.StartsAt, EndsAt: w.EndsAt, LastBlockedEnd: oldEnd, Gating: true,
						SeenBlocked: true, Phase: phase}
					result := AdvanceWithMode([]State{prev}, []Window{w}, end, 0, mode)
					if result.Action != ActionSend || !result.States[0].LastBlockedEnd.Equal(end) || result.Generation == Generation([]State{prev}) {
						t.Fatalf("did not advance expired generation: %+v", result)
					}
					again := AdvanceWithMode(result.States, []Window{w}, end.Add(time.Hour), 0, mode)
					if again.Generation != result.Generation {
						t.Fatalf("stale poll re-keyed generation: %q -> %q", result.Generation, again.Generation)
					}
					// A retry/post-send probe can already show the NEXT active window.
					next := w
					next.StartsAt = end
					next.EndsAt = end.Add(5 * time.Hour)
					observed := AdvanceWithMode(result.States, []Window{next}, end.Add(time.Second), 0, mode)
					if observed.Generation != result.Generation || !observed.States[0].StartsAt.Equal(next.StartsAt) || !observed.States[0].EndsAt.Equal(next.EndsAt) {
						t.Fatalf("active probe changed pending identity/evidence: %+v", observed)
					}
				})
			}
			prev := State{LimitID: w.LimitID, Slot: w.Slot, Kind: w.Kind, PeriodSeconds: w.PeriodSeconds,
				StartsAt: w.StartsAt, EndsAt: end, LastBlockedEnd: oldEnd, Gating: true, SeenBlocked: true,
				Phase: PhaseAnchored, Hypothesis: PhaseAnchored}
			before := AdvanceWithMode([]State{prev}, []Window{w}, end.Add(-time.Nanosecond), 0, mode)
			if before.Action == ActionSend || !before.States[0].LastBlockedEnd.Equal(oldEnd) {
				t.Fatalf("sent before expiry: %+v", before)
			}
			for _, phase := range []string{PhaseInconclusive, PhaseStopped} {
				prev.Phase = phase
				prev.Hypothesis = phase
				stopped := AdvanceWithMode([]State{prev}, []Window{w}, end.Add(time.Hour), 0, mode)
				if stopped.Action == ActionSend || !stopped.States[0].LastBlockedEnd.Equal(oldEnd) {
					t.Fatalf("revived %s without new block: %+v", phase, stopped)
				}
			}
			unused := w
			unused.UsedPercent = 0
			if result := AdvanceWithMode(nil, []Window{unused}, end, 0, mode); result.Action == ActionSend {
				t.Fatalf("sent for unused window: %+v", result)
			}
			nonGating := w
			nonGating.Gating = false
			if result := AdvanceWithMode(nil, []Window{nonGating}, end, 0, mode); result.Action == ActionSend {
				t.Fatalf("sent for unselected window: %+v", result)
			}
			unknown := w
			unknown.EndsAt = time.Time{}
			if result := AdvanceWithMode(nil, []Window{unknown}, end, 0, mode); result.Action == ActionSend {
				t.Fatalf("sent with missing reset: %+v", result)
			}
			// Other selected quota windows remain authoritative blockers.
			weekly := makeWindow(KindWeekly, 604800, end.Add(time.Hour), true, true)
			blocked := AdvanceWithMode(nil, []Window{w, weekly}, end, 3*time.Second, mode)
			if blocked.Action != ActionWait || !blocked.NotBefore.Equal(weekly.EndsAt.Add(3*time.Second)) {
				t.Fatalf("bypassed blocked quota: %+v", blocked)
			}
		})
	}
	for _, mode := range []string{"auto", "blocked_only"} {
		result := AdvanceWithMode(nil, []Window{w}, end, 0, mode)
		if result.Action == ActionSend {
			t.Fatalf("%s gained automatic rolling: %+v", mode, result)
		}
	}
}

func TestKeeperRollingSeveralAnchoredGenerations(t *testing.T) {
	for _, mode := range []string{"always", "rolling"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			db := newMockStore()
			db.settings.Enabled, db.settings.WindowMode = true, mode
			now := time.Date(2026, 9, 23, 8, 0, 0, 0, time.UTC)
			windows := []Window{makeWindow(KindFiveHour, 18000, now.Add(time.Hour), true, true)}
			for i := 0; i < 3; i++ {
				windows = append(windows, makeWindow(KindFiveHour, 18000, windows[i].EndsAt.Add(3*time.Second+5*time.Hour), false, true))
			}
			sender := &mockSender{}
			k := &Keeper{Store: db, Catalog: mockCatalog{refs: []AccountRef{{AuthID: "rolling", Plan: "plus"}}},
				Prober: &mockFnProber{fn: func(AccountRef) Snapshot { return Snapshot{Windows: []Window{windows[sender.calls]}} }},
				Sender: sender, Owner: "test", Now: func() time.Time { return now }}
			if err := k.Process(ctx, now); err != nil {
				t.Fatal(err)
			}
			generations := map[string]bool{}
			for i := 1; i <= 3; i++ {
				now = windows[i-1].EndsAt.Add(3 * time.Second)
				if err := k.Process(ctx, now); err != nil {
					t.Fatal(err)
				}
				state := db.windows["rolling"][0]
				if sender.calls != i || len(db.attempts) != i || state.Phase != PhaseAnchored || !state.EndsAt.Equal(windows[i].EndsAt) {
					t.Fatalf("cycle %d: calls=%d states=%#v attempts=%#v", i, sender.calls, state, db.attempts)
				}
				key := db.attempts[i-1].GenerationKey
				if generations[key] || db.attempts[i-1].AttemptNo != 1 {
					t.Fatalf("reused generation/budget: %#v", db.attempts)
				}
				generations[key] = true
				now = now.Add(time.Minute)
				if err := k.Process(ctx, now); err != nil {
					t.Fatal(err)
				}
				if sender.calls != i {
					t.Fatalf("sent twice in cycle %d", i)
				}
			}
		})
	}
}
