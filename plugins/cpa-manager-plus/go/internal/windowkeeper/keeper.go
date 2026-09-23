package windowkeeper

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type Catalog interface {
	List(context.Context) ([]AccountRef, error)
}

type Prober interface {
	Probe(context.Context, AccountRef) (Snapshot, error)
}

type Sender interface {
	Send(context.Context, AccountRef, Settings) (SendResult, error)
}

type Keeper struct {
	Store   Store
	Catalog Catalog
	Prober  Prober
	Sender  Sender
	Owner   string
	Now     func() time.Time
	Wake    chan struct{}
}

func (k *Keeper) now() time.Time {
	if k.Now != nil {
		return k.Now()
	}
	return time.Now().UTC()
}

func (k *Keeper) Recover(ctx context.Context) error {
	if err := k.Store.ReleaseAll(ctx); err != nil {
		return err
	}
	return k.Store.RequeueStarted(ctx, k.now())
}

func (k *Keeper) Run(ctx context.Context) {
	_ = k.Recover(ctx)
	for {
		if ctx.Err() != nil {
			return
		}
		_ = k.Process(ctx, k.now())
		timer := time.NewTimer(k.sleepFor(ctx))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-k.Wake:
			timer.Stop()
		case <-timer.C:
		}
	}
}

func (k *Keeper) WakeUp() {
	if k.Wake == nil {
		return
	}
	select {
	case k.Wake <- struct{}{}:
	default:
	}
}

func (k *Keeper) sleepFor(ctx context.Context) time.Duration {
	settings, _, _ := k.Store.LoadSettings(ctx)
	poll := time.Duration(settings.PollSeconds) * time.Second
	if poll <= 0 {
		poll = 20 * time.Second
	}
	if !settings.Enabled {
		return poll
	}
	accounts, err := k.Store.ListAccounts(ctx)
	if err != nil {
		return poll
	}
	now := k.now()
	wait := poll
	for _, account := range accounts {
		if account.Disabled || account.Unavailable || account.PauseReason != "" {
			continue
		}
		override := decodeOverride(account.OverrideJSON)
		if override.Enabled == "off" {
			continue
		}
		if account.NotBefore.IsZero() {
			return time.Second
		}
		if delta := account.NotBefore.Sub(now); delta > 0 && delta < wait {
			wait = delta
		}
	}
	if wait < time.Second {
		return time.Second
	}
	return wait
}

func (k *Keeper) Process(ctx context.Context, now time.Time) error {
	settings, ok, err := k.Store.LoadSettings(ctx)
	if err != nil {
		return err
	}
	if !ok {
		settings = DefaultSettings()
	}
	if !settings.Enabled || k.Catalog == nil {
		return nil
	}
	refs, err := k.Catalog.List(ctx)
	if err != nil {
		return err
	}
	for _, ref := range refs {
		if err := k.Store.TouchAccount(ctx, Account{
			AuthID: ref.AuthID, AuthIndex: ref.AuthIndex, AccountID: ref.AccountID,
			Email: ref.Email, Name: ref.Name, Disabled: ref.Disabled, Unavailable: ref.Unavailable,
		}); err != nil {
			return err
		}
	}
	accounts, err := k.Store.ListAccounts(ctx)
	if err != nil {
		return err
	}
	known := map[string]AccountRef{}
	for _, ref := range refs {
		known[ref.AuthID] = ref
	}
	poll := time.Duration(settings.PollSeconds) * time.Second
	for _, account := range accounts {
		ref, ok := known[account.AuthID]
		if !ok {
			continue
		}
		if ref.Disabled || ref.Unavailable {
			_ = k.Store.SetNotBefore(ctx, ref.AuthID, now.Add(poll))
			continue
		}
		if ref.NextRetryAfter.After(now) {
			_ = k.Store.SetNotBefore(ctx, ref.AuthID, ref.NextRetryAfter)
			continue
		}
		override := decodeOverride(account.OverrideJSON)
		if override.Enabled == "off" {
			_ = k.Store.SetNotBefore(ctx, ref.AuthID, now.Add(poll))
			continue
		}
		if account.PauseReason != "" {
			_ = k.Store.SetNotBefore(ctx, ref.AuthID, now.Add(poll))
			continue
		}
		if account.NotBefore.After(now) {
			continue
		}
		if err := k.processOne(ctx, ref, settings, now); err != nil {
			return err
		}
	}
	return nil
}

func (k *Keeper) Activate(ctx context.Context, authID string) error {
	settings, _, err := k.settings(ctx)
	if err != nil {
		return err
	}
	if !settings.Enabled {
		return errors.New("quota window keeper is disabled")
	}
	ref, err := k.ref(ctx, authID)
	if err != nil {
		return err
	}
	_ = k.Store.SetNotBefore(ctx, authID, k.now())
	return k.processOne(ctx, ref, settings, k.now())
}

