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
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync/atomic"
	"unsafe"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
	"gopkg.in/yaml.v3"
)

const (
	defaultManagerBaseURL   = "http://127.0.0.1:18317"
	managementHealthPathRel = "/cpa-manager-plus/health"
	managementProxyPathRel  = "/cpa-manager-plus/proxy"
	managementHealthPathAbs = "/v0/management/cpa-manager-plus/health"
	managementProxyPathAbs  = "/v0/management/cpa-manager-plus/proxy"
	resourceAppPath         = "/v0/resource/plugins/cpa-manager-plus/app"
	contentTypeJSON         = "application/json; charset=utf-8"
	contentTypeHTML         = "text/html; charset=utf-8"
	maxProxyBodyBytes       = 8 << 20
)

var pluginVersion = "0.2.1"

var activeConfig atomic.Value

func init() {
	activeConfig.Store(defaultPluginConfig())
}

func main() {}

type envelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *envelopeError  `json:"error,omitempty"`
}

type envelopeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type lifecycleRequest struct {
	ConfigYAML []byte `json:"config_yaml"`
}

type pluginConfig struct {
	ManagerBaseURL string `yaml:"manager_base_url"`
	ManagementKey  string `yaml:"management_key"`
	AdminKey       string `yaml:"admin_key"` // deprecated: use management_key
}

type registration struct {
	SchemaVersion uint32                   `json:"schema_version"`
	Metadata      pluginapi.Metadata       `json:"metadata"`
	Capabilities  registrationCapabilities `json:"capabilities"`
}

type registrationCapabilities struct {
	ManagementAPI bool `json:"management_api"`
}

type managementRegistrationResponse struct {
	Routes    []pluginapi.ManagementRoute `json:"routes,omitempty"`
	Resources []pluginapi.ResourceRoute   `json:"resources,omitempty"`
}

type managementRequest struct {
	pluginapi.ManagementRequest
	HostCallbackID string `json:"host_callback_id,omitempty"`
}

type managementResponse struct {
	StatusCode int         `json:"StatusCode"`
	Headers    http.Header `json:"Headers"`
	Body       []byte      `json:"Body"`
}

type proxyRequest struct {
	Method string          `json:"method"`
	Path   string          `json:"path"`
	Query  string          `json:"query"`
	Body   json.RawMessage `json:"body"`
}

type healthResponse struct {
	OK             bool   `json:"ok"`
	ManagerBaseURL string `json:"manager_base_url,omitempty"`
	ManagerStatus  int    `json:"manager_status,omitempty"`
	Error          string `json:"error,omitempty"`
}

type hostHTTPResult struct {
	StatusCode int
	Headers    map[string][]string
	Body       []byte
}

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
	case pluginabi.MethodManagementRegister:
		return okEnvelope(managementRegistrationResponse{
			Routes: []pluginapi.ManagementRoute{
				{Method: http.MethodGet, Path: "/cpa-manager-plus/health"},
				{Method: http.MethodPost, Path: "/cpa-manager-plus/proxy"},
			},
			Resources: []pluginapi.ResourceRoute{{
				Path:        "/app",
				Menu:        "CPA Manager Plus",
				Description: "Manager Plus 仪表盘 / 用量 / 监控 / 巡检（经插件反向代理 Manager Server）",
			}},
		})
	case pluginabi.MethodManagementHandle:
		return handleManagement(request)
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

func defaultPluginConfig() pluginConfig {
	return pluginConfig{ManagerBaseURL: defaultManagerBaseURL}
}

func mergeConfig(base, override pluginConfig) pluginConfig {
	if strings.TrimSpace(override.ManagerBaseURL) != "" {
		base.ManagerBaseURL = override.ManagerBaseURL
	}
	if strings.TrimSpace(override.ManagementKey) != "" {
		base.ManagementKey = override.ManagementKey
	}
	if strings.TrimSpace(override.AdminKey) != "" && base.ManagementKey == "" {
		base.ManagementKey = override.AdminKey
	}
	return base
}

func normalizeConfig(cfg pluginConfig) pluginConfig {
	cfg.ManagerBaseURL = strings.TrimRight(strings.TrimSpace(cfg.ManagerBaseURL), "/")
	if cfg.ManagerBaseURL == "" {
		cfg.ManagerBaseURL = defaultManagerBaseURL
	}
	cfg.ManagementKey = strings.TrimSpace(cfg.ManagementKey)
	if cfg.ManagementKey == "" {
		cfg.ManagementKey = strings.TrimSpace(cfg.AdminKey)
	}
	cfg.AdminKey = ""
	return cfg
}

