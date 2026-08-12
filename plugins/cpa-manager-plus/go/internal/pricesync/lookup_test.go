package pricesync

import (
	"context"
	"net/http"
	"testing"
)

func TestLookupReturnsSelectableExactSourcePrices(t *testing.T) {
	fetch := func(_ context.Context, url string, _ http.Header) (HTTPResponse, error) {
		switch url {
		case ModelsDevURL:
			return HTTPResponse{StatusCode: http.StatusOK, Body: []byte(`{"xai":{"models":{"gpt-test":{"id":"gpt-test","cost":{"input":2,"output":6,"cache_read":0.3}}}}}`)}, nil
		case LiteLLMURL:
			return HTTPResponse{StatusCode: http.StatusOK, Body: []byte(`{"gpt-test":{"input_cost_per_token":0.000003,"output_cost_per_token":0.000007}}`)}, nil
		default:
			return HTTPResponse{StatusCode: http.StatusOK, Body: []byte(`{"data":[{"id":"gpt-test","pricing":{"prompt":"0.000004","completion":"0.000008"}}]}`)}, nil
		}
	}
	result, err := Lookup(context.Background(), "gpt-test", fetch)
	if err != nil {
		t.Fatal(err)
	}
	if result.Model != "gpt-test" || len(result.Sources) != 3 {
		t.Fatalf("result=%#v", result)
	}
	if result.Sources[0].Source != sourceModelsDevXAI || result.Sources[0].Price.Prompt != 2 {
		t.Fatalf("sources=%#v", result.Sources)
	}
	if result.Sources[1].Source != sourceLiteLLM || result.Sources[2].Source != sourceOpenRouter {
		t.Fatalf("source order=%#v", result.Sources)
	}
}

func TestLookupRequiresModel(t *testing.T) {
	if _, err := Lookup(context.Background(), " ", func(context.Context, string, http.Header) (HTTPResponse, error) {
		return HTTPResponse{}, nil
	}); err == nil {
		t.Fatal("expected model validation error")
	}
}