func (k *Keeper) Probe(ctx context.Context, authID string) error {
	settings, _, err := k.settings(ctx)
	if err != nil {
		return err
	}
	ref, err := k.ref(ctx, authID)
	if err != nil {
		return err
	}
	return k.observe(ctx, ref, settings, k.now())
}

func (k *Keeper) processOne(ctx context.Context, ref AccountRef, settings Settings, now time.Time) error {
	override := decodeOverride(loadOverride(ctx, k, ref.AuthID))
	if !settings.Enabled || override.Enabled == "off" || ref.Disabled || ref.Unavailable || ref.NextRetryAfter.After(now) {
		return nil
	}
	effective := settings
	if override.Model != "" {
		effective.Model = override.Model
	}
	until := now.Add(time.Duration(settings.RequestTimeoutSeconds+30) * time.Second)
	claimed, err := k.Store.Claim(ctx, ref.AuthID, k.Owner, now, until)
	if err != nil || !claimed {
		return err
	}
	defer k.Store.Release(ctx, ref.AuthID, k.Owner)
	accounts, _ := k.Store.ListAccounts(ctx)
	for _, account := range accounts {
		if account.AuthID == ref.AuthID && account.PauseReason != "" {
			return nil
		}
	}
	if err := k.observe(ctx, ref, effective, now); err != nil {
		next := now.Add(time.Duration(settings.PollSeconds) * time.Second)
		var statusErr interface{ StatusCode() int }
		if errors.As(err, &statusErr) && statusErr.StatusCode() >= 500 {
			next = now.Add(time.Duration(settings.RetryBaseSeconds) * time.Second)
		}
		_ = k.Store.SetNotBefore(ctx, ref.AuthID, next)
		return nil
	}
	states, err := k.Store.LoadWindows(ctx, ref.AuthID)
	if err != nil {
		return err
	}
	var current []Window
	for _, state := range states {
		if state.Absent {
			continue
		}
		current = append(current, Window{
			LimitID: state.LimitID, Slot: state.Slot, Kind: state.Kind, PeriodSeconds: state.PeriodSeconds,
			UsedPercent: state.UsedPercent, LimitReached: state.Phase == PhaseBlocked, StartsAt: state.StartsAt,
			EndsAt: state.EndsAt, TimeSource: state.TimeSource, Gating: state.Gating,
		})
	}
	result := Advance(states, current, now, time.Duration(settings.SkewSeconds)*time.Second)
	if err := k.Store.SaveWindows(ctx, ref.AuthID, result.States); err != nil {
		return err
	}
	poll := time.Duration(settings.PollSeconds) * time.Second
	next := now.Add(poll)
	if result.Action == ActionWait && !result.NotBefore.IsZero() {
		next = result.NotBefore
	}
	if result.Action != ActionSend {
		return k.Store.SetNotBefore(ctx, ref.AuthID, next)
	}
	done, err := k.Store.HasSuccess(ctx, ref.AuthID, result.Generation)
	if err != nil || done {
		return k.Store.SetNotBefore(ctx, ref.AuthID, next)
	}
	priorAttempts, err := k.Store.AttemptCount(ctx, ref.AuthID, result.Generation)
	if err != nil {
		return err
	}
	attemptNo := priorAttempts + 1
	if attemptNo > settings.MaxAttempts {
		stopGeneration(result.States)
		if err := k.Store.SaveWindows(ctx, ref.AuthID, result.States); err != nil {
			return err
		}
		return k.Store.SetNotBefore(ctx, ref.AuthID, next)
	}
	id, err := k.Store.StartAttempt(ctx, Attempt{
		AccountID: ref.AuthID, GenerationKey: result.Generation, StartedAt: now, AttemptNo: attemptNo,
	})
	if err != nil {
		return err
	}
	sent, err := k.Sender.Send(ctx, ref, effective)
	if err != nil || !sent.OK {
		kind := sent.Kind
		if err != nil {
			kind = ErrKindRetry
		}
		decision := Decide(sent.Status, kind, attemptNo, settings.MaxAttempts)
		_ = k.Store.FinishAttempt(ctx, id, "failed", sent.Status, kind, trim(sent.Excerpt), sent.ResponseID)
		if decision.Action == PolicyActionPause {
			reason := "reauth"
			if kind == ErrKindConfig {
				reason = "config"
			}
			_ = k.Store.SetPause(ctx, ref.AuthID, reason)
			return k.Store.SetNotBefore(ctx, ref.AuthID, now.Add(poll))
		}
		if decision.Action == PolicyActionProbe {
			return k.Store.SetNotBefore(ctx, ref.AuthID, now.Add(poll))
		}
		if decision.Action == PolicyActionStop {
			stopGeneration(result.States)
			if saveErr := k.Store.SaveWindows(ctx, ref.AuthID, result.States); saveErr != nil {
				return saveErr
			}
			return k.Store.SetNotBefore(ctx, ref.AuthID, next)
		}
		jitter := float64(now.UnixNano()%1000) / 1000
		return k.Store.SetNotBefore(ctx, ref.AuthID, now.Add(Delay(attemptNo, time.Duration(settings.RetryBaseSeconds)*time.Second, time.Duration(settings.RetryMaxSeconds)*time.Second, jitter)))
	}
	before := map[string]State{}
	for _, state := range result.States {
		before[Key(state)] = state
	}
	observeErr := k.observe(ctx, ref, effective, now)
	after, loadErr := k.Store.LoadWindows(ctx, ref.AuthID)
	if loadErr != nil {
		return loadErr
	}
	for i := range after {
		prev, ok := before[Key(after[i])]
		if !ok || prev.Phase != PhaseDue {
			continue
		}
		if observeErr != nil {
			after[i].Hypothesis = PhaseInconclusive
			after[i].Phase = PhaseInconclusive
			continue
		}
		after[i].Hypothesis = Judge(prev.StartsAt, prev.EndsAt, after[i].StartsAt, after[i].EndsAt, now, time.Duration(after[i].PeriodSeconds)*time.Second, time.Duration(settings.SkewSeconds)*time.Second)
		switch after[i].Hypothesis {
		case PhaseAnchored:
			after[i].Phase = PhaseAnchored
		case PhaseFixed:
			after[i].Phase = PhaseFixed
		default:
			after[i].Phase = PhaseInconclusive
		}
	}
	if err := k.Store.SaveWindows(ctx, ref.AuthID, after); err != nil {
		return err
	}
	if err := k.Store.FinishAttempt(ctx, id, "succeeded", sent.Status, "", trim(sent.Excerpt), sent.ResponseID); err != nil {
		return err
	}
	return k.Store.SetNotBefore(ctx, ref.AuthID, now.Add(poll))
}

