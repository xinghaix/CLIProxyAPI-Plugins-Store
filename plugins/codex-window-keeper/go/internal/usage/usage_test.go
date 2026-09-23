package usage

import (
	"testing"
	"time"
)

func TestParseKeepsBothSlotsAndDerivesStart(t *testing.T) {
	now := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)
	snap, err := Parse([]byte(`{
		"plan_type": "plus",
		"rate_limit": {
			"primary_window": {"used_percent": 25, "limit_window_seconds": 604800, "reset_at": 1788000000},
			"secondary_window": {"used_percent": 10, "limit_window_seconds": 2592000, "reset_at": 1789000000}
		}
	}`), now)
	if err != nil {
		t.Fatal(err)
	}
	if Group(snap.PlanType) != GroupPlusTeam || len(snap.Windows) != 2 {
		t.Fatalf("snap = %+v", snap)
	}
	if snap.Windows[0].Kind != KindWeekly || snap.Windows[0].Slot != "primary" {
		t.Fatalf("primary = %+v", snap.Windows[0])
	}
	if snap.Windows[1].Kind != KindMonthly || snap.Windows[1].TimeSource != SourceDerived {
		t.Fatalf("secondary = %+v", snap.Windows[1])
	}
	if !snap.Windows[0].StartsAt.Equal(snap.Windows[0].EndsAt.Add(-604800 * time.Second)) {
		t.Fatalf("start = %s end = %s", snap.Windows[0].StartsAt, snap.Windows[0].EndsAt)
	}
}

func TestFillGapsIncludesMissingMonthlyButNotOtherModel(t *testing.T) {
	now := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)
	snap, err := Parse([]byte(`{
		"plan_type": "plus",
		"rate_limit": {
			"primary_window": {"used_percent": 1, "limit_window_seconds": 18000, "reset_at": 1788000000},
			"secondary_window": {"used_percent": 2, "limit_window_seconds": 604800, "reset_at": 1788100000}
		},
		"additional_rate_limits": [
			{"limit_name": "codex", "rate_limit": {"primary_window": {"used_percent": 3, "limit_window_seconds": 2592000, "reset_at": 1790000000}}},
			{"limit_name": "gpt-5.3-codex-spark", "rate_limit": {"primary_window": {"used_percent": 9, "limit_window_seconds": 2592000, "reset_at": 1790000000}}}
		],
		"code_review_rate_limit": {"primary_window": {"used_percent": 80, "limit_window_seconds": 18000, "reset_after_seconds": 100}}
	}`), now)
	if err != nil {
		t.Fatal(err)
	}
	snap.Windows = MarkOtherModels(snap.Windows, "gpt-5.4")
	selected := Select(snap.Windows, Options{IncludeAdditional: "fill_gaps"})
	gating := map[string]bool{}
	for _, window := range selected {
		if window.Gating {
			gating[window.LimitID+"/"+window.Kind] = true
		}
	}
	if !gating["codex/five_hour"] || !gating["codex/weekly"] || !gating["additional:codex/monthly"] {
		t.Fatalf("gating = %#v", gating)
	}
	if gating["additional:gpt-5-3-codex-spark/monthly"] || gating["code_review/five_hour"] {
		t.Fatalf("unexpected gating %#v", gating)
	}
	all := Select(snap.Windows, Options{IncludeAdditional: "all", IncludeCodeReview: true})
	none := Select(snap.Windows, Options{IncludeAdditional: "none"})
	if countGating(all) <= countGating(selected) || countGating(none) != 2 {
		t.Fatalf("all %d selected %d none %d", countGating(all), countGating(selected), countGating(none))
	}
	var review Window
	for _, window := range selected {
		if window.LimitID == "code_review" {
			review = window
		}
	}
	if review.TimeSource != SourceProbeRelative || review.EndsAt.IsZero() {
		t.Fatalf("review = %+v", review)
	}
}

func TestExplicitStartAndPlanMismatch(t *testing.T) {
	now := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)
	snap, err := Parse([]byte(`{
		"plan_type": "pro",
		"rate_limit": {
			"primary": {"used_percent": 100, "window_minutes": 300, "reset_at": 1788000000, "start_at": 1787982000},
			"secondary": {"used_percent": 4, "limit_window_seconds": 604800, "reset_at": 1789000000}
		}
	}`), now)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Windows[0].Kind != KindFiveHour || snap.Windows[0].TimeSource != SourceExplicit || snap.Windows[0].PeriodSeconds != 18000 {
		t.Fatalf("window = %+v", snap.Windows[0])
	}
	if Mismatch(Group(snap.PlanType), snap.Windows) == "" {
		t.Fatal("expected pro mismatch for five hour window")
	}
	free, err := Parse([]byte(`{"plan_type":"free","rate_limit":{"primary_window":{"used_percent":1,"limit_window_seconds":2592000,"reset_at":1789000000}}}`), now)
	if err != nil || Mismatch(Group(free.PlanType), free.Windows) != "" {
		t.Fatalf("free = %+v err %v", free, err)
	}
}

func countGating(windows []Window) int {
	count := 0
	for _, window := range windows {
		if window.Gating {
			count++
		}
	}
	return count
}
