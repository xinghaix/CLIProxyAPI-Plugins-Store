package app

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/pricesync"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/store"
)

func TestPriceSyncUsesUsageTargetsAndProtectsManual(t *testing.T) {
	runtime, err := New([]byte("data_dir: " + t.TempDir() + "\nbatch_size: 1"))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	runtime.HandleUsage(pluginapi.UsageRecord{Model: "gpt-test", RequestedAt: time.Now(), Detail: pluginapi.UsageDetail{TotalTokens: 1}})
	deadline := time.Now().Add(time.Second)
	for {
		count, err := runtime.Store().EventCount(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if count == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("event not persisted")
		}
		time.Sleep(10 * time.Millisecond)
	}
	runtime.SetHTTPDo(func(_ context.Context, _ string, target string, _ http.Header, _ []byte) (pricesync.HTTPResponse, error) {
		if target == pricesync.LiteLLMURL {
			return pricesync.HTTPResponse{StatusCode: 200, Body: []byte(`{"gpt-test":{"input_cost_per_token":0.000001,"output_cost_per_token":0.000002}}`)}, nil
		}
		return pricesync.HTTPResponse{StatusCode: 500}, nil
	})
	result, err := runtime.SyncPrices(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Imported != 1 {
		t.Fatalf("result=%#v", result)
	}
	prices, err := runtime.Store().Prices(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if prices["gpt-test"].Source != "litellm" {
		t.Fatalf("prices=%#v", prices)
	}
	if err := runtime.Store().ReplacePrices(context.Background(), map[string]store.Price{"gpt-test": {Prompt: 9, Source: "manual"}}); err != nil {
		t.Fatal(err)
	}
	result, err = runtime.SyncPrices(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.ProtectedManual != 1 {
		t.Fatalf("manual protection result=%#v", result)
	}
	prices, _ = runtime.Store().Prices(context.Background())
	if prices["gpt-test"].Prompt != 9 {
		t.Fatalf("manual price overwritten: %#v", prices["gpt-test"])
	}
}

func TestPriceSyncSettingsValidateAndPersist(t *testing.T) {
	runtime, err := New([]byte("data_dir: " + t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := runtime.UpdatePriceSyncSettings(context.Background(), PriceSyncSettings{Enabled: true, IntervalHours: 12}); err != nil {
		t.Fatal(err)
	}
	settings := runtime.PriceSyncSettings()
	if !settings.Enabled || settings.IntervalHours != 12 || !settings.ProtectManual {
		t.Fatalf("settings=%#v", settings)
	}
	if err := runtime.UpdatePriceSyncSettings(context.Background(), PriceSyncSettings{IntervalHours: 1}); err == nil {
		t.Fatal("expected interval validation")
	}
}
