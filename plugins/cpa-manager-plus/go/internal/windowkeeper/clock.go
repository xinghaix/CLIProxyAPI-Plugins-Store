package windowkeeper

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

func Key(state State) string {
	return state.LimitID + "|" + state.Slot + "|" + strconv.FormatInt(state.PeriodSeconds, 10)
}

func Advance(prev []State, current []Window, now time.Time, skew time.Duration) Result {
	previous := map[string]State{}
	for _, state := range prev {
		previous[Key(state)] = state
	}
	seen := map[string]bool{}
	var states []State
	for _, window := range current {
		state := fromWindow(previous[windowKey(window)], window, now)
		states = append(states, state)
		seen[Key(state)] = true
	}
	for _, state := range prev {
		if seen[Key(state)] || state.Absent {
			continue
		}
		state.Absent = true
		state.Gating = false
		state.Phase = PhaseAbsent
		states = append(states, state)
	}
	result := Result{Action: ActionIdle, States: states}
	var due []State
	gating := 0
	blocked := 0
	baseline := 0
	fixed := 0
	var latest time.Time
	for _, state := range states {
		if !state.Gating || state.Absent {
			continue
		}
		gating++
		switch state.Phase {
		case PhaseBlocked:
			blocked++
			if state.EndsAt.After(latest) {
				latest = state.EndsAt
			}
		case PhaseBaseline:
			baseline++
		case PhaseDue:
			due = append(due, state)
		case PhaseFixed, PhaseInconclusive, PhaseStopped:
			fixed++
		}
	}
	switch {
	case gating == 0:
		result.Action = ActionIdle
	case blocked > 0:
		result.Action = ActionWait
		if !latest.IsZero() {
			result.NotBefore = latest.Add(skew)
		}
	case len(due) > 0:
		result.Action = ActionSend
		result.Generation = Generation(due)
	case fixed == gating:
		result.Action = ActionFixed
	case baseline == gating:
		result.Action = ActionBaseline
	default:
		result.Action = ActionIdle
	}
	return result
}

func Judge(beforeStart, beforeEnd, afterStart, afterEnd, completed time.Time, period, skew time.Duration) string {
	tolerance := skew
	if tolerance < 2*time.Minute {
		tolerance = 2 * time.Minute
	}
	if !beforeEnd.IsZero() && !afterEnd.IsZero() && abs(afterEnd.Sub(beforeEnd)) < time.Second {
		return PhaseFixed
	}
	if !afterStart.IsZero() && abs(afterStart.Sub(completed)) <= tolerance && (period <= 0 || afterEnd.IsZero() || abs(afterEnd.Sub(afterStart.Add(period))) <= tolerance) {
		return PhaseAnchored
	}
	if !afterStart.IsZero() && afterStart.Before(completed.Add(-tolerance)) {
		return PhaseFixed
	}
	return PhaseInconclusive
}

func fromWindow(prev State, window Window, now time.Time) State {
	state := State{
		LimitID: window.LimitID, Slot: window.Slot, Kind: window.Kind, PeriodSeconds: window.PeriodSeconds,
		StartsAt: window.StartsAt, EndsAt: window.EndsAt, TimeSource: window.TimeSource,
		UsedPercent: window.UsedPercent, Gating: window.Gating,
		SeenBlocked: prev.SeenBlocked, Hypothesis: prev.Hypothesis, LastBlockedEnd: prev.LastBlockedEnd,
	}
	blocked := window.LimitReached || window.UsedPercent >= 100
	if !window.EndsAt.IsZero() && !window.EndsAt.After(now) {
		blocked = false
	}
	switch {
	case blocked:
		state.Phase = PhaseBlocked
		state.SeenBlocked = true
		state.LastBlockedEnd = window.EndsAt
	case prev.Hypothesis == PhaseFixed:
		state.Phase = PhaseFixed
		state.Hypothesis = PhaseFixed
	case prev.Phase == PhaseInconclusive || prev.Phase == PhaseStopped:
		state.Phase = prev.Phase
		state.Hypothesis = prev.Hypothesis
	case prev.Phase == PhaseBlocked || prev.Phase == PhaseDue:
		state.Phase = PhaseDue
		state.SeenBlocked = true
		state.Hypothesis = ""
	case prev.Hypothesis == PhaseAnchored || prev.Phase == PhaseAnchored:
		state.Phase = PhaseAnchored
		state.Hypothesis = PhaseAnchored
	case !prev.SeenBlocked:
		state.Phase = PhaseBaseline
	default:
		state.Phase = PhaseClear
	}
	return state
}

func windowKey(window Window) string {
	return window.LimitID + "|" + window.Slot + "|" + strconv.FormatInt(window.PeriodSeconds, 10)
}

func Generation(states []State) string {
	parts := make([]string, 0, len(states))
	for _, state := range states {
		end := state.LastBlockedEnd
		if end.IsZero() {
			end = state.EndsAt
		}
		parts = append(parts, Key(state)+"@"+strconv.FormatInt(end.Unix(), 10))
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func abs(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}
