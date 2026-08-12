package pricesync

import (
	"context"
	"net/http"
	"testing"
)

func TestRunMapsSourcesAndKeepsCandidatesUnapplied(t *testing.T) {
	fetch := func(_ context.Context, url string, _ http.Header) (HTTPResponse, error) {
		switch url {
		case ModelsDevURL:
			return HTTPResponse{StatusCode: 200, Body: []byte(`{"xai":{"models":{"grok-test":{"id":"grok-test","cost":{"input":9,"output":10}}}}}`)}, nil
		case LiteLLMURL:
			return HTTPResponse{StatusCode: 200, Body: []byte(`{"gpt-4.1":{"input_cost_per_token":0.000002,"output_cost_per_token":0.000008},"gpt-4.1-mini":{"input_cost_per_token":0.0000004,"output_cost_per_token":0.0000016}}`)}, nil
		default:
			return HTTPResponse{StatusCode: 200, Body: []byte(`{"data":[{"id":"vendor/claude-test","pricing":{"prompt":"0.000003","completion":"0.000015"}}]}`)}, nil
		}
	}
	result, err := Run(context.Background(), []string{"gpt-4.1", "claude-test", "unknown"}, fetch)
	if err != nil {
		t.Fatal(err)
	}
	price := result.Matched["gpt-4.1"]
	if price.Prompt != 2 || price.Completion != 8 || price.Source != sourceLiteLLM {
		t.Fatalf("price=%#v", price)
	}
	if len(result.Candidates) != 1 || result.Candidates[0].Model != "claude-test" {
		t.Fatalf("candidates=%#v", result.Candidates)
	}
	if len(result.Unmatched) != 1 || result.Unmatched[0] != "unknown" {
		t.Fatalf("unmatched=%#v", result.Unmatched)
	}
}

func TestRunMatchesModelsDevXAIBeforeFallbacks(t *testing.T) {
	fetch := func(_ context.Context, url string, _ http.Header) (HTTPResponse, error) {
		switch url {
		case ModelsDevURL:
			return HTTPResponse{StatusCode: 200, Body: []byte(`{"xai":{"models":{"grok-4.5":{"id":"grok-4.5","cost":{"input":2,"output":6,"cache_read":0.3}}}},"openrouter":{"models":{"x-ai/grok-4.5":{"id":"x-ai/grok-4.5","cost":{"input":99,"output":99,"cache_read":99}}}}}`)}, nil
		case LiteLLMURL:
			return HTTPResponse{StatusCode: 200, Body: []byte(`{"xai/grok-4.5":{"input_cost_per_token":0.000007,"output_cost_per_token":0.000008}}`)}, nil
		default:
			return HTTPResponse{StatusCode: 200, Body: []byte(`{"data":[{"id":"x-ai/grok-4.5","pricing":{"prompt":"0.000009","completion":"0.000010"}}]}`)}, nil
		}
	}
	result, err := Run(context.Background(), []string{"grok-4.5"}, fetch)
	if err != nil {
		t.Fatal(err)
	}
	price := result.Matched["grok-4.5"]
	if price.Source != sourceModelsDevXAI || price.SourceModelID != "xai/grok-4.5" || price.Prompt != 2 || price.Completion != 6 || price.CacheRead != 0.3 {
		t.Fatalf("price=%#v", price)
	}
	if result.SourceResults[0].Source != sourceModelsDevXAI || result.SourceResults[0].Matched != 1 || result.SourceResults[0].Applied != 0 {
		t.Fatalf("source results=%#v", result.SourceResults)
	}
}

func TestRunUsesFieldLevelFallbackWithoutOverwritingPrimaryValues(t *testing.T) {
	fetch := func(_ context.Context, url string, _ http.Header) (HTTPResponse, error) {
		switch url {
		case ModelsDevURL:
			return HTTPResponse{StatusCode: 200, Body: []byte(`{"xai":{"models":{"grok-test":{"id":"grok-test","cost":{"input":2,"output":6}}}}}`)}, nil
		case LiteLLMURL:
			return HTTPResponse{StatusCode: 200, Body: []byte(`{"xai/grok-test":{"input_cost_per_token":0.000009,"output_cost_per_token":0.000010,"cache_read_input_token_cost":0.0000005,"cache_creation_input_token_cost":0.0000007}}`)}, nil
		default:
			return HTTPResponse{StatusCode: 200, Body: []byte(`{"data":[]}`)}, nil
		}
	}
	result, err := Run(context.Background(), []string{"grok-test"}, fetch)
	if err != nil {
		t.Fatal(err)
	}
	price := result.Matched["grok-test"]
	if price.Prompt != 2 || price.Completion != 6 || price.CacheRead != 0.5 || price.CacheCreation != 0.7 {
		t.Fatalf("price=%#v", price)
	}
	if price.Source != sourceModelsDevXAI {
		t.Fatalf("source=%q", price.Source)
	}
}

func TestRunMatchesCaseInsensitiveModelID(t *testing.T) {
	fetch := func(_ context.Context, url string, _ http.Header) (HTTPResponse, error) {
		if url == ModelsDevURL {
			return HTTPResponse{StatusCode: 200, Body: []byte(`{"xai":{"models":{"gpt-test":{"id":"gpt-test","cost":{"input":1}}}}}`)}, nil
		}
		if url == LiteLLMURL {
			return HTTPResponse{StatusCode: 200, Body: []byte(`{"gpt-test":{"input_cost_per_token":0.000001}}`)}, nil
		}
		return HTTPResponse{StatusCode: 503}, nil
	}
	result, err := Run(context.Background(), []string{"GPT-TEST"}, fetch)
	if err != nil {
		t.Fatal(err)
	}
	price, ok := result.Matched["GPT-TEST"]
	if !ok || price.Prompt != 1 || price.Source != sourceModelsDevXAI || price.SourceModelID != "xai/gpt-test" {
		t.Fatalf("result=%#v", result)
	}
	if len(result.Candidates) != 0 {
		t.Fatalf("unexpected candidates=%#v", result.Candidates)
	}
}

func TestRunSurvivesSingleSourceFailure(t *testing.T) {
	fetch := func(_ context.Context, url string, _ http.Header) (HTTPResponse, error) {
		if url == ModelsDevURL {
			return HTTPResponse{StatusCode: 503}, nil
		}
		if url == LiteLLMURL {
			return HTTPResponse{}, assertError{}
		}
		return HTTPResponse{StatusCode: 200, Body: []byte(`{"data":[{"id":"gpt-test","pricing":{"prompt":"0.000001","completion":"0.000002"}}]}`)}, nil
	}
	result, err := Run(context.Background(), []string{"gpt-test"}, fetch)
	if err != nil {
		t.Fatal(err)
	}
	if result.Matched["gpt-test"].Prompt != 1 {
		t.Fatalf("result=%#v", result)
	}
}

type assertError struct{}

func (assertError) Error() string { return "offline" }
