package main

/*
#include <stdint.h>
#include <stdlib.h>

typedef struct {
	void* ptr;
	size_t len;
} cliproxy_buffer;

typedef struct {
	uint32_t abi_version;
	void* host_ctx;
	void* call;
	void* free_buffer;
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
*/
import "C"

import (
	"encoding/json"
	"strings"
	"sync/atomic"
	"unsafe"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"gopkg.in/yaml.v3"
)

// enabled controls whether the developer-to-system role normalization is active.
var enabled atomic.Bool

// --- Envelope types ---

type envelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *envelopeError  `json:"error,omitempty"`
}

type envelopeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// --- Lifecycle types ---

type lifecycleRequest struct {
	ConfigYAML []byte `json:"config_yaml"`
}

type pluginConfig struct {
	Enabled bool `yaml:"enabled"`
}

// --- Registration types ---

type registration struct {
	SchemaVersion uint32                 `json:"schema_version"`
	Metadata      pluginapi.Metadata     `json:"metadata"`
	Capabilities  registrationCapability `json:"capabilities"`
}

type registrationCapability struct {
	RequestNormalizer bool `json:"request_normalizer"`
}

var pluginVersion = "0.1.5"

func main() {}

//export cliproxy_plugin_init
func cliproxy_plugin_init(_ *C.cliproxy_host_api, plugin *C.cliproxy_plugin_api) C.int {
	if plugin == nil {
		return 1
	}
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
	case pluginabi.MethodRequestNormalize:
		return normalizeRequest(request)
	default:
		return errorEnvelope("unknown_method", "unknown method: "+method), nil
	}
}

func configure(raw []byte) error {
	var req lifecycleRequest
	if len(raw) > 0 {
		if errUnmarshal := json.Unmarshal(raw, &req); errUnmarshal != nil {
			return errUnmarshal
		}
	}

	cfg := pluginConfig{}
	if len(req.ConfigYAML) > 0 {
		if errUnmarshal := yaml.Unmarshal(req.ConfigYAML, &cfg); errUnmarshal != nil {
			return errUnmarshal
		}
	}
	// The host sets plugins.configs.<id>.enabled, but the plugin can also
	// use its own "enabled" field for fine-grained control. Either being
	// true means normalization is active.
	enabled.Store(cfg.Enabled)
	return nil
}

func pluginRegistration() registration {
	return registration{
		SchemaVersion: pluginabi.SchemaVersion,
		Metadata: pluginapi.Metadata{
			Name:             "developer-role-normalizer",
			Version:          pluginVersion,
			Author:           "xinghaix",
			GitHubRepository: "https://github.com/xinghaix/CLIProxyAPI",
			ConfigFields: []pluginapi.ConfigField{{
				Name:        "enabled",
				Type:        pluginapi.ConfigFieldTypeBoolean,
				Description: "When true, convert developer message roles to system before sending to OpenAI-compatible providers.",
			}},
		},
		Capabilities: registrationCapability{
			RequestNormalizer: true,
		},
	}
}

func normalizeRequest(raw []byte) ([]byte, error) {
	var req pluginapi.RequestTransformRequest
	if errUnmarshal := json.Unmarshal(raw, &req); errUnmarshal != nil {
		return nil, errUnmarshal
	}
	body := req.Body
	if !shouldNormalize(req) {
		return okEnvelope(pluginapi.PayloadResponse{Body: body})
	}
	normalized := normalizeDeveloperRole(body)
	if normalized == nil {
		return okEnvelope(pluginapi.PayloadResponse{Body: body})
	}
	return okEnvelope(pluginapi.PayloadResponse{Body: normalized})
}

// shouldNormalize decides whether to apply the developer-to-system conversion.
// The plugin only acts on OpenAI-compatible target formats where the provider
// may not recognize the "developer" role.
func shouldNormalize(req pluginapi.RequestTransformRequest) bool {
	if !enabled.Load() {
		return false
	}
	// Apply when the target format is openai or codex (OpenAI-compatible protocols).
	to := strings.ToLower(strings.TrimSpace(req.ToFormat))
	if to == "openai" || to == "codex" {
		return true
	}
	return false
}

// normalizeDeveloperRole converts any "developer" message role to "system"
// in the JSON payload's "messages" array. It uses gjson to locate role
// values and performs a single-pass byte copy with replacements.
func normalizeDeveloperRole(payload []byte) []byte {
	messagesResult := gjson.GetBytes(payload, "messages")
	if !messagesResult.IsArray() {
		return nil
	}

	type roleReplace struct{ idx int }
	var replacements []roleReplace
	messagesArr := messagesResult.Array()
	for i := range messagesArr {
		role := messagesArr[i].Get("role")
		if role.String() != "developer" {
			continue
		}
		replacements = append(replacements, roleReplace{idx: role.Index})
	}
	if len(replacements) == 0 {
		return nil
	}

	const oldMark, newMark = `"developer"`, `"system"`
	const delta = len(newMark) - len(oldMark) // -3
	newSize := len(payload) + delta*len(replacements)

	out := make([]byte, newSize)
	dst := out
	processedUpTo := 0

	for _, r := range replacements {
		n := copy(dst, payload[processedUpTo:r.idx])
		dst = dst[n:]
		n = copy(dst, newMark)
		dst = dst[n:]
		processedUpTo = r.idx + len(oldMark)
	}
	copy(dst, payload[processedUpTo:])
	return out
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

// Ensure sjson and gjson imports are used even when the build configuration
// varies. sjson is available for future header injection extensions.
var _ = sjson.Set
