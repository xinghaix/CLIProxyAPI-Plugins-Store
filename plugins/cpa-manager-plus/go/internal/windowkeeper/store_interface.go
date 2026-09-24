package windowkeeper

import (
	"context"
	"time"
)

type Store interface {
	LoadSettings(ctx context.Context) (Settings, bool, error)
	SaveSettings(ctx context.Context, settings Settings) error
	TouchAccount(ctx context.Context, account Account) error
	ListAccounts(ctx context.Context) ([]Account, error)
	SetPlan(ctx context.Context, authID, plan, group, mismatch string) error
	SetOverride(ctx context.Context, authID, raw string) error
	SetPause(ctx context.Context, authID, reason string) error
	SetNotBefore(ctx context.Context, authID string, when time.Time) error
	SaveWindows(ctx context.Context, authID string, states []State) error
	LoadWindows(ctx context.Context, authID string) ([]State, error)
	Claim(ctx context.Context, authID, owner string, now, until time.Time) (bool, error)
	Release(ctx context.Context, authID, owner string) error
	ReleaseAll(ctx context.Context) error
	RequeueStarted(ctx context.Context, now time.Time) error
	AttemptCount(ctx context.Context, authID, generation string) (int, error)
	HasSuccess(ctx context.Context, authID, generation string) (bool, error)
	StartAttempt(ctx context.Context, attempt Attempt) (int64, error)
	FinishAttempt(ctx context.Context, id int64, finish AttemptFinish) error
	ListAttempts(ctx context.Context, limit int) ([]Attempt, error)
}
