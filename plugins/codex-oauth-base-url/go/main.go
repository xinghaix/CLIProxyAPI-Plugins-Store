// Package main implements the codex-oauth-base-url CLIProxyAPI plugin.
//
// It is published from the CLIProxyAPI-Plugins-Store repository and built as a
// c-shared dynamic library for the CPA C ABI.
//
// Codex OAuth credentials resolve their upstream from base_url in auth.Attributes
// and fall back to a hardcoded https://chatgpt.com/backend-api/codex when the
// attribute is absent. The built-in file synthesizer never writes that attribute
// for OAuth auth files, so the fallback always wins and the upstream cannot be
// changed without patching the host.
//
// This plugin registers a Codex auth provider that fills the gap from outside the
// host source tree. While parsing a Codex OAuth auth file it reproduces every
// attribute the built-in synthesizer derives from that file and adds base_url on
// top, so enabling or removing the plugin changes nothing except the upstream URL.
package main

/*
#include <stdint.h>
#include <stdlib.h>

typedef struct {
	void* ptr;
	size_t len;
} cliproxy_buffer;

typedef int (*cliproxy_host_call_fn)(void*, const char*, const uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_host_free_fn)(void*, size_t);

typedef struct {
	uint32_t abi_version;
	void* host_ctx;
	cliproxy_host_call_fn call;
	cliproxy_host_free_fn free_buffer;
} cliproxy_host_api;

typedef int (*cliproxy_plugin_call_fn)(char*, uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_plugin_free_fn)(void*, size_t);
typedef void (*cliproxy_plugin_shutdown_fn)(void);

typedef struct {
	uint32_t abi_version;
	cliproxy_plugin_call_fn call;
	cliproxy_plugin_free_fn free_buffer;
	cliproxy_plugin_shutdown_fn shutdown;
} cliproxy_plugin_api;

extern int cliproxyPluginCall(char*, uint8_t*, size_t, cliproxy_buffer*);
extern void cliproxyPluginFree(void*, size_t);
extern void cliproxyPluginShutdown(void);

static const cliproxy_host_api* stored_host;

static void store_host_api(const cliproxy_host_api* host) {
	stored_host = host;
}

static int call_host_api(const char* method, const uint8_t* request, size_t request_len, cliproxy_buffer* response) {
	if (stored_host == NULL || stored_host->call == NULL) {
		return 1;
	}
	return stored_host->call(stored_host->host_ctx, method, request, request_len, response);
}

static void free_host_buffer(void* ptr, size_t len) {
	if (stored_host != NULL && stored_host->free_buffer != NULL && ptr != NULL) {
		stored_host->free_buffer(ptr, len);
	}
}
*/
import "C"

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"unsafe"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
	"gopkg.in/yaml.v3"
)

const (
	// pluginName is the plugin file basename and the plugins.configs key.
	pluginName = "codex-oauth-base-url"
	// codexProviderKey is the built-in provider key this plugin attaches to. The
	// host routes an auth file to the plugin whose auth.identifier matches the
	// file's type field, so this must be the literal codex to see Codex auth files.
	codexProviderKey = "codex"
	// codexAuthInfoClaim is the OpenAI namespaced claim holding chatgpt_plan_type.
	codexAuthInfoClaim = "https://api.openai.com/auth"
)

// pluginVersion is the released plugin version. The build sets it with
// -ldflags "-X main.pluginVersion=<version>" so a tag and the shipped library
// can never disagree; the literal here is the development default.
var pluginVersion = "0.1.0"

// pluginConfig is the plugins.configs.<pluginName> subtree owned by this plugin.
type pluginConfig struct {
	// BaseURL is the upstream base URL applied to Codex OAuth credentials that do
	// not carry their own base_url. Empty leaves the built-in default in place.
	BaseURL string `yaml:"base-url"`
}

// lifecycleRequest is the payload the host sends on plugin.register and
// plugin.reconfigure. It carries the raw YAML of this plugin's config subtree.
type lifecycleRequest struct {
	ConfigYAML []byte `json:"config_yaml"`
}

