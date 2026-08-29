package store

import (
	"fmt"
	"testing"
)

func TestIsBusy(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{fmt.Errorf("constraint failed"), false},
		{fmt.Errorf("database is locked (517)"), true},
		{fmt.Errorf("SQLITE_BUSY_SNAPSHOT"), true},
		{fmt.Errorf("sqlite_busy"), true},
	}
	for _, tt := range tests {
		if got := IsBusy(tt.err); got != tt.want {
			t.Fatalf("IsBusy(%v) = %v, want %v", tt.err, got, tt.want)
		}
	}
}
