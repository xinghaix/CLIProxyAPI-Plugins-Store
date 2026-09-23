package upstream

import (
	"encoding/json"
	"testing"
)

func TestAuthMetadataNestedAndJWT(t *testing.T) {
	for _, tc := range []struct{ name, document, plan, account string }{
		{name: "nested", document: "{\"metadata\":{\"plan_type\":\"pro\",\"chatgpt_account_id\":\"acct_1\"}}", plan: "pro", account: "acct_1"},
		{name: "token", document: "{\"id_token\":\"e30.eyJwbGFuX3R5cGUiOiJwbHVzIiwiY2hhdGdwdF9hY2NvdW50X2lkIjoiYWNjdF9uZXN0ZWQifQ.sig\"}", plan: "plus", account: "acct_nested"},
		{name: "bad", document: "{not-json", plan: "", account: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan, account := AuthMetadata(json.RawMessage(tc.document))
			if plan != tc.plan || account != tc.account {
				t.Fatalf("AuthMetadata = (%q, %q), want (%q, %q)", plan, account, tc.plan, tc.account)
			}
		})
	}
}
