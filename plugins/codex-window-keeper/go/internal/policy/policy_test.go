package policy

import (
	"testing"
	"time"
)

func TestRetryDoublesUntilCap(t *testing.T) {
	first := Delay(1, 2*time.Second, 5*time.Minute, 0.5)
	second := Delay(2, 2*time.Second, 5*time.Minute, 0.5)
	if second < first*3/2 || second > first*5/2 {
		t.Fatalf("first %s second %s", first, second)
	}
	capped := Delay(20, 2*time.Second, 5*time.Minute, 1)
	if capped > 5*time.Minute || capped < 4*time.Minute {
		t.Fatalf("capped = %s", capped)
	}
}

func TestQuotaDoesNotConsumeAttempt(t *testing.T) {
	got := Decide(429, "", 1, 5)
	if got.Action != ActionProbe || got.Consume {
		t.Fatalf("%+v", got)
	}
}

func TestSixthAttemptStops(t *testing.T) {
	got := Decide(500, KindRetry, 5, 5)
	if got.Action != ActionStop {
		t.Fatalf("%+v", got)
	}
}
