package windowkeeper

import "time"

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
	switch Classify(status, kind) {
	case ErrKindAuth, ErrKindConfig:
		return Decision{Action: PolicyActionPause}
	case ErrKindQuota:
		return Decision{Action: PolicyActionProbe}
	default:
		if maxAttempts > 0 && attempt >= maxAttempts {
			return Decision{Action: PolicyActionStop, Consume: true}
		}
		return Decision{Action: PolicyActionBackoff, Consume: true}
	}
}

func Classify(status int, kind string) string {
	switch kind {
	case ErrKindAuth, ErrKindConfig, ErrKindQuota, ErrKindRetry:
		return kind
	}
	switch status {
	case 401, 403:
		return ErrKindAuth
	case 429:
		return ErrKindQuota
	case 400:
		return ErrKindConfig
	default:
		return ErrKindRetry
	}
}
