package main

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

const testUpstreamBaseURL = "https://codex-relay.example.com"

func TestAuthIdentifierClaimsCodex(t *testing.T) {
	raw, errHandle := handleMethod(pluginabi.MethodAuthIdentifier, nil)
	if errHandle != nil {
		t.Fatalf("auth.identifier returned error: %v", errHandle)
	}
	var payload struct {
		Identifier string `json:"identifier"`
	}
	decodeResult(t, raw, &payload)
	if payload.Identifier != "codex" {
		t.Fatalf("identifier = %q, want codex", payload.Identifier)
	}
}

func TestParseCodexOAuthAuthRewritesBaseURLAndKeepsHostDerivedAttributes(t *testing.T) {
	configureForTest(t, testUpstreamBaseURL)
	metadata := map[string]any{
		"type":          "codex",
		"access_token":  "access-token",
		"refresh_token": "refresh-token",
		"id_token":      idTokenWithPlanType(t, "plus"),
		"account_id":    "account-1",
		"email":         "user@example.com",
		"priority":      float64(7),
		"note":          "  primary seat  ",
	}
	response := parseForTest(t, metadata)
	if !response.Handled {
		t.Fatal("codex OAuth auth file was not handled")
	}
	if response.Auth.Provider != "codex" {
		t.Fatalf("provider = %q, want codex", response.Auth.Provider)
	}
	if response.Auth.Label != "user@example.com" {
		t.Fatalf("label = %q, want the account email", response.Auth.Label)
	}
	attributes := response.Auth.Attributes
	for key, want := range map[string]string{
		"base_url":  testUpstreamBaseURL,
		"plan_type": "plus",
		"priority":  "7",
		"note":      "primary seat",
	} {
		if attributes[key] != want {
			t.Errorf("attributes[%q] = %q, want %q", key, attributes[key], want)
		}
	}
	for key, want := range map[string]any{
		"access_token":  "access-token",
		"refresh_token": "refresh-token",
		"account_id":    "account-1",
		"email":         "user@example.com",
	} {
		if response.Auth.Metadata[key] != want {
			t.Errorf("metadata[%q] = %#v, want %#v", key, response.Auth.Metadata[key], want)
		}
	}
}

func TestParseWithoutConfiguredBaseURLKeepsHostSynthesizerInCharge(t *testing.T) {
	configureForTest(t, "")
	response := parseForTest(t, map[string]any{
		"type":         "codex",
		"access_token": "access-token",
	})
	if response.Handled {
		t.Fatal("unconfigured plugin must return the auth file to the built-in synthesizer")
	}
}

func TestParseHonoursPerAuthBaseURLOverride(t *testing.T) {
	configureForTest(t, testUpstreamBaseURL)
	response := parseForTest(t, map[string]any{
		"type":         "codex",
		"access_token": "access-token",
		"base-url":     "https://per-auth.example.com",
	})
	if !response.Handled {
		t.Fatal("per-auth base-url override was not handled")
	}
	if got := response.Auth.Attributes["base_url"]; got != "https://per-auth.example.com" {
		t.Fatalf("base_url = %q, want the per-auth override", got)
	}
}

func TestParseLeavesForeignAndAPIKeyPayloadsAlone(t *testing.T) {
	configureForTest(t, testUpstreamBaseURL)
	cases := map[string]map[string]any{
		"non-codex provider": {"type": "claude", "access_token": "token"},
		"api key only":       {"type": "codex", "api_key": "sk-test"},
		"no provider type":   {"access_token": "token"},
	}
	for name, metadata := range cases {
		if response := parseForTest(t, metadata); response.Handled {
			t.Errorf("%s: payload must not be handled", name)
		}
	}
}

func TestParseRejectsUnreadableAuthPayloads(t *testing.T) {
	configureForTest(t, testUpstreamBaseURL)
	for name, rawJSON := range map[string][]byte{
		"malformed json": []byte("{not json"),
		"empty file":     nil,
		"json array":     []byte("[1,2,3]"),
	} {
		request, errMarshal := json.Marshal(pluginapi.AuthParseRequest{Provider: "codex", RawJSON: rawJSON})
		if errMarshal != nil {
			t.Fatalf("%s: marshal auth parse request: %v", name, errMarshal)
		}
		response, errHandle := handleMethod(pluginabi.MethodAuthParse, request)
		if errHandle != nil {
			t.Fatalf("%s: auth.parse returned error: %v", name, errHandle)
		}
		var parsed pluginapi.AuthParseResponse
		decodeResult(t, response, &parsed)
		if parsed.Handled {
			t.Errorf("%s: unreadable auth payload must not be handled", name)
		}
	}
}

