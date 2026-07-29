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
	if registration.Metadata.Version != "0.5.4" {
		t.Fatalf("version = %s", registration.Metadata.Version)
	}
	if registration.SchemaVersion != 1 {
		t.Fatalf("schema version = %d, want 1", registration.SchemaVersion)
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

func TestDecodeHostAuthList(t *testing.T) {
	tests := []struct {
		name         string
		raw          string
		wantNames    []string
		wantProvider []string
		wantErr      bool
	}{
		{
			name:         "host files wrapper",
			raw:          `{"files":[{"name":"codex.json","provider":"codex","auth_index":"codex-1"}]}`,
			wantNames:    []string{"codex.json"},
			wantProvider: []string{"codex"},
		},
		{
			name: "empty files wrapper",
			raw:  `{"files":[]}`,
		},
		{
			name:      "top level array",
			raw:       `[{"name":"xai.json","provider":"xai"}]`,
			wantNames: []string{"xai.json"},
		},
		{
			name:      "legacy items wrapper",
			raw:       `{"items":[{"name":"items.json","provider":"codex"}]}`,
			wantNames: []string{"items.json"},
		},
		{
			name:      "legacy auths wrapper",
			raw:       `{"auths":[{"name":"auths.json","provider":"xai"}]}`,
			wantNames: []string{"auths.json"},
		},
		{
			name:      "files takes precedence",
			raw:       `{"files":[{"name":"files.json","provider":"codex"}],"items":[{"name":"items.json","provider":"xai"}]}`,
			wantNames: []string{"files.json"},
		},
		{
			name:    "unknown wrapper",
			raw:     `{"entries":[]}`,
			wantErr: true,
		},
		{
			name:    "invalid JSON",
			raw:     `{`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries, err := decodeHostAuthList(json.RawMessage(tt.raw))
			if (err != nil) != tt.wantErr {
				t.Fatalf("decodeHostAuthList() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if len(entries) != len(tt.wantNames) {
				t.Fatalf("entry count = %d, want %d", len(entries), len(tt.wantNames))
			}
			for index, wantName := range tt.wantNames {
				if entries[index].Name != wantName {
					t.Errorf("entries[%d].Name = %q, want %q", index, entries[index].Name, wantName)
				}
				if index < len(tt.wantProvider) && entries[index].Provider != tt.wantProvider[index] {
					t.Errorf("entries[%d].Provider = %q, want %q", index, entries[index].Provider, tt.wantProvider[index])
				}
			}
		})
	}
}
