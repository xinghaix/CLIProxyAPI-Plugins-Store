package keeper

import (
	"context"
	"testing"
	"time"

	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/codex-window-keeper/go/internal/config"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/codex-window-keeper/go/internal/store"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/codex-window-keeper/go/internal/usage"
)

type catalog struct{ refs []AccountRef }

func (c catalog) List(context.Context) ([]AccountRef, error) { return c.refs, nil }

type prober struct {
	snaps []usage.Snapshot
	n     int
}

func (p *prober) Probe(context.Context, AccountRef) (usage.Snapshot, error) {
	if p.n >= len(p.snaps) {
		return p.snaps[len(p.snaps)-1], nil
	}
	snap := p.snaps[p.n]
	p.n++
	return snap, nil
}

type sender struct{ calls int }

func (s *sender) Send(context.Context, AccountRef, config.Settings) (SendResult, error) {
	s.calls++
	return SendResult{OK: true, Status: 200, Excerpt: "OK", ResponseID: "resp"}, nil
}

func TestSendsOnceWhenWindowClearsAndSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	db, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	settings := config.Default()
	settings.Enabled = true
	settings.PollSeconds = 20
	settings.SkewSeconds = 3
	if err := db.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 23, 8, 0, 0, 0, time.UTC)
	end := now.Add(time.Hour)
	blocked := usage.Window{LimitID: "codex", Slot: "primary", Kind: usage.KindFiveHour, PeriodSeconds: 18000, UsedPercent: 100, LimitReached: true, EndsAt: end, StartsAt: end.Add(-5 * time.Hour)}
	cleared := blocked
	cleared.LimitReached = false
	cleared.UsedPercent = 1
	cleared.StartsAt = end.Add(3 * time.Second)
	cleared.EndsAt = cleared.StartsAt.Add(5 * time.Hour)
	probe := &prober{snaps: []usage.Snapshot{{PlanType: "plus", Windows: []usage.Window{blocked}}, {PlanType: "plus", Windows: []usage.Window{cleared}}, {PlanType: "plus", Windows: []usage.Window{cleared}}}}
	send := &sender{}
	k := &Keeper{Store: db, Catalog: catalog{refs: []AccountRef{{AuthID: "ada", Email: "ada@example", Plan: "plus"}}}, Prober: probe, Sender: send, Owner: "test", Now: func() time.Time { return now }}
	if err := k.Process(ctx, now); err != nil {
		t.Fatal(err)
	}
	if send.calls != 0 {
		t.Fatalf("sent while blocked: %d", send.calls)
	}
	if err := k.Process(ctx, end.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	if send.calls != 1 {
		t.Fatalf("calls = %d", send.calls)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	k.Store = db
	if err := k.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	if err := k.Process(ctx, end.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if send.calls != 1 {
		t.Fatalf("restart sent again: %d", send.calls)
	}
}
