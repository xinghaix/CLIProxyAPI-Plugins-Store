package policy

import "time"

const (
	ActionProbe   = "probe"
	ActionBackoff = "backoff"
	ActionPause   = "pause_account"
	ActionStop    = "stop_window"
	KindQuota     = "quota"
	KindAuth      = "auth"
	KindConfig    = "config"
	KindRetry     = "retry"
)

type Decision struct {
	Action  string
	Consume bool
}

func Delay(attempt int, base, max time.Duration, unit float64) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if base <= 0 {
		base = time.Second
	}
	delay := base
	for i := 1; i < attempt; i++ {
		if delay > max/2 {
			delay = max
			break
		}
		delay *= 2
	}
	if max > 0 && delay > max {
		delay = max
	}
	if unit < 0 {
		unit = 0
	}
	if unit > 1 {
		unit = 1
	}
	delay = time.Duration(float64(delay) * (0.8 + 0.4*unit))
	if max > 0 && delay > max {
		return max
	}
	return delay
}

func Decide(status int, kind string, attempt, maxAttempts int) Decision {
	switch classify(status, kind) {
	case KindAuth, KindConfig:
		return Decision{Action: ActionPause}
	case KindQuota:
		return Decision{Action: ActionProbe}
	default:
		if maxAttempts > 0 && attempt >= maxAttempts {
			return Decision{Action: ActionStop, Consume: true}
		}
		return Decision{Action: ActionBackoff, Consume: true}
	}
}

func classify(status int, kind string) string {
	switch kind {
	case KindAuth, KindConfig, KindQuota, KindRetry:
		return kind
	}
	switch status {
	case 401, 403:
		return KindAuth
	case 429:
		return KindQuota
	case 400:
		return KindConfig
	default:
		return KindRetry
	}
}
