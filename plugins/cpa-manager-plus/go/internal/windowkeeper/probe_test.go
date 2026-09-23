package windowkeeper

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestProbeUsageUsesCPARequestAndDecodesBodyVariants(t *testing.T) {
	responses := []struct {
		name string
		body string
	}{
		{name: "string body", body: "{\"status_code\":200,\"body\":\"{\\\"plan_type\\\":\\\"plus\\\",\\\"rate_limit\\\":{\\\"primary_window\\\":{\\\"used_percent\\\":10,\\\"limit_window_seconds\\\":604800,\\\"reset_at\\\":1790000000}}}\"}"},
		{name: "object body", body: "{\"statusCode\":200,\"body\":{\"plan_type\":\"plus\",\"rate_limit\":{\"primary_window\":{\"used_percent\":10,\"limit_window_seconds\":604800,\"reset_at\":1790000000}}}}"},
	}
	for _, tc := range responses {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			probe, err := ProbeUsage(context.Background(), func(method, target string, headers map[string]string, body []byte) (int, []byte, error) {
				called = true
				if method != http.MethodPost || target != "http://127.0.0.1:8317/v0/management/api-call" {
					t.Fatalf("call %s %s", method, target)
				}
				if headers["Authorization"] != "Bearer management-key" {
					t.Fatalf("auth header = %q", headers["Authorization"])
				}
				var request map[string]any
				if err := json.Unmarshal(body, &request); err != nil {
					t.Fatal(err)
				}
				if request["authIndex"] != "idx-1" {
					t.Fatalf("request authIndex = %#v", request)
				}
				h, ok := request["header"].(map[string]any)
				if !ok || h["Chatgpt-Account-Id"] != "acct-1" {
					t.Fatalf("upstream header = %#v", request["header"])
				}
				return 200, []byte(tc.body), nil
			}, "http://127.0.0.1:8317", "management-key", "idx-1", "acct-1", "codex_cli_rs/0.76.0", time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC))
			if err != nil {
				t.Fatal(err)
			}
			if !called || probe.PlanType != "plus" || len(probe.Windows) != 1 {
				t.Fatalf("probe = %+v", probe)
			}
		})
	}
}
