package store

import (
	"context"
	"testing"
	"time"
)

func TestEnrichCredentials(t *testing.T) {
	ctx := context.Background()
	st, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	now := time.Now().UnixMilli()
	from := now - 100000
	events := []Event{
		{Hash: "e1", TimestampMS: from + 1000, Provider: "codex", Model: "gpt-5", AuthIndex: "idx-1", InputTokens: 100, OutputTokens: 50, TotalTokens: 150},
		{Hash: "e2", TimestampMS: from + 2000, Provider: "codex", Model: "gpt-5", AuthIndex: "idx-1", InputTokens: 200, OutputTokens: 50, TotalTokens: 250, Failed: true},
	}
	if _, err := st.InsertEvents(ctx, events); err != nil {
		t.Fatal(err)
	}
	if err := st.ReplacePrices(ctx, map[string]Price{"gpt-5": {Prompt: 1, Completion: 2}}); err != nil {
		t.Fatal(err)
	}

	creds := []CredentialIdentity{
		{Key: "row-1", AuthIndex: "idx-1"},
	}
	res, err := st.EnrichCredentials(ctx, creds, 200000)
	if err != nil {
		t.Fatal(err)
	}
	enr, ok := res["row-1"]
	if !ok || enr.History == nil {
		t.Fatalf("missing enrichment: %#v", res)
	}
	if enr.History.Requests != 2 || enr.History.SuccessCalls != 1 || enr.History.FailureCalls != 1 {
		t.Fatalf("unexpected history: %#v", enr.History)
	}
	if len(enr.RecentStatuses) != 8 {
		t.Fatalf("expected 8 status slots, got %d", len(enr.RecentStatuses))
	}
	if enr.RecentStatuses[7] != "fail" || enr.RecentStatuses[6] != "ok" {
		t.Fatalf("unexpected recent statuses: %#v", enr.RecentStatuses)
	}
}
