package store

import "testing"

func TestAuthTypeDoesNotChangeAPIEstimate(t *testing.T) {
	usage := eventRow{Model: "gpt-5.4", InputTokens: 100_000, ServiceTier: "fast"}
	oauth := usage
	oauth.AuthType = "oauth"
	apiKey := usage
	apiKey.AuthType = "api_key"
	left := estimateEventCost(oauth, Price{})
	right := estimateEventCost(apiKey, Price{})
	if left != right {
		t.Fatalf("auth type changed the API estimate: oauth=%+v api=%+v", left, right)
	}
	if left.Status != "estimated" || left.ServiceTier != "fast" || left.TierSource != "requested" {
		t.Fatalf("expected the same requested Fast estimate: %+v", left)
	}
}