type registration struct {
	SchemaVersion uint32                 `json:"schema_version"`
	Metadata      pluginapi.Metadata     `json:"metadata"`
	Capabilities  registrationCapability `json:"capabilities"`
}

type registrationCapability struct {
	AuthProvider bool `json:"auth_provider"`
}

type envelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *envelopeError  `json:"error,omitempty"`
}

type envelopeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// configuredBaseURL holds the base URL from the last successful configure call.
// It is swapped atomically because plugin.reconfigure can race with parsing.
var configuredBaseURL atomic.Pointer[string]

func main() {}

//export cliproxy_plugin_init
func cliproxy_plugin_init(host *C.cliproxy_host_api, plugin *C.cliproxy_plugin_api) C.int {
	if plugin == nil {
		return 1
	}
	C.store_host_api(host)
	plugin.abi_version = C.uint32_t(pluginabi.ABIVersion)
	plugin.call = C.cliproxy_plugin_call_fn(C.cliproxyPluginCall)
	plugin.free_buffer = C.cliproxy_plugin_free_fn(C.cliproxyPluginFree)
	plugin.shutdown = C.cliproxy_plugin_shutdown_fn(C.cliproxyPluginShutdown)
	return 0
}

//export cliproxyPluginCall
func cliproxyPluginCall(method *C.char, request *C.uint8_t, requestLen C.size_t, response *C.cliproxy_buffer) C.int {
	if response != nil {
		response.ptr = nil
		response.len = 0
	}
	if method == nil {
		writeResponse(response, errorEnvelope("invalid_method", "method is required"))
		return 1
	}
	var requestBytes []byte
	if request != nil && requestLen > 0 {
		requestBytes = C.GoBytes(unsafe.Pointer(request), C.int(requestLen))
	}
	raw, errHandle := handleMethod(C.GoString(method), requestBytes)
	if errHandle != nil {
		writeResponse(response, errorEnvelope("plugin_error", errHandle.Error()))
		return 1
	}
	writeResponse(response, raw)
	return 0
}

//export cliproxyPluginFree
func cliproxyPluginFree(ptr unsafe.Pointer, len C.size_t) {
	if ptr != nil {
		C.free(ptr)
	}
}

//export cliproxyPluginShutdown
func cliproxyPluginShutdown() {}

func handleMethod(method string, request []byte) ([]byte, error) {
	switch method {
	case pluginabi.MethodPluginRegister, pluginabi.MethodPluginReconfigure:
		if errConfigure := configure(request); errConfigure != nil {
			return nil, errConfigure
		}
		return okEnvelope(pluginRegistration())
	case pluginabi.MethodAuthIdentifier:
		return okEnvelope(map[string]string{"identifier": codexProviderKey})
	case pluginabi.MethodAuthParse:
		return parseAuth(request)
	case pluginabi.MethodAuthLoginStart, pluginabi.MethodAuthLoginPoll, pluginabi.MethodAuthRefresh:
		// Claiming the codex identifier also claims these hooks. The host routes
		// Codex OAuth login to its built-in routes and never wraps the Codex
		// executor in the plugin refresh adapter, so they are unreachable today.
		// Fail loudly rather than silently returning empty credentials if that
		// ever changes.
		return errorEnvelope(
			"unsupported_method",
			pluginName+" does not implement "+method+"; it only rewrites the Codex upstream base URL while parsing auth files",
		), nil
	default:
		return errorEnvelope("unknown_method", "unknown method: "+method), nil
	}
}

