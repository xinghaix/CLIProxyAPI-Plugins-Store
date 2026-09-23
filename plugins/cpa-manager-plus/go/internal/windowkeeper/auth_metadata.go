package windowkeeper

import (
	"encoding/base64"
	"encoding/json"
	"strings"
)

// AuthMetadata extracts only non-secret identity fields from a host.auth.get payload.
// The raw credential document must never be logged or persisted by callers.
func AuthMetadata(document json.RawMessage) (planType, accountID string) {
	var root map[string]any
	if json.Unmarshal(document, &root) != nil || root == nil {
		return "", ""
	}
	scopes := make([]map[string]any, 0, 12)
	appendScope := func(value any) {
		if scope, ok := value.(map[string]any); ok {
			scopes = append(scopes, scope)
		}
	}
	appendScope(root)
	for _, key := range []string{"json", "auth", "metadata", "attributes", "oauth", "user", "subscription", "installed", "web", "token", "id_token_claims"} {
		appendScope(root[key])
	}
	for _, scope := range scopes {
		if planType == "" {
			planType = firstString(scope, "plan_type", "planType", "account_plan_type")
		}
		if accountID == "" {
			accountID = firstString(scope, "chatgpt_account_id", "chatgptAccountId", "account_id")
		}
		for _, key := range []string{"id_token", "idToken"} {
			if token, ok := scope[key].(string); ok {
				claims := decodeJWTClaims(token)
				if planType == "" {
					planType = firstString(claims, "plan_type", "planType", "chatgpt_plan_type")
				}
				if accountID == "" {
					accountID = firstString(claims, "chatgpt_account_id", "chatgptAccountId", "account_id")
				}
			}
		}
		if planType != "" && accountID != "" {
			break
		}
	}
	return strings.ToLower(strings.TrimSpace(planType)), strings.TrimSpace(accountID)
}

func firstString(scope map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := scope[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func decodeJWTClaims(token string) map[string]any {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		payload, err = base64.URLEncoding.DecodeString(parts[1])
	}
	if err != nil {
		return nil
	}
	var claims map[string]any
	if json.Unmarshal(payload, &claims) != nil {
		return nil
	}
	return claims
}