func TestUnimplementedAuthHooksFailLoudly(t *testing.T) {
	for _, method := range []string{pluginabi.MethodAuthLoginStart, pluginabi.MethodAuthLoginPoll, pluginabi.MethodAuthRefresh} {
		raw, errHandle := handleMethod(method, nil)
		if errHandle != nil {
			t.Fatalf("%s returned a transport error: %v", method, errHandle)
		}
		var envelope envelope
		if errUnmarshal := json.Unmarshal(raw, &envelope); errUnmarshal != nil {
			t.Fatalf("%s: decode envelope: %v", method, errUnmarshal)
		}
		if envelope.OK || envelope.Error == nil || envelope.Error.Code != "unsupported_method" {
			t.Fatalf("%s: want an unsupported_method error envelope, got %s", method, raw)
		}
	}
}

func TestConfigureReadsBaseURLFromPluginConfigYAML(t *testing.T) {
	request, errMarshal := json.Marshal(lifecycleRequest{ConfigYAML: []byte("enabled: true\npriority: 1\nbase-url: \"https://yaml.example.com/\"\n")})
	if errMarshal != nil {
		t.Fatalf("marshal lifecycle request: %v", errMarshal)
	}
	if _, errHandle := handleMethod(pluginabi.MethodPluginRegister, request); errHandle != nil {
		t.Fatalf("plugin.register returned error: %v", errHandle)
	}
	if got := currentBaseURL(); got != "https://yaml.example.com" {
		t.Fatalf("configured base URL = %q, want https://yaml.example.com", got)
	}
}

func TestBaseURLProblemClassifiesValues(t *testing.T) {
	cases := map[string]string{
		"":                                "",
		"https://codex-relay.example.com": "",
		"http://127.0.0.1:18999":          "",
		"codex-relay.example.com":         "missing an http:// or https:// scheme",
		"ftp://codex-relay.example.com":   "missing an http:// or https:// scheme",
		"https://":                        "missing a host",
		"http://[::1]:namedport":          "not a valid URL: parse \"http://[::1]:namedport\": invalid port \":namedport\" after host",
	}
	for baseURL, want := range cases {
		if got := baseURLProblem(baseURL); got != want {
			t.Errorf("baseURLProblem(%q) = %q, want %q", baseURL, got, want)
		}
	}
}

func configureForTest(t *testing.T, baseURL string) {
	t.Helper()
	configYAML := "enabled: true\n"
	if baseURL != "" {
		configYAML += "base-url: \"" + baseURL + "\"\n"
	}
	request, errMarshal := json.Marshal(lifecycleRequest{ConfigYAML: []byte(configYAML)})
	if errMarshal != nil {
		t.Fatalf("marshal lifecycle request: %v", errMarshal)
	}
	if _, errHandle := handleMethod(pluginabi.MethodPluginReconfigure, request); errHandle != nil {
		t.Fatalf("plugin.reconfigure returned error: %v", errHandle)
	}
}

func parseForTest(t *testing.T, metadata map[string]any) pluginapi.AuthParseResponse {
	t.Helper()
	raw, errMarshal := json.Marshal(metadata)
	if errMarshal != nil {
		t.Fatalf("marshal auth metadata: %v", errMarshal)
	}
	request, errMarshalRequest := json.Marshal(pluginapi.AuthParseRequest{
		Provider: "codex",
		Path:     "/auths/codex-user.json",
		FileName: "codex-user.json",
		RawJSON:  raw,
	})
	if errMarshalRequest != nil {
		t.Fatalf("marshal auth parse request: %v", errMarshalRequest)
	}
	response, errHandle := handleMethod(pluginabi.MethodAuthParse, request)
	if errHandle != nil {
		t.Fatalf("auth.parse returned error: %v", errHandle)
	}
	var parsed pluginapi.AuthParseResponse
	decodeResult(t, response, &parsed)
	return parsed
}

func decodeResult(t *testing.T, raw []byte, target any) {
	t.Helper()
	var parsed struct {
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result"`
		Error  *envelopeError  `json:"error"`
	}
	if errUnmarshal := json.Unmarshal(raw, &parsed); errUnmarshal != nil {
		t.Fatalf("decode envelope: %v", errUnmarshal)
	}
	if !parsed.OK {
		t.Fatalf("envelope is not ok: %s", raw)
	}
	if errUnmarshal := json.Unmarshal(parsed.Result, target); errUnmarshal != nil {
		t.Fatalf("decode result: %v", errUnmarshal)
	}
}

// idTokenWithPlanType builds an unsigned JWT carrying the OpenAI plan claim.
func idTokenWithPlanType(t *testing.T, planType string) string {
	t.Helper()
	payload, errMarshal := json.Marshal(map[string]any{
		codexAuthInfoClaim: map[string]any{"chatgpt_plan_type": planType},
	})
	if errMarshal != nil {
		t.Fatalf("marshal id token claims: %v", errMarshal)
	}
	return "header." + base64.RawURLEncoding.EncodeToString(payload) + ".signature"
}
