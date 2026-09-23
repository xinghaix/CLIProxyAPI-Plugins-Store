package windowkeeper

import (
	"strings"
	"testing"
	"time"
)

func makeWindow(kind string, period int64, end time.Time, blocked bool, gating bool) Window {
	return Window{LimitID: "codex", Slot: kind, Kind: kind, PeriodSeconds: period, EndsAt: end, StartsAt: end.Add(-time.Duration(period) * time.Second), LimitReached: blocked, UsedPercent: map[bool]float64{true: 100, false: 10}[blocked], Gating: gating, TimeSource: SourceDerived}
}

func TestNoSendUntilAWindowHasBeenBlocked(t *testing.T) {
	now := time.Date(2026, 9, 23, 8, 0, 0, 0, time.UTC)
	result := Advance(nil, []Window{makeWindow(KindMonthly, 2592000, now.Add(24*time.Hour), false, true)}, now, 3*time.Second)
	if result.Action != ActionBaseline {
		t.Fatalf("action = %s", result.Action)
	}
}

func TestBlockedWindowWaitsUntilEndPlusSkew(t *testing.T) {
	now := time.Date(2026, 9, 23, 8, 0, 0, 0, time.UTC)
	end := now.Add(2 * time.Hour)
	first := Advance(nil, []Window{makeWindow(KindWeekly, 604800, end, true, true)}, now, 3*time.Second)
	if first.Action != ActionWait || !first.NotBefore.Equal(end.Add(3*time.Second)) {
		t.Fatalf("first = %+v", first)
	}
	second := Advance(first.States, []Window{makeWindow(KindWeekly, 604800, end, false, true)}, end.Add(3*time.Second), 3*time.Second)
	if second.Action != ActionSend || second.Generation == "" {
		t.Fatalf("second = %+v", second)
	}
}

func TestFiveHourRecoveryDoesNotSendWhileMonthlyBlocked(t *testing.T) {
	now := time.Date(2026, 9, 23, 8, 0, 0, 0, time.UTC)
	blocked := Advance(nil, []Window{
		makeWindow(KindFiveHour, 18000, now.Add(time.Hour), true, true),
		makeWindow(KindMonthly, 2592000, now.Add(48*time.Hour), true, true),
	}, now, time.Second)
	open := Advance(blocked.States, []Window{
		makeWindow(KindFiveHour, 18000, now.Add(4*time.Hour), false, true),
		makeWindow(KindMonthly, 2592000, now.Add(48*time.Hour), true, true),
	}, now.Add(time.Hour), time.Second)
	if open.Action != ActionWait {
		t.Fatalf("action = %s", open.Action)
	}
}

func TestOneGenerationForSeveralRecoveredWindows(t *testing.T) {
	now := time.Date(2026, 9, 23, 8, 0, 0, 0, time.UTC)
	blocked := Advance(nil, []Window{
		makeWindow(KindWeekly, 604800, now.Add(time.Hour), true, true),
		makeWindow(KindMonthly, 2592000, now.Add(2*time.Hour), true, true),
	}, now, time.Second)
	open := Advance(blocked.States, []Window{
		makeWindow(KindWeekly, 604800, now.Add(7*24*time.Hour), false, true),
		makeWindow(KindMonthly, 2592000, now.Add(30*24*time.Hour), false, true),
	}, now.Add(3*time.Hour), time.Second)
	if open.Action != ActionSend || !strings.Contains(open.Generation, KindWeekly) || !strings.Contains(open.Generation, KindMonthly) {
		t.Fatalf("generation = %s action %s", open.Generation, open.Action)
	}
	again := Advance(markState(open.States, PhaseAnchored), []Window{
		makeWindow(KindWeekly, 604800, now.Add(7*24*time.Hour), false, true),
		makeWindow(KindMonthly, 2592000, now.Add(30*24*time.Hour), false, true),
	}, now.Add(4*time.Hour), time.Second)
	if again.Action == ActionSend {
		t.Fatalf("sent again: %+v", again)
	}
}

func TestInconclusiveGenerationDoesNotSendTwice(t *testing.T) {
	now := time.Date(2026, 9, 23, 8, 0, 0, 0, time.UTC)
	blockedEnd := now.Add(time.Hour)
	blocked := Advance(nil, []Window{makeWindow(KindWeekly, 604800, blockedEnd, true, true)}, now, time.Second)
	recovered := Advance(blocked.States, []Window{makeWindow(KindWeekly, 604800, now.Add(8*24*time.Hour), false, true)}, blockedEnd.Add(time.Second), time.Second)
	if recovered.Action != ActionSend {
		t.Fatalf("recovered action = %s", recovered.Action)
	}
	for i := range recovered.States {
		if recovered.States[i].Phase == PhaseDue {
			recovered.States[i].Phase = PhaseInconclusive
			recovered.States[i].Hypothesis = PhaseInconclusive
		}
	}
	next := Advance(recovered.States, []Window{makeWindow(KindWeekly, 604800, now.Add(9*24*time.Hour), false, true)}, blockedEnd.Add(2*time.Hour), time.Second)
	if next.Action == ActionSend {
		t.Fatalf("resent after inconclusive result: %+v", next)
	}
}

func TestJudgeAnchoredAndFixed(t *testing.T) {
	completed := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	beforeEnd := completed.Add(-time.Minute)
	afterStart := completed.Add(20 * time.Second)
	afterEnd := afterStart.Add(5 * time.Hour)
	if Judge(beforeEnd.Add(-5*time.Hour), beforeEnd, afterStart, afterEnd, completed, 5*time.Hour, 3*time.Second) != PhaseAnchored {
		t.Fatal("expected anchored")
	}
	if Judge(beforeEnd.Add(-5*time.Hour), beforeEnd, beforeEnd.Add(-5*time.Hour), beforeEnd, completed, 5*time.Hour, 3*time.Second) != PhaseFixed {
		t.Fatal("expected fixed when end does not move")
	}
}

func markState(states []State, hypothesis string) []State {
	for i := range states {
		if states[i].Phase == PhaseDue {
			states[i].Phase = PhaseAnchored
			states[i].Hypothesis = hypothesis
		}
	}
	return states
}