func currentConfig() pluginConfig {
	raw := activeConfig.Load()
	if cfg, ok := raw.(pluginConfig); ok {
		return cfg
	}
	return defaultPluginConfig()
}

func pluginRegistration() registration {
	return registration{
		SchemaVersion: pluginabi.SchemaVersion,
		Metadata: pluginapi.Metadata{
			Name:             "CPA Manager Plus",
			Version:          pluginVersion,
			Author:           "xinghaix",
			GitHubRepository: "https://github.com/xinghaix/CLIProxyAPI-Plugins-Store",
			ConfigFields: []pluginapi.ConfigField{
				{Name: "manager_base_url", Type: pluginapi.ConfigFieldTypeString, Description: "Manager Server base URL (default http://127.0.0.1:18317)"},
				{Name: "management_key", Type: pluginapi.ConfigFieldTypeString, Description: "Manager Plus admin Bearer token for proxy to Manager Server (optional if Manager allows unauthenticated local access)"},
			},
		},
		Capabilities: registrationCapabilities{ManagementAPI: true},
	}
}

func handleManagement(raw []byte) ([]byte, error) {
	var req managementRequest
	if len(raw) > 0 {
		if errUnmarshal := json.Unmarshal(raw, &req); errUnmarshal != nil {
			return nil, errUnmarshal
		}
	}
	path := strings.TrimRight(strings.TrimSpace(req.Path), "/")
	if path == "" {
		path = "/"
	}
	isHealth := path == managementHealthPathRel || path == managementHealthPathAbs
	isProxy := path == managementProxyPathRel || path == managementProxyPathAbs

	switch {
	case strings.EqualFold(req.Method, http.MethodGet) && strings.HasPrefix(path, "/v0/resource/plugins/cpa-manager-plus"):
		return okEnvelope(handleResource(path))
	case strings.EqualFold(req.Method, http.MethodGet) && isHealth:
		return okEnvelope(handleHealth(req.ManagementRequest))
	case strings.EqualFold(req.Method, http.MethodPost) && isProxy:
		return okEnvelope(handleProxy(req.ManagementRequest))
	default:
		return okEnvelope(jsonResponse(http.StatusNotFound, map[string]any{"error": "plugin route not found", "path": path}))
	}
}

func handleResource(requestPath string) managementResponse {
	filePath, ok := resourceFileForPath(requestPath)
	if !ok {
		return jsonResponse(http.StatusNotFound, map[string]any{"error": "resource not found", "path": requestPath})
	}
	body, errRead := embeddedWebFS.ReadFile(filePath)
	if errRead != nil {
		return jsonResponse(http.StatusNotFound, map[string]any{"error": "resource not found", "path": requestPath})
	}
	return managementResponse{
		StatusCode: http.StatusOK,
		Headers:    http.Header{"Content-Type": []string{contentTypeForResourceFile(filePath)}},
		Body:       append([]byte(nil), body...),
	}
}

func resourceFileForPath(requestPath string) (string, bool) {
	cleaned := path.Clean("/" + strings.TrimSpace(requestPath))
	if cleaned == resourceAppPath || cleaned == "/v0/resource/plugins/cpa-manager-plus/app/" {
		return "web-dist/index.html", true
	}
	return "", false
}

func contentTypeForResourceFile(filePath string) string {
	return contentTypeHTML
}

func handleHealth(req pluginapi.ManagementRequest) managementResponse {
	cfg := currentConfig()
	status, _, _, err := proxyToManager(cfg, req, http.MethodGet, "/health", "", nil)
	out := healthResponse{ManagerBaseURL: cfg.ManagerBaseURL, ManagerStatus: status}
	if err != nil {
		out.Error = err.Error()
		return jsonResponse(http.StatusBadGateway, out)
	}
	out.OK = status >= 200 && status < 300
	if !out.OK {
		out.Error = fmt.Sprintf("manager health status %d", status)
		return jsonResponse(http.StatusBadGateway, out)
	}
	return jsonResponse(http.StatusOK, out)
}

