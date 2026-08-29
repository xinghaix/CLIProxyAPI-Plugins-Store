package ingest

import (
	"fmt"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func TestToEventCopiesMappedModelAndAlias(t *testing.T) {
	event := ToEvent(pluginapi.UsageRecord{
		Model:       "gpt-5",
		Alias:       "g5",
		RequestedAt: time.UnixMilli(1_700_000_000_000),
		Detail:      pluginapi.UsageDetail{TotalTokens: 8},
	})
	if event.Model != "gpt-5" {
		t.Fatalf("model = %q, want gpt-5", event.Model)
	}
	if event.Alias != "g5" {
		t.Fatalf("alias = %q, want g5", event.Alias)
	}
}

func TestToEventTrimsAliasAndLeavesItEmptyWhenMissing(t *testing.T) {
	event := ToEvent(pluginapi.UsageRecord{Model: " gpt-5 ", Alias: "  "})
	if event.Model != "gpt-5" {
		t.Fatalf("model = %q, want gpt-5", event.Model)
	}
	if event.Alias != "" {
		t.Fatalf("alias = %q, want empty", event.Alias)
	}
}

func TestKeepUnflushedBatchOnBusy(t *testing.T) {
	if !keepUnflushedBatch(fmt.Errorf("database is locked (517)")) {
		t.Fatal("busy insert must keep the batch")
	}
	if keepUnflushedBatch(fmt.Errorf("constraint failed")) {
		t.Fatal("non-busy insert must drop the batch")
	}
	if keepUnflushedBatch(nil) {
		t.Fatal("success must not keep the batch")
	}
}

func TestToEventRewritesEpochTimestamp(t *testing.T) {
	before := time.Now().Add(-time.Second).UnixMilli()
	event := ToEvent(pluginapi.UsageRecord{Model: "gpt-5", RequestedAt: time.Unix(0, 0)})
	if event.TimestampMS <= 0 || event.TimestampMS < before {
		t.Fatalf("timestamp = %d, want current time", event.TimestampMS)
	}
}
