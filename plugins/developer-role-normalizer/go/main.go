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
	"gopkg.in/yaml.v3"
)

const (
	defaultMatchMode = "contains"
	defaultStrategy  = "role_to_system"
)

var pluginVersion = "0.3.0"

var activeConfig atomic.Value

func init() {
	activeConfig.Store(defaultPluginConfig())
}

func main() {}

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
	// Enabled is kept as a backwards-compatible alias for normalize_enabled.
	// CPA also stores the host-managed plugin switch at this key; when present,
	// false disables normalization and true leaves it enabled.
	Enabled          *bool            `yaml:"enabled"`
	NormalizeEnabled *bool            `yaml:"normalize_enabled"`
	TargetFormats    configurableList `yaml:"target_formats"`
	ModelMatch       modelMatchConfig `yaml:"model_match"`
	Strategy         string           `yaml:"strategy"`
}

type normalizerConfig struct {
	NormalizeEnabled bool
	TargetFormats    []string
	ModelMatch       modelMatchRule
	Strategy         string
}

type modelMatchConfig struct {
	Mode    string           `yaml:"mode"`
	Include configurableList `yaml:"include"`
	Exclude configurableList `yaml:"exclude"`
}

type modelMatchRule struct {
	Mode    string
	Include []string
	Exclude []string
}

type configurableList struct {
	Set    bool
	Values []string
}

