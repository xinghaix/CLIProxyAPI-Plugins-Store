package app

import (
	"context"
	"encoding/json"
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


func TestConfirmPriceSyncCandidateRemovesFromLastResultAndPersists(t *testing.T) {
	runtime, err := New([]byte("data_dir: " + t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()

	runtime.priceMu.Lock()
	runtime.priceStatus.LastResult = &pricesync.Result{
		Imported: 0,
		Candidates: []pricesync.CandidateGroup{
			{Model: "claude-test", Candidates: []pricesync.Candidate{{Source: "openrouter", SourceModelID: "vendor/claude-test", Score: 0.8}}},
			{Model: "other-model", Candidates: []pricesync.Candidate{{Source: "litellm", SourceModelID: "other-model", Score: 0.7}}},
		},
	}
	runtime.priceMu.Unlock()

	price := store.Price{Prompt: 3, Completion: 15, Source: "openrouter", SourceModelID: "vendor/claude-test"}
	if err := runtime.ConfirmPriceSyncCandidate(context.Background(), "claude-test", price); err != nil {
		t.Fatal(err)
	}

	prices, err := runtime.Store().Prices(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if prices["claude-test"].Source != "openrouter" || prices["claude-test"].Prompt != 3 {
		t.Fatalf("prices=%#v", prices["claude-test"])
	}

	status := runtime.PriceSyncStatus()
	if status.LastResult == nil {
		t.Fatal("expected lastResult")
	}
	if status.LastResult.Imported != 0 {
		t.Fatalf("imported=%d", status.LastResult.Imported)
	}
	if len(status.LastResult.Candidates) != 1 || status.LastResult.Candidates[0].Model != "other-model" {
		t.Fatalf("candidates=%#v", status.LastResult.Candidates)
	}

	raw, ok, err := runtime.Store().Setting(context.Background(), priceSyncStatusKey)
	if err != nil || !ok {
		t.Fatalf("setting err=%v ok=%v", err, ok)
	}
	var persisted PriceSyncStatus
	if err := json.Unmarshal(raw, &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted.LastResult == nil || len(persisted.LastResult.Candidates) != 1 || persisted.LastResult.Candidates[0].Model != "other-model" {
		t.Fatalf("persisted=%#v", persisted.LastResult)
	}
}

func TestDismissPriceSyncCandidatesClearsAndPersists(t *testing.T) {
	runtime, err := New([]byte("data_dir: " + t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()

	runtime.priceMu.Lock()
	runtime.priceStatus.LastResult = &pricesync.Result{
		Candidates: []pricesync.CandidateGroup{
			{Model: "a"},
			{Model: "b"},
		},
	}
	runtime.priceMu.Unlock()

	if err := runtime.DismissPriceSyncCandidates(context.Background(), []string{"a"}); err != nil {
		t.Fatal(err)
	}
	status := runtime.PriceSyncStatus()
	if len(status.LastResult.Candidates) != 1 || status.LastResult.Candidates[0].Model != "b" {
		t.Fatalf("after partial dismiss=%#v", status.LastResult.Candidates)
	}

	if err := runtime.DismissPriceSyncCandidates(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	status = runtime.PriceSyncStatus()
	if len(status.LastResult.Candidates) != 0 {
		t.Fatalf("after full dismiss=%#v", status.LastResult.Candidates)
	}
}

func TestSyncPricesDropsCandidatesForAlreadyPricedModels(t *testing.T) {
	runtime, err := New([]byte("data_dir: " + t.TempDir() + "\nbatch_size: 1"))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()

	// Seed usage so model is a sync target, plus an existing price (e.g. previously confirmed).
	runtime.HandleUsage(pluginapi.UsageRecord{Model: "claude-test", RequestedAt: time.Now(), Detail: pluginapi.UsageDetail{TotalTokens: 1}})
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
	if _, err := runtime.Store().UpsertSyncedPrices(context.Background(), map[string]store.Price{
		"claude-test": {Prompt: 1, Completion: 2, Source: "openrouter", SourceModelID: "vendor/claude-test"},
	}); err != nil {
		t.Fatal(err)
	}

	runtime.SetHTTPDo(func(_ context.Context, _ string, target string, _ http.Header, _ []byte) (pricesync.HTTPResponse, error) {
		if target == pricesync.LiteLLMURL {
			// No exact match; fuzzy-capable remote id only.
			return pricesync.HTTPResponse{StatusCode: 200, Body: []byte(`{}`)}, nil
		}
		return pricesync.HTTPResponse{StatusCode: 200, Body: []byte(`{"data":[{"id":"vendor/claude-test","pricing":{"prompt":"0.000003","completion":"0.000015"}}]}`)}, nil
	})

	result, err := runtime.SyncPrices(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Candidates) != 0 {
		t.Fatalf("expected no candidates for the confirmed source mapping, got %#v", result.Candidates)
	}
	status := runtime.PriceSyncStatus()
	if status.LastResult == nil || len(status.LastResult.Candidates) != 0 {
		t.Fatalf("status candidates=%#v", status.LastResult)
	}
}

func TestConfirmThenSyncDoesNotResurfaceCandidate(t *testing.T) {
	runtime, err := New([]byte("data_dir: " + t.TempDir() + "\nbatch_size: 1"))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()

	runtime.HandleUsage(pluginapi.UsageRecord{Model: "claude-test", RequestedAt: time.Now(), Detail: pluginapi.UsageDetail{TotalTokens: 1}})
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
			return pricesync.HTTPResponse{StatusCode: 200, Body: []byte(`{}`)}, nil
		}
		return pricesync.HTTPResponse{StatusCode: 200, Body: []byte(`{"data":[{"id":"vendor/claude-test","pricing":{"prompt":"0.000003","completion":"0.000015"}}]}`)}, nil
	})

	first, err := runtime.SyncPrices(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Candidates) == 0 {
		t.Fatalf("expected fuzzy candidate on first sync: %#v", first)
	}

	cand := first.Candidates[0]
	if cand.Model != "claude-test" || len(cand.Candidates) == 0 {
		t.Fatalf("candidates=%#v", first.Candidates)
	}
	price := cand.Candidates[0].Price
	price.Source = cand.Candidates[0].Source
	price.SourceModelID = cand.Candidates[0].SourceModelID
	if err := runtime.ConfirmPriceSyncCandidate(context.Background(), cand.Model, price); err != nil {
		t.Fatal(err)
	}
	if len(runtime.PriceSyncStatus().LastResult.Candidates) != 0 {
		t.Fatalf("after confirm candidates=%#v", runtime.PriceSyncStatus().LastResult.Candidates)
	}

	second, err := runtime.SyncPrices(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Candidates) != 0 {
		t.Fatalf("candidate resurfaced after confirm: %#v", second.Candidates)
	}
}

func TestFilterResolvedCandidateGroups(t *testing.T) {
	groups := []pricesync.CandidateGroup{
		{Model: "confirmed", Candidates: []pricesync.Candidate{{Source: "litellm", SourceModelID: "vendor/confirmed"}}},
		{Model: "manual", Candidates: []pricesync.Candidate{{Source: "litellm", SourceModelID: "vendor/manual"}}},
		{Model: "different-source", Candidates: []pricesync.Candidate{{Source: "litellm", SourceModelID: "vendor/new"}}},
	}
	out := filterResolvedCandidateGroups(groups, map[string]store.Price{
		"confirmed":        {Source: "LiteLLM", SourceModelID: "vendor/confirmed"},
		"manual":           {Source: "manual"},
		"different-source": {Source: "openrouter", SourceModelID: "vendor/old"},
	})
	if len(out) != 2 || out[0].Model != "manual" || out[1].Model != "different-source" {
		t.Fatalf("out=%#v", out)
	}
	if len(filterResolvedCandidateGroups(nil, map[string]store.Price{"a": {}})) != 0 {
		t.Fatal("nil groups should yield empty slice")
	}
}
