package windowkeeper

import (
	"context"
	"testing"
	"time"
)

func TestExcerptFromUpstreamDetailJSON(t *testing.T) {
	got := ExcerptFromUpstream(`{"detail":"Unsupported parameter: reasoning_effort"}`)
	if got != "Unsupported parameter: reasoning_effort" {
		t.Fatalf("got %q", got)
	}
}

type statusError struct {
	code int
	msg  string
}

func (e statusError) Error() string   { return e.msg }
func (e statusError) StatusCode() int { return e.code }

type failingSender struct {
	result SendResult
	err    error
}

func (s failingSender) Send(context.Context, AccountRef, Settings) (SendResult, error) {
	return s.result, s.err
}

func TestFinishAttemptStoresExcerptFromSendError(t *testing.T) {
	ctx := context.Background()
	store := newMockStore()
	store.settings.Enabled = true
	store.settings.PollSeconds = 20
	store.settings.SkewSeconds = 3
	store.settings.MaxAttempts = 3

	now := time.Date(2026, 9, 23, 8, 0, 0, 0, time.UTC)
	end := now.Add(time.Hour)
	cleared := Window{
		LimitID: "codex", Slot: "primary", Kind: KindFiveHour, PeriodSeconds: 18000,
		UsedPercent: 0, LimitReached: false, EndsAt: end.Add(5 * time.Hour), StartsAt: end, Gating: true,
	}
	_ = store.TouchAccount(ctx, Account{AuthID: "ada", Email: "ada@example", NotBefore: now})
	_ = store.SaveWindows(ctx, "ada", []State{{
		LimitID: "codex", Slot: "primary", Kind: KindFiveHour, PeriodSeconds: 18000,
		Phase: PhaseDue, Gating: true, SeenBlocked: true,
	}})

	probe := &mockProber{snaps: []Snapshot{{PlanType: "plus", Windows: []Window{cleared}}}}
	detailJSON := `{"detail":"Unsupported parameter: reasoning_effort"}`
	sender := failingSender{
		// Simulate old Send path: status filled, excerpt empty, kind wrongly retry, err carries body.
		result: SendResult{Status: 400, Kind: ErrKindRetry, Excerpt: ""},
		err:    statusError{code: 400, msg: detailJSON},
	}
	k := &Keeper{
		Store:   store,
		Catalog: mockCatalog{refs: []AccountRef{{AuthID: "ada", Email: "ada@example", Plan: "plus"}}},
		Prober:  probe,
		Sender:  sender,
		Owner:   "test",
		Now:     func() time.Time { return now },
	}

	if err := k.processOne(ctx, AccountRef{AuthID: "ada", Email: "ada@example", Plan: "plus"}, store.settings, now); err != nil {
		t.Fatal(err)
	}
	if len(store.attempts) != 1 {
		t.Fatalf("attempts=%d", len(store.attempts))
	}
	a := store.attempts[0]
	if a.Status != "failed" {
		t.Fatalf("status=%q", a.Status)
	}
	if a.HTTPStatus != 400 {
		t.Fatalf("http_status=%d", a.HTTPStatus)
	}
	if a.ErrorKind != ErrKindConfig {
		t.Fatalf("error_kind=%q want config", a.ErrorKind)
	}
	// Excerpt falls back to err.Error() then trim; raw JSON is acceptable here.
	if a.Excerpt != detailJSON && a.Excerpt != "Unsupported parameter: reasoning_effort" {
		t.Fatalf("excerpt=%q", a.Excerpt)
	}
}

func TestFinishAttemptUsesSendExcerptWhenPresent(t *testing.T) {
	ctx := context.Background()
	store := newMockStore()
	store.settings.Enabled = true
	store.settings.PollSeconds = 20
	store.settings.SkewSeconds = 3
	store.settings.MaxAttempts = 3

	now := time.Date(2026, 9, 23, 8, 0, 0, 0, time.UTC)
	end := now.Add(time.Hour)
	cleared := Window{
		LimitID: "codex", Slot: "primary", Kind: KindFiveHour, PeriodSeconds: 18000,
		UsedPercent: 0, LimitReached: false, EndsAt: end.Add(5 * time.Hour), StartsAt: end, Gating: true,
	}
	_ = store.TouchAccount(ctx, Account{AuthID: "ada", Email: "ada@example", NotBefore: now})
	_ = store.SaveWindows(ctx, "ada", []State{{
		LimitID: "codex", Slot: "primary", Kind: KindFiveHour, PeriodSeconds: 18000,
		Phase: PhaseDue, Gating: true, SeenBlocked: true,
	}})

	probe := &mockProber{snaps: []Snapshot{{PlanType: "plus", Windows: []Window{cleared}}}}
	sender := failingSender{
		result: SendResult{
			Status:  400,
			Kind:    ErrKindConfig,
			Excerpt: "Unsupported parameter: reasoning_effort",
		},
		err: statusError{code: 400, msg: `{"detail":"Unsupported parameter: reasoning_effort"}`},
	}
	k := &Keeper{
		Store:   store,
		Catalog: mockCatalog{refs: []AccountRef{{AuthID: "ada", Email: "ada@example", Plan: "plus"}}},
		Prober:  probe,
		Sender:  sender,
		Owner:   "test",
		Now:     func() time.Time { return now },
	}

	if err := k.processOne(ctx, AccountRef{AuthID: "ada", Email: "ada@example", Plan: "plus"}, store.settings, now); err != nil {
		t.Fatal(err)
	}
	a := store.attempts[0]
	if a.Excerpt != "Unsupported parameter: reasoning_effort" {
		t.Fatalf("excerpt=%q", a.Excerpt)
	}
	if a.ErrorKind != ErrKindConfig {
		t.Fatalf("error_kind=%q", a.ErrorKind)
	}
}
