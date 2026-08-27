package ingest

import (
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
