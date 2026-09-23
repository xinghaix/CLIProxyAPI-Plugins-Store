package windowkeeper

import (
	"testing"
	"time"
)

func TestRetryDoublesUntilCap(t *testing.T) {
	base := 2 * time.Second
	max := 5 * time.Minute
	if d := Delay(1, base, max, 0.5); d != 2*time.Second {
		t.Fatalf("first = %s", d)
	}
	if d := Delay(2, base, max, 0.5); d != 4*time.Second {
		t.Fatalf("second = %s", d)
	}
	if d := Delay(3, base, max, 0.5); d != 8*time.Second {
		t.Fatalf("third = %s", d)
	}
	if d := Delay(20, base, max, 0.5); d > max {
		t.Fatalf("delay exceeded max: %s", d)
	}
}

func TestQuotaDoesNotConsumeAttempt(t *testing.T) {
	decision := Decide(429, "", 1, 5)
	if decision.Action != PolicyActionProbe || decision.Consume {
		t.Fatalf("decision = %+v", decision)
	}
}

func TestSixthAttemptStops(t *testing.T) {
	decision := Decide(500, "", 5, 5)
	if decision.Action != PolicyActionStop {
		t.Fatalf("decision = %+v", decision)
	}
}
