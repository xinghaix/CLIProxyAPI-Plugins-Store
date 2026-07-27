package pricesync

import (
	"context"
	"net/http"
	"testing"
)

func TestRunMapsSourcesAndKeepsCandidatesUnapplied(t *testing.T) {
	fetch := func(_ context.Context, url string, _ http.Header) (HTTPResponse, error) {
		if url == LiteLLMURL {
			return HTTPResponse{StatusCode: 200, Body: []byte(`{"gpt-4.1":{"input_cost_per_token":0.000002,"output_cost_per_token":0.000008},"gpt-4.1-mini":{"input_cost_per_token":0.0000004,"output_cost_per_token":0.0000016}}`)}, nil
		}
		return HTTPResponse{StatusCode: 200, Body: []byte(`{"data":[{"id":"vendor/claude-test","pricing":{"prompt":"0.000003","completion":"0.000015"}}]}`)}, nil
	}
	result, err := Run(context.Background(), []string{"gpt-4.1", "claude-test", "unknown"}, fetch)
	if err != nil {
		t.Fatal(err)
	}
	price := result.Matched["gpt-4.1"]
	if price.Prompt != 2 || price.Completion != 8 || price.Source != "litellm" {
		t.Fatalf("price=%#v", price)
	}
	if len(result.Candidates) != 1 || result.Candidates[0].Model != "claude-test" {
		t.Fatalf("candidates=%#v", result.Candidates)
	}
	if len(result.Unmatched) != 1 || result.Unmatched[0] != "unknown" {
		t.Fatalf("unmatched=%#v", result.Unmatched)
	}
}

func TestRunMatchesCaseInsensitiveModelID(t *testing.T) {
	fetch := func(_ context.Context, url string, _ http.Header) (HTTPResponse, error) {
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
	if !ok || price.Prompt != 1 || price.SourceModelID != "gpt-test" {
		t.Fatalf("result=%#v", result)
	}
	if len(result.Candidates) != 0 {
		t.Fatalf("unexpected candidates=%#v", result.Candidates)
	}
}

func TestRunSurvivesSingleSourceFailure(t *testing.T) {
	fetch := func(_ context.Context, url string, _ http.Header) (HTTPResponse, error) {
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
