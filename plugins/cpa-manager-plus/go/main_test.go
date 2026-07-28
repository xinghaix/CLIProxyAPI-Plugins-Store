package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestResourceFileForPath(t *testing.T) {
	for _, requestPath := range []string{resourceAppPath, resourceAppPath + "/"} {
		file, ok := resourceFileForPath(requestPath)
		if !ok || file != "web-dist/index.html" {
			t.Fatalf("resourceFileForPath(%q) = %q, %v", requestPath, file, ok)
		}
	}
	if _, ok := resourceFileForPath("/v0/resource/plugins/cpa-manager-plus/../main.go"); ok {
		t.Fatal("path traversal must not resolve")
	}
}

func TestManagementRegistrationIsLocal(t *testing.T) {
	registration := managementRegistration()
	if len(registration.Routes) != 2 {
		t.Fatalf("routes = %#v", registration.Routes)
	}
	foundAPI := false
	for _, route := range registration.Routes {
		if route.Method == http.MethodPost && route.Path == managementAPIPathRel {
			foundAPI = true
		}
	}
	if !foundAPI {
		t.Fatal("local API route is not registered")
	}
}

func TestPluginRegistrationExposesUsagePlugin(t *testing.T) {
	registration := pluginRegistration()
	if !registration.Capabilities.ManagementAPI || !registration.Capabilities.UsagePlugin {
		t.Fatalf("capabilities = %#v", registration.Capabilities)
	}
	if registration.Metadata.Version != "0.4.1" {
		t.Fatalf("version = %s", registration.Metadata.Version)
	}
	for _, field := range registration.Metadata.ConfigFields {
		if strings.Contains(field.Name, "manager") || strings.Contains(field.Name, "proxy") {
			t.Fatalf("external manager field remained: %s", field.Name)
		}
	}
}

func TestEnvelope(t *testing.T) {
	body, err := okEnvelope(map[string]string{"runtime": "local"})
	if err != nil {
		t.Fatal(err)
	}
	var parsed envelope
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatal(err)
	}
	if !parsed.OK || !strings.Contains(string(parsed.Result), "local") {
		t.Fatalf("unexpected envelope: %s", body)
	}
}