func configure(raw []byte) error {
	baseURL := ""
	if len(raw) > 0 {
		var req lifecycleRequest
		if errUnmarshal := json.Unmarshal(raw, &req); errUnmarshal != nil {
			return errUnmarshal
		}
		if len(req.ConfigYAML) > 0 {
			var cfg pluginConfig
			if errDecode := yaml.Unmarshal(req.ConfigYAML, &cfg); errDecode != nil {
				return errDecode
			}
			baseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
		}
	}
	if reason := baseURLProblem(baseURL); reason != "" {
		// Keep the configured value so the panel and this log agree, but say why
		// requests will fail. The Management panel renders plugin descriptions, so
		// this is the only channel that can carry a correction back to the user.
		logWarning(pluginName+": configured base-url looks wrong", map[string]any{
			"base_url": baseURL,
			"reason":   reason,
		})
	}
	value := baseURL
	configuredBaseURL.Store(&value)
	return nil
}

// baseURLProblem reports why an upstream base URL cannot work, or "" when it is
// usable. Empty is valid: it means the plugin stays out of the way.
func baseURLProblem(baseURL string) string {
	if baseURL == "" {
		return ""
	}
	parsed, errParse := url.Parse(baseURL)
	if errParse != nil {
		return "not a valid URL: " + errParse.Error()
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "missing an http:// or https:// scheme"
	}
	if parsed.Host == "" {
		return "missing a host"
	}
	return ""
}

// logWarning reports a message through the host logger. It is best-effort: a
// plugin must never fail because logging is unavailable.
func logWarning(message string, fields map[string]any) {
	payload, errMarshal := json.Marshal(map[string]any{
		"level":   "warn",
		"message": message,
		"fields":  fields,
	})
	if errMarshal != nil {
		return
	}
	cMethod := C.CString(pluginabi.MethodHostLog)
	defer C.free(unsafe.Pointer(cMethod))
	request := (*C.uint8_t)(C.CBytes(payload))
	defer C.free(unsafe.Pointer(request))
	var response C.cliproxy_buffer
	if C.call_host_api(cMethod, request, C.size_t(len(payload)), &response) != 0 {
		return
	}
	if response.ptr != nil {
		C.free_host_buffer(response.ptr, response.len)
	}
}

func pluginRegistration() registration {
	return registration{
		SchemaVersion: pluginabi.SchemaVersion,
		Metadata: pluginapi.Metadata{
			Name:             pluginName,
			Version:          pluginVersion,
			Author:           "xinghaix",
			GitHubRepository: "https://github.com/xinghaix/CLIProxyAPI-Plugins-Store",
			Logo:             "",
			ConfigFields: []pluginapi.ConfigField{{
				Name:        "base-url",
				Type:        pluginapi.ConfigFieldTypeString,
				Description: "Upstream base URL applied to Codex OAuth credentials. Empty keeps the built-in default.",
			}},
		},
		Capabilities: registrationCapability{AuthProvider: true},
	}
}

func parseAuth(raw []byte) ([]byte, error) {
	var req pluginapi.AuthParseRequest
	if errUnmarshal := json.Unmarshal(raw, &req); errUnmarshal != nil {
		return nil, errUnmarshal
	}
	auth, handled := buildAuth(req)
	if !handled {
		// Handing the file back to the host keeps the built-in synthesizer in
		// charge, so an unconfigured plugin is a no-op.
		return okEnvelope(pluginapi.AuthParseResponse{Handled: false})
	}
	return okEnvelope(pluginapi.AuthParseResponse{Handled: true, Auth: auth})
}

// buildAuth mirrors the attributes the host's file synthesizer derives from a
// Codex OAuth auth file, and adds base_url. It returns handled=false for any
// payload this plugin must not own.
func buildAuth(req pluginapi.AuthParseRequest) (pluginapi.AuthData, bool) {
	metadata, okDecode := decodeAuthMetadata(req.RawJSON)
	if !okDecode {
		return pluginapi.AuthData{}, false
	}
	if !strings.EqualFold(metadataString(metadata, "type"), codexProviderKey) {
		return pluginapi.AuthData{}, false
	}
	if !isOAuthCredential(metadata) {
		return pluginapi.AuthData{}, false
	}
	baseURL := firstNonEmpty(metadataString(metadata, "base_url", "base-url"), currentBaseURL())
	if baseURL == "" {
		return pluginapi.AuthData{}, false
	}

	attributes := map[string]string{"base_url": baseURL}
	if planType := resolvePlanType(metadata); planType != "" {
		attributes["plan_type"] = planType
	}
	if priority := priorityAttribute(metadata); priority != "" {
		attributes["priority"] = priority
	}
	if note := metadataString(metadata, "note"); note != "" {
		attributes["note"] = note
	}

	return pluginapi.AuthData{
		Provider:   codexProviderKey,
		Label:      firstNonEmpty(metadataString(metadata, "email"), codexProviderKey),
		Metadata:   metadata,
		Attributes: attributes,
	}, true
}

