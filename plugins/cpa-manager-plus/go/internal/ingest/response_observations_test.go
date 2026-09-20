package ingest

import (
	"net/http"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/responsemodel"
)

func TestToEventResponseCorrelationUsesSharedKeyWithoutChangingIdentity(t *testing.T) {
	headers := http.Header{"X-Request-Id": []string{"request-123"}}
	record := pluginapi.UsageRecord{AuthID: "auth", Model: "billed", Alias: "requested", RequestedAt: time.Unix(1, 0), ResponseHeaders: headers}
	event := ToEvent(record)
	want := responsemodel.CorrelationKey(record.AuthID, headers)
	if want == "" || event.ResponseCorrelationKey != want {
		t.Fatalf("key=%q want %q", event.ResponseCorrelationKey, want)
	}
	originalHash := event.Hash
	event.ResponseCorrelationKey = "different metadata"
	event.ResponseModel = "upstream"
	if eventHash(event) != originalHash {
		t.Fatal("response metadata changed usage hash")
	}
	if event.Model != "billed" || event.Alias != "requested" {
		t.Fatalf("routed model changed: %#v", event)
	}
	record.ResponseHeaders = nil
	if key := ToEvent(record).ResponseCorrelationKey; key != "" {
		t.Fatalf("invented absent-header key: %q", key)
	}
	record.ResponseHeaders = headers
	record.AuthID = ""
	if key := ToEvent(record).ResponseCorrelationKey; key != "" {
		t.Fatalf("invented absent-auth key: %q", key)
	}
}

func TestDecodeEventCarriesBothHostMetadataAndCorrelation(t *testing.T) {
	event, err := DecodeEvent([]byte(`{"AuthID":"auth","Model":"billed","RequestedAt":"2026-01-01T00:00:00Z","ResponseModel":"host","ResponseHeaders":{"X-Request-Id":["request-123"]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if event.ResponseModel != "host" || event.ResponseCorrelationKey == "" {
		t.Fatalf("event=%#v", event)
	}
}
