package windowkeeper

import (
	"context"
	"testing"
	"time"
)

type mockStore struct {
	settings Settings
	accounts []Account
	windows  map[string][]State
	attempts []Attempt
	leases   map[string]string
}

func newMockStore() *mockStore {
	return &mockStore{
		settings: DefaultSettings(),
		windows:  make(map[string][]State),
		leases:   make(map[string]string),
	}
}

func (m *mockStore) LoadSettings(ctx context.Context) (Settings, bool, error) {
	return m.settings, true, nil
}
func (m *mockStore) SaveSettings(ctx context.Context, s Settings) error {
	m.settings = s
	return nil
}
func (m *mockStore) TouchAccount(ctx context.Context, a Account) error {
	for i, existing := range m.accounts {
		if existing.AuthID == a.AuthID {
			m.accounts[i] = a
			return nil
		}
	}
	m.accounts = append(m.accounts, a)
	return nil
}
func (m *mockStore) ListAccounts(ctx context.Context) ([]Account, error) {
	return append([]Account(nil), m.accounts...), nil
}
func (m *mockStore) SetPlan(ctx context.Context, authID, plan, group, mismatch string) error {
	for i := range m.accounts {
		if m.accounts[i].AuthID == authID {
			m.accounts[i].PlanType = plan
			m.accounts[i].PlanGroup = group
			m.accounts[i].ShapeMismatch = mismatch
		}
	}
	return nil
}
func (m *mockStore) SetOverride(ctx context.Context, authID, raw string) error {
	for i := range m.accounts {
		if m.accounts[i].AuthID == authID {
			m.accounts[i].OverrideJSON = raw
		}
	}
	return nil
}
func (m *mockStore) SetPause(ctx context.Context, authID, reason string) error {
	for i := range m.accounts {
		if m.accounts[i].AuthID == authID {
			m.accounts[i].PauseReason = reason
		}
	}
	return nil
}
func (m *mockStore) SetNotBefore(ctx context.Context, authID string, when time.Time) error {
	for i := range m.accounts {
		if m.accounts[i].AuthID == authID {
			m.accounts[i].NotBefore = when
		}
	}
	return nil
}
func (m *mockStore) SaveWindows(ctx context.Context, authID string, states []State) error {
	m.windows[authID] = append([]State(nil), states...)
	return nil
}
func (m *mockStore) LoadWindows(ctx context.Context, authID string) ([]State, error) {
	return append([]State(nil), m.windows[authID]...), nil
}
func (m *mockStore) Claim(ctx context.Context, authID, owner string, now, until time.Time) (bool, error) {
	if cur, ok := m.leases[authID]; ok && cur != "" {
		return false, nil
	}
	m.leases[authID] = owner
	return true, nil
}
func (m *mockStore) Release(ctx context.Context, authID, owner string) error {
	delete(m.leases, authID)
	return nil
}
func (m *mockStore) ReleaseAll(ctx context.Context) error {
	m.leases = make(map[string]string)
	return nil
}
func (m *mockStore) RequeueStarted(ctx context.Context, now time.Time) error {
	return nil
}
func (m *mockStore) AttemptCount(ctx context.Context, authID, generation string) (int, error) {
	c := 0
	for _, a := range m.attempts {
		if a.AccountID == authID && a.GenerationKey == generation {
			c++
		}
	}
	return c, nil
}
func (m *mockStore) HasSuccess(ctx context.Context, authID, generation string) (bool, error) {
	for _, a := range m.attempts {
		if a.AccountID == authID && a.GenerationKey == generation && a.Status == "succeeded" {
			return true, nil
		}
	}
	return false, nil
}
func (m *mockStore) StartAttempt(ctx context.Context, attempt Attempt) (int64, error) {
	attempt.ID = int64(len(m.attempts) + 1)
	m.attempts = append(m.attempts, attempt)
	return attempt.ID, nil
}
func (m *mockStore) FinishAttempt(ctx context.Context, id int64, status string, httpStatus int, kind, excerpt, responseID string) error {
	for i := range m.attempts {
		if m.attempts[i].ID == id {
			m.attempts[i].Status = status
			m.attempts[i].HTTPStatus = httpStatus
			m.attempts[i].ErrorKind = kind
			m.attempts[i].Excerpt = excerpt
			m.attempts[i].ResponseID = responseID
			return nil
		}
	}
	return nil
}
func (m *mockStore) ListAttempts(ctx context.Context, limit int) ([]Attempt, error) {
	return append([]Attempt(nil), m.attempts...), nil
}