func stopGeneration(states []State) {
	for i := range states {
		if states[i].Phase == PhaseDue {
			states[i].Phase = PhaseStopped
			states[i].Hypothesis = "retry_exhausted"
		}
	}
}

func (k *Keeper) observe(ctx context.Context, ref AccountRef, settings Settings, now time.Time) error {
	snap, err := k.Prober.Probe(ctx, ref)
	if err != nil {
		return err
	}
	windows := MarkOtherModels(snap.Windows, settings.Model)
	windows = Select(windows, Options{
		Kinds: settings.Kinds, IncludeCodeReview: settings.IncludeCodeReview,
		IncludeAdditional: settings.IncludeAdditional, Model: settings.Model,
	})
	prev, err := k.Store.LoadWindows(ctx, ref.AuthID)
	if err != nil {
		return err
	}
	advanced := Advance(prev, windows, now, time.Duration(settings.SkewSeconds)*time.Second)
	plan := snap.PlanType
	if plan == "" {
		plan = ref.Plan
	}
	group := Group(plan)
	if err := k.Store.SetPlan(ctx, ref.AuthID, plan, group, Mismatch(group, windows)); err != nil {
		return err
	}
	return k.Store.SaveWindows(ctx, ref.AuthID, advanced.States)
}

func (k *Keeper) settings(ctx context.Context) (Settings, bool, error) {
	settings, ok, err := k.Store.LoadSettings(ctx)
	if err != nil {
		return Settings{}, false, err
	}
	if !ok {
		settings = DefaultSettings()
	}
	return settings, ok, nil
}

func (k *Keeper) ref(ctx context.Context, authID string) (AccountRef, error) {
	refs, err := k.Catalog.List(ctx)
	if err != nil {
		return AccountRef{}, err
	}
	for _, ref := range refs {
		if ref.AuthID == authID {
			return ref, nil
		}
	}
	return AccountRef{}, errNotFound
}

var errNotFound = errString("account not found")

type errString string

func (e errString) Error() string { return string(e) }

type accountOverride struct {
	Enabled string `json:"enabled"`
	Model   string `json:"model"`
}

func decodeOverride(raw string) accountOverride {
	var override accountOverride
	_ = json.Unmarshal([]byte(raw), &override)
	return override
}

func loadOverride(ctx context.Context, k *Keeper, authID string) string {
	accounts, err := k.Store.ListAccounts(ctx)
	if err != nil {
		return ""
	}
	for _, account := range accounts {
		if account.AuthID == authID {
			return account.OverrideJSON
		}
	}
	return ""
}

func trim(text string) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) > 120 {
		return string(runes[:120])
	}
	return string(runes)
}