func handleProxy(req pluginapi.ManagementRequest) managementResponse {
	if len(req.Body) > maxProxyBodyBytes {
		return jsonResponse(http.StatusRequestEntityTooLarge, map[string]any{"error": "body too large"})
	}
	var payload proxyRequest
	if len(req.Body) > 0 {
		if errUnmarshal := json.Unmarshal(req.Body, &payload); errUnmarshal != nil {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
		}
	}
	method := strings.ToUpper(strings.TrimSpace(payload.Method))
	if method == "" {
		method = http.MethodGet
	}
	path := strings.TrimSpace(payload.Path)
	if path == "" || !strings.HasPrefix(path, "/") {
		return jsonResponse(http.StatusBadRequest, map[string]any{"error": "path must start with /"})
	}
	var body []byte
	if len(payload.Body) > 0 && string(payload.Body) != "null" {
		body = append([]byte(nil), payload.Body...)
	}
	if errValidate := validateProxyTarget(method, path); errValidate != nil {
		return jsonResponse(http.StatusForbidden, map[string]any{"error": errValidate.Error()})
	}
	cfg := currentConfig()
	status, respHeaders, respBody, err := proxyToManager(cfg, req, method, path, payload.Query, body)
	if err != nil {
		return jsonResponse(http.StatusBadGateway, map[string]any{"error": err.Error()})
	}
	return managementResponse{
		StatusCode: status,
		Headers:    respHeaders,
		Body:       respBody,
	}
}

func proxyToManager(cfg pluginConfig, req pluginapi.ManagementRequest, method, path, query string, body []byte) (int, http.Header, []byte, error) {
	if errValidate := validateProxyTarget(method, path); errValidate != nil {
		return 0, nil, nil, errValidate
	}
	target, errURL := buildManagerURL(cfg.ManagerBaseURL, path, query)
	if errURL != nil {
		return 0, nil, nil, errURL
	}
	headers := http.Header{}
	headers.Set("Accept", "application/json")
	if auth := managerAuthorization(cfg, req); auth != "" {
		headers.Set("Authorization", auth)
	}
	if len(body) > 0 {
		headers.Set("Content-Type", "application/json")
	}
	resp, errDo := callHostHTTP(method, target, headers, body)
	if errDo != nil {
		return 0, nil, nil, errDo
	}
	return resp.StatusCode, proxyResponseHeaders(resp.Headers), resp.Body, nil
}

func validateProxyTarget(method, path string) error {
	method = strings.ToUpper(strings.TrimSpace(method))
	if _, ok := allowedProxyMethods[method]; !ok {
		return fmt.Errorf("method %s is not allowed", method)
	}
	path = strings.TrimRight(strings.TrimSpace(path), "/")
	if path == "" {
		path = "/"
	}
	for _, rule := range allowedProxyPathRules {
		if rule(path) {
			return nil
		}
	}
	return fmt.Errorf("manager path %s is not allowed", path)
}

var allowedProxyMethods = map[string]struct{}{
	http.MethodGet:    {},
	http.MethodPost:   {},
	http.MethodPut:    {},
	http.MethodPatch:  {},
	http.MethodDelete: {},
}

var allowedProxyPathRules = []func(string) bool{
	exactPath("/health"),
	exactPath("/status"),
	prefixPath("/usage-service/"),
	exactPath("/v0/management/dashboard/summary"),
	exactPath("/v0/management/usage"),
	prefixPath("/v0/management/usage/"),
	exactPath("/v0/management/model-prices"),
	prefixPath("/v0/management/model-prices/"),
	exactPath("/v0/management/api-key-aliases"),
	prefixPath("/v0/management/api-key-aliases/"),
	exactPath("/v0/management/monitoring/header-snapshots"),
	exactPath("/v0/management/monitoring/analytics"),
	exactPath("/v0/management/codex-inspection/run"),
	exactPath("/v0/management/codex-inspection/runs"),
	prefixPath("/v0/management/codex-inspection/runs/"),
}

func exactPath(want string) func(string) bool {
	return func(path string) bool { return path == want }
}

func prefixPath(prefix string) func(string) bool {
	return func(path string) bool { return strings.HasPrefix(path, prefix) }
}

func proxyResponseHeaders(upstream map[string][]string) http.Header {
	headers := http.Header{}
	for key, values := range upstream {
		if strings.EqualFold(key, "Content-Type") {
			for _, value := range values {
				if strings.TrimSpace(value) != "" {
					headers.Add("Content-Type", value)
				}
			}
		}
	}
	if headers.Get("Content-Type") == "" {
		headers.Set("Content-Type", contentTypeJSON)
	}
	return headers
}

func managerAuthorization(cfg pluginConfig, _ pluginapi.ManagementRequest) string {
	key := strings.TrimSpace(cfg.ManagementKey)
	if key == "" {
		return ""
	}
	if !strings.HasPrefix(strings.ToLower(key), "bearer ") {
		key = "Bearer " + key
	}
	return key
}