func (l *configurableList) UnmarshalYAML(node *yaml.Node) error {
	l.Set = true
	l.Values = nil
	switch node.Kind {
	case yaml.SequenceNode:
		for _, item := range node.Content {
			var value string
			if errDecode := item.Decode(&value); errDecode != nil {
				return errDecode
			}
			l.Values = append(l.Values, splitConfigListValue(value)...)
		}
	case yaml.ScalarNode:
		var value string
		if errDecode := node.Decode(&value); errDecode != nil {
			return errDecode
		}
		l.Values = splitConfigListValue(value)
	case yaml.MappingNode:
		var values []string
		if errDecode := node.Decode(&values); errDecode != nil {
			return errDecode
		}
		l.Values = values
	default:
		return nil
	}
	return nil
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
func cliproxyPluginFree(ptr unsafe.Pointer, _ C.size_t) {
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

	cfg := defaultPluginConfig()
	if len(req.ConfigYAML) > 0 {
		var decoded pluginConfig
		if errUnmarshal := yaml.Unmarshal(req.ConfigYAML, &decoded); errUnmarshal != nil {
			return errUnmarshal
		}
		cfg = mergeConfig(cfg, decoded)
	}
	activeConfig.Store(normalizeConfig(cfg))
	return nil
}

func defaultPluginConfig() normalizerConfig {
	return normalizerConfig{
		NormalizeEnabled: true,
		TargetFormats:    []string{"openai", "codex"},
		ModelMatch: modelMatchRule{
			Mode:    defaultMatchMode,
			Include: []string{"deepseek"},
			Exclude: nil,
		},
		Strategy: defaultStrategy,
	}
}

func mergeConfig(base normalizerConfig, override pluginConfig) normalizerConfig {
	if override.Enabled != nil {
		base.NormalizeEnabled = *override.Enabled
	}
	if override.NormalizeEnabled != nil {
		base.NormalizeEnabled = *override.NormalizeEnabled
	}
	if override.TargetFormats.Set {
		base.TargetFormats = override.TargetFormats.Values
	}
	if strings.TrimSpace(override.ModelMatch.Mode) != "" {
		base.ModelMatch.Mode = override.ModelMatch.Mode
	}
	if override.ModelMatch.Include.Set {
		base.ModelMatch.Include = override.ModelMatch.Include.Values
	}
	if override.ModelMatch.Exclude.Set {
		base.ModelMatch.Exclude = override.ModelMatch.Exclude.Values
	}
	if strings.TrimSpace(override.Strategy) != "" {
		base.Strategy = override.Strategy
	}
	return base
}

func normalizeConfig(cfg normalizerConfig) normalizerConfig {
	cfg.TargetFormats = normalizeConfigList(cfg.TargetFormats, true)
	if len(cfg.TargetFormats) == 0 {
		cfg.TargetFormats = []string{"openai", "codex"}
	}

	cfg.ModelMatch.Mode = normalizeMatchMode(cfg.ModelMatch.Mode)
	cfg.ModelMatch.Include = normalizeConfigList(cfg.ModelMatch.Include, true)
	cfg.ModelMatch.Exclude = normalizeConfigList(cfg.ModelMatch.Exclude, true)

	strategy := strings.ToLower(strings.TrimSpace(cfg.Strategy))
	if strategy != defaultStrategy {
		strategy = defaultStrategy
	}
	cfg.Strategy = strategy
	return cfg
}

func currentConfig() normalizerConfig {
	raw := activeConfig.Load()
	if cfg, ok := raw.(normalizerConfig); ok {
		return cfg
	}
	return defaultPluginConfig()
}

func pluginRegistration() registration {
	return registration{
		SchemaVersion: pluginabi.SchemaVersion,
		Metadata: pluginapi.Metadata{
			Name:             "developer-role-normalizer",
			Version:          pluginVersion,
			Author:           "xinghaix",
			GitHubRepository: "https://github.com/xinghaix/CLIProxyAPI-Plugins-Store",
			ConfigFields: []pluginapi.ConfigField{
				{
					Name:        "normalize_enabled",
					Type:        pluginapi.ConfigFieldTypeBoolean,
					Description: "When true, normalize developer messages for matched target models. Default: true.",
				},
				{
					Name:        "target_formats",
					Type:        pluginapi.ConfigFieldTypeArray,
					Description: "Target formats to normalize. Default: [openai, codex].",
				},
				{
					Name:        "model_match",
					Type:        pluginapi.ConfigFieldTypeObject,
					Description: "Model matching rule, for example {mode: contains, include: [deepseek], exclude: []}. Matching is case-insensitive. Default only matches models containing deepseek.",
				},
				{
					Name:        "strategy",
					Type:        pluginapi.ConfigFieldTypeEnum,
					EnumValues:  []string{defaultStrategy},
					Description: "How to rewrite developer messages. role_to_system converts developer role to system.",
				},
			},
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
// By default it only acts on OpenAI-compatible targets whose model identifier
// contains "deepseek", because those provider models do not support the
// developer role in Chat Completions-compatible payloads.
func shouldNormalize(req pluginapi.RequestTransformRequest) bool {
	cfg := currentConfig()
	if !cfg.NormalizeEnabled {
		return false
	}
	if cfg.Strategy != defaultStrategy {
		return false
	}
	if !matchesAny("exact", strings.ToLower(strings.TrimSpace(req.ToFormat)), cfg.TargetFormats) {
		return false
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = gjson.GetBytes(req.Body, "model").String()
	}
	return matchesModel(cfg.ModelMatch, model)
}

func matchesModel(rule modelMatchRule, model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	if len(rule.Exclude) > 0 && matchesAny(rule.Mode, model, rule.Exclude) {
		return false
	}
	if len(rule.Include) == 0 {
		return true
	}
	return matchesAny(rule.Mode, model, rule.Include)
}

func matchesAny(mode, value string, patterns []string) bool {
	if strings.TrimSpace(value) == "" {
		return false
	}
	for _, pattern := range patterns {
		if matchesPattern(mode, value, pattern) {
			return true
		}
	}
	return false
}

func matchesPattern(mode, value, pattern string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	if value == "" || pattern == "" {
		return false
	}
	if pattern == "*" {
		return true
	}
	switch normalizeMatchMode(mode) {
	case "exact":
		return value == pattern
	case "prefix":
		return strings.HasPrefix(value, pattern)
	case "suffix":
		return strings.HasSuffix(value, pattern)
	default:
		return strings.Contains(value, pattern)
	}
}

func normalizeMatchMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "exact", "prefix", "suffix", "contains":
		return strings.ToLower(strings.TrimSpace(mode))
	default:
		return defaultMatchMode
	}
}

func splitConfigListValue(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func normalizeConfigList(values []string, lower bool) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		for _, part := range splitConfigListValue(value) {
			if lower {
				part = strings.ToLower(part)
			}
			if _, ok := seen[part]; ok {
				continue
			}
			seen[part] = struct{}{}
			out = append(out, part)
		}
	}
	return out
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