func decodeAuthMetadata(raw []byte) (map[string]any, bool) {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil, false
	}
	var metadata map[string]any
	if errUnmarshal := json.Unmarshal(raw, &metadata); errUnmarshal != nil || metadata == nil {
		return nil, false
	}
	return metadata, true
}

// isOAuthCredential reports whether the file carries subscription tokens. Codex
// entries that only carry an API key belong to the codex-api-key config path and
// are left to the host.
func isOAuthCredential(metadata map[string]any) bool {
	for _, key := range []string{"access_token", "refresh_token", "id_token"} {
		if metadataString(metadata, key) != "" {
			return true
		}
	}
	return false
}

// resolvePlanType mirrors the host's Codex plan extraction: metadata first, then
// the id_token claim. Losing this would advertise the wrong plan's model catalog.
func resolvePlanType(metadata map[string]any) string {
	if planType := metadataString(metadata, "plan_type"); planType != "" {
		return planType
	}
	return planTypeFromIDToken(metadataString(metadata, "id_token"))
}

func planTypeFromIDToken(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ""
	}
	payload, errDecode := base64URLDecode(parts[1])
	if errDecode != nil {
		return ""
	}
	var claims map[string]any
	if errUnmarshal := json.Unmarshal(payload, &claims); errUnmarshal != nil {
		return ""
	}
	authInfo, okClaims := claims[codexAuthInfoClaim].(map[string]any)
	if !okClaims {
		return ""
	}
	return metadataString(authInfo, "chatgpt_plan_type")
}

func base64URLDecode(data string) ([]byte, error) {
	switch len(data) % 4 {
	case 2:
		data += "=="
	case 3:
		data += "="
	}
	return base64.URLEncoding.DecodeString(data)
}

// priorityAttribute mirrors the host's numeric-or-numeric-string priority read.
func priorityAttribute(metadata map[string]any) string {
	raw, ok := metadata["priority"]
	if !ok {
		return ""
	}
	switch value := raw.(type) {
	case float64:
		return strconv.Itoa(int(value))
	case string:
		trimmed := strings.TrimSpace(value)
		if _, errAtoi := strconv.Atoi(trimmed); errAtoi == nil {
			return trimmed
		}
	}
	return ""
}

// metadataString returns the first non-empty trimmed string value among keys.
func metadataString(metadata map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := metadata[key].(string)
		if !ok {
			continue
		}
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func currentBaseURL() string {
	pointer := configuredBaseURL.Load()
	if pointer == nil {
		return ""
	}
	return strings.TrimSpace(*pointer)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func okEnvelope(v any) ([]byte, error) {
	raw, errMarshal := json.Marshal(v)
	if errMarshal != nil {
		return nil, errMarshal
	}
	return json.Marshal(envelope{OK: true, Result: raw})
}

func errorEnvelope(code, message string) []byte {
	raw, _ := json.Marshal(envelope{OK: false, Error: &envelopeError{Code: code, Message: message}})
	return raw
}

func writeResponse(response *C.cliproxy_buffer, raw []byte) {
	if response == nil || len(raw) == 0 {
		return
	}
	ptr := C.CBytes(raw)
	if ptr == nil {
		return
	}
	response.ptr = ptr
	response.len = C.size_t(len(raw))
}