func buildManagerURL(base, path, query string) (string, error) {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	u, errParse := url.Parse(base + path)
	if errParse != nil {
		return "", errParse
	}
	if strings.TrimSpace(query) != "" {
		q, errQ := url.ParseQuery(query)
		if errQ != nil {
			return "", fmt.Errorf("invalid query: %w", errQ)
		}
		u.RawQuery = q.Encode()
	}
	return u.String(), nil
}

func callHostHTTP(method, target string, headers http.Header, body []byte) (hostHTTPResult, error) {
	h := map[string][]string(headers)
	payload := map[string]any{
		"method":  method,
		"url":     target,
		"headers": h,
		"body":    body,
	}
	result, errCall := callHost(pluginabi.MethodHostHTTPDo, payload)
	if errCall != nil {
		return hostHTTPResult{}, errCall
	}
	var resp hostHTTPResult
	if errUnmarshal := json.Unmarshal(result, &resp); errUnmarshal != nil {
		return hostHTTPResult{}, fmt.Errorf("decode host.http.do: %w", errUnmarshal)
	}
	return resp, nil
}

func callHost(method string, payload any) (json.RawMessage, error) {
	rawPayload, errMarshal := json.Marshal(payload)
	if errMarshal != nil {
		return nil, errMarshal
	}
	cMethod := C.CString(method)
	defer C.free(unsafe.Pointer(cMethod))
	var response C.cliproxy_buffer
	var requestPtr *C.uint8_t
	if len(rawPayload) > 0 {
		cPayload := C.CBytes(rawPayload)
		if cPayload == nil {
			return nil, fmt.Errorf("allocate host callback payload %s", method)
		}
		defer C.free(cPayload)
		requestPtr = (*C.uint8_t)(cPayload)
	}
	callCode := C.call_host_api(cMethod, requestPtr, C.size_t(len(rawPayload)), &response)
	var rawResponse []byte
	if response.ptr != nil && response.len > 0 {
		rawResponse = C.GoBytes(response.ptr, C.int(response.len))
	}
	if response.ptr != nil {
		C.free_host_buffer(response.ptr, response.len)
	}
	if len(rawResponse) == 0 {
		return nil, fmt.Errorf("host callback %s returned no response, code=%d", method, int(callCode))
	}
	var env envelope
	if errUnmarshal := json.Unmarshal(rawResponse, &env); errUnmarshal != nil {
		return nil, fmt.Errorf("decode host envelope %s: %w", method, errUnmarshal)
	}
	if !env.OK {
		if env.Error != nil {
			return nil, fmt.Errorf("%s: %s", env.Error.Code, env.Error.Message)
		}
		return nil, fmt.Errorf("host callback %s failed", method)
	}
	if callCode != 0 {
		return nil, fmt.Errorf("host callback %s code=%d", method, int(callCode))
	}
	return append(json.RawMessage(nil), env.Result...), nil
}

func htmlResponse(statusCode int, body []byte) managementResponse {
	return managementResponse{
		StatusCode: statusCode,
		Headers:    http.Header{"Content-Type": []string{contentTypeHTML}},
		Body:       body,
	}
}

func jsonResponse(statusCode int, payload any) managementResponse {
	raw, errMarshal := json.Marshal(payload)
	if errMarshal != nil {
		statusCode = http.StatusInternalServerError
		raw = []byte(`{"error":"marshal failed"}`)
	}
	return managementResponse{
		StatusCode: statusCode,
		Headers:    http.Header{"Content-Type": []string{contentTypeJSON}},
		Body:       raw,
	}
}

func okEnvelope(result any) ([]byte, error) {
	raw, errMarshal := json.Marshal(envelope{OK: true, Result: mustRaw(result)})
	if errMarshal != nil {
		return nil, errMarshal
	}
	return raw, nil
}

func mustRaw(v any) json.RawMessage {
	switch t := v.(type) {
	case json.RawMessage:
		return t
	case managementResponse:
		raw, _ := json.Marshal(t)
		return raw
	default:
		raw, _ := json.Marshal(v)
		return raw
	}
}

func errorEnvelope(code, message string) []byte {
	raw, _ := json.Marshal(envelope{OK: false, Error: &envelopeError{Code: code, Message: message}})
	return raw
}

func writeResponse(response *C.cliproxy_buffer, data []byte) {
	if response == nil || len(data) == 0 {
		return
	}
	ptr := C.CBytes(data)
	if ptr == nil {
		return
	}
	response.ptr = ptr
	response.len = C.size_t(len(data))
}

var _ = bytes.Reader{}