type mockCatalog struct{ refs []AccountRef }

func (c mockCatalog) List(context.Context) ([]AccountRef, error) { return c.refs, nil }

type mockProber struct {
	snaps []Snapshot
	n     int
}

func (p *mockProber) Probe(context.Context, AccountRef) (Snapshot, error) {
	if p.n >= len(p.snaps) {
		return p.snaps[len(p.snaps)-1], nil
	}
	snap := p.snaps[p.n]
	p.n++
	return snap, nil
}

type mockSender struct{ calls int }

func (s *mockSender) Send(context.Context, AccountRef, Settings) (SendResult, error) {
	s.calls++
	return SendResult{OK: true, Status: 200, Excerpt: "OK", ResponseID: "resp"}, nil
}

func TestKeeperProcessFlow(t *testing.T) {
	ctx := context.Background()
	store := newMockStore()
	store.settings.Enabled = true
	store.settings.PollSeconds = 20
	store.settings.SkewSeconds = 3

	now := time.Date(2026, 9, 23, 8, 0, 0, 0, time.UTC)
	end := now.Add(time.Hour)
	blocked := Window{
		LimitID: "codex", Slot: "primary", Kind: KindFiveHour, PeriodSeconds: 18000,
		UsedPercent: 100, LimitReached: true, EndsAt: end, StartsAt: end.Add(-5 * time.Hour),
	}
	cleared := blocked
	cleared.LimitReached = false
	cleared.UsedPercent = 1
	cleared.StartsAt = end.Add(3 * time.Second)
	cleared.EndsAt = cleared.StartsAt.Add(5 * time.Hour)

	probe := &mockProber{snaps: []Snapshot{
		{PlanType: "plus", Windows: []Window{blocked}},
		{PlanType: "plus", Windows: []Window{cleared}},
		{PlanType: "plus", Windows: []Window{cleared}},
	}}
	send := &mockSender{}
	k := &Keeper{
		Store:   store,
		Catalog: mockCatalog{refs: []AccountRef{{AuthID: "ada", Email: "ada@example", Plan: "plus"}}},
		Prober:  probe,
		Sender:  send,
		Owner:   "test",
		Now:     func() time.Time { return now },
	}

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

	// Repeated call in same cycle does not resend
	if err := k.Process(ctx, end.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if send.calls != 1 {
		t.Fatalf("sent again: %d", send.calls)
	}
}

func TestSleepForSkipsDisabledAndPaused(t *testing.T) {
	ctx := context.Background()
	store := newMockStore()
	store.settings.Enabled = true
	store.settings.PollSeconds = 30

	now := time.Date(2026, 9, 23, 8, 0, 0, 0, time.UTC)
	k := &Keeper{Store: store, Now: func() time.Time { return now }}

	if d := k.sleepFor(ctx); d != 30*time.Second {
		t.Fatalf("empty sleep = %v", d)
	}

	_ = store.TouchAccount(ctx, Account{AuthID: "dis", Disabled: true})
	_ = store.TouchAccount(ctx, Account{AuthID: "pau", PauseReason: "reauth"})

	if d := k.sleepFor(ctx); d < 30*time.Second {
		t.Fatalf("disabled/paused caused busy loop: %v", d)
	}

	_ = store.TouchAccount(ctx, Account{AuthID: "act"})
	if d := k.sleepFor(ctx); d != time.Second {
		t.Fatalf("zero not_before sleep = %v, want 1s", d)
	}
}
