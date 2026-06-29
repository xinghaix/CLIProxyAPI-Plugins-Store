package main

import (
	"net/http"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func TestNormalizeConfig(t *testing.T) {
	cfg := normalizeConfig(pluginConfig{
		ManagerBaseURL: " http://127.0.0.1:18317/ ",
		AdminKey:       " legacy-key ",
	})
	if cfg.ManagerBaseURL != "http://127.0.0.1:18317" {
		t.Fatalf("ManagerBaseURL = %q", cfg.ManagerBaseURL)
	}
	if cfg.ManagementKey != "legacy-key" {
		t.Fatalf("ManagementKey = %q", cfg.ManagementKey)
	}
	if cfg.AdminKey != "" {
		t.Fatalf("AdminKey was not cleared: %q", cfg.AdminKey)
	}

	cfg = normalizeConfig(pluginConfig{ManagementKey: " next-key ", AdminKey: "legacy-key"})
	if cfg.ManagerBaseURL != defaultManagerBaseURL {
		t.Fatalf("default ManagerBaseURL = %q", cfg.ManagerBaseURL)
	}
	if cfg.ManagementKey != "next-key" {
		t.Fatalf("ManagementKey should prefer management_key, got %q", cfg.ManagementKey)
	}
}

func TestBuildManagerURL(t *testing.T) {
	got, err := buildManagerURL("http://127.0.0.1:18317/", "/v0/management/usage", "model=gpt-5&space=a+b")
	if err != nil {
		t.Fatalf("buildManagerURL error: %v", err)
	}
	want := "http://127.0.0.1:18317/v0/management/usage?model=gpt-5&space=a+b"
	if got != want {
		t.Fatalf("buildManagerURL = %q, want %q", got, want)
	}

	if _, err := buildManagerURL("http://127.0.0.1:18317", "/usage-service/info", "%zz"); err == nil {
		t.Fatal("expected invalid query error")
	}
}

func TestManagerAuthorization(t *testing.T) {
	if got := managerAuthorization(pluginConfig{}, pluginapi.ManagementRequest{}); got != "" {
		t.Fatalf("empty key auth = %q", got)
	}
	if got := managerAuthorization(pluginConfig{ManagementKey: "abc"}, pluginapi.ManagementRequest{}); got != "Bearer abc" {
		t.Fatalf("raw key auth = %q", got)
	}
	if got := managerAuthorization(pluginConfig{ManagementKey: "Bearer abc"}, pluginapi.ManagementRequest{}); got != "Bearer abc" {
		t.Fatalf("bearer key auth = %q", got)
	}
}

func TestValidateProxyTarget(t *testing.T) {
	allowed := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/health"},
		{http.MethodGet, "/usage-service/info"},
		{http.MethodGet, "/v0/management/dashboard/summary"},
		{http.MethodGet, "/v0/management/monitoring/analytics"},
		{http.MethodPost, "/v0/management/codex-inspection/run"},
		{http.MethodPost, "/v0/management/codex-inspection/runs/12/actions"},
	}
	for _, tc := range allowed {
		if err := validateProxyTarget(tc.method, tc.path); err != nil {
			t.Fatalf("validateProxyTarget(%s, %s) unexpected error: %v", tc.method, tc.path, err)
		}
	}

	blocked := []struct {
		method string
		path   string
	}{
		{http.MethodConnect, "/health"},
		{http.MethodGet, "/management.html"},
		{http.MethodGet, "/v0/management/config"},
		{http.MethodGet, "http://evil.test/health"},
	}
	for _, tc := range blocked {
		if err := validateProxyTarget(tc.method, tc.path); err == nil {
			t.Fatalf("validateProxyTarget(%s, %s) expected error", tc.method, tc.path)
		}
	}
}

func TestProxyResponseHeaders(t *testing.T) {
	headers := proxyResponseHeaders(map[string][]string{"content-type": {"text/plain; charset=utf-8"}, "X-Other": {"ignored"}})
	if got := headers.Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := headers.Get("X-Other"); got != "" {
		t.Fatalf("unexpected X-Other passthrough: %q", got)
	}
	fallback := proxyResponseHeaders(nil)
	if !strings.HasPrefix(fallback.Get("Content-Type"), "application/json") {
		t.Fatalf("fallback Content-Type = %q", fallback.Get("Content-Type"))
	}
}

func TestResourceFileForPath(t *testing.T) {
	cases := []struct {
		requestPath string
		wantFile    string
		wantOK      bool
	}{
		{resourceAppPath, "web-dist/index.html", true},
		{"/v0/resource/plugins/cpa-manager-plus/app/", "web-dist/index.html", true},
		{"/v0/resource/plugins/cpa-manager-plus/assets/app.js", "", false},
		{"/v0/resource/plugins/cpa-manager-plus/../main.go", "", false},
		{"/v0/resource/plugins/other/app", "", false},
	}
	for _, tc := range cases {
		got, ok := resourceFileForPath(tc.requestPath)
		if ok != tc.wantOK || got != tc.wantFile {
			t.Fatalf("resourceFileForPath(%q) = (%q, %v), want (%q, %v)", tc.requestPath, got, ok, tc.wantFile, tc.wantOK)
		}
	}
}

func TestContentTypeForResourceFile(t *testing.T) {
	if got := contentTypeForResourceFile("web-dist/index.html"); got != contentTypeHTML {
		t.Fatalf("contentTypeForResourceFile = %q, want %q", got, contentTypeHTML)
	}
	if got := contentTypeForResourceFile("anything.bin"); got != contentTypeHTML {
		t.Fatalf("contentTypeForResourceFile = %q, want %q", got, contentTypeHTML)
	}
}

func TestHandleResourceReadsEmbeddedVueAssets(t *testing.T) {
	app := handleResource(resourceAppPath)
	if app.StatusCode != http.StatusOK {
		t.Fatalf("app status = %d", app.StatusCode)
	}
	appBody := string(app.Body)
	if !strings.Contains(appBody, "COMPILED VUE APP") && !strings.Contains(appBody, "<meta charset") {
		t.Fatalf("app html does not look like compiled Vue app")
	}
	if got := app.Headers.Get("Content-Type"); got != contentTypeHTML {
		t.Fatalf("app Content-Type = %q", got)
	}

	unknown := handleResource("/v0/resource/plugins/cpa-manager-plus/assets/app.js")
	if unknown.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown resource status = %d, want 404", unknown.StatusCode)
	}
}
