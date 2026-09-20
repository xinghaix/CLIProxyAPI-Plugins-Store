package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cursor-oauth/go/internal/cursorapi"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cursor-oauth/go/internal/cursorauth"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cursor-oauth/go/internal/cursorsession"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cursor-oauth/go/internal/openai"
)

type Handler struct {
	auth     *cursorauth.Service
	cursor   CursorClient
	emitter  StreamEmitter
	host     HostCaller
	usage    *usageStore
	sessions *cursorsession.Store
	turns    *sessionTurnLocks
}

func NewHandler(dependencies Dependencies) *Handler {
	if dependencies.Auth == nil {
		dependencies.Auth = cursorauth.NewService(http.DefaultClient, cursorauth.DefaultEndpoints())
	}
	if dependencies.Cursor == nil {
		client, err := cursorapi.NewClient(cursorapi.Config{BaseURL: "https://api2.cursor.sh"})
		if err == nil {
			dependencies.Cursor = client
		}
	}
	return &Handler{
		auth: dependencies.Auth, cursor: dependencies.Cursor, emitter: dependencies.Emitter,
		host: dependencies.Host, usage: newUsageStore(), sessions: cursorsession.NewStore(cursorsession.Options{}),
		turns: newSessionTurnLocks(),
	}
}

func (handler *Handler) Call(ctx context.Context, method string, request []byte) []byte {
	raw, _ := handler.CallWithStatus(ctx, method, request)
	return raw
}

func (handler *Handler) CallWithStatus(ctx context.Context, method string, request []byte) ([]byte, bool) {
	result, err := handler.dispatch(ctx, method, request)
	if err != nil {
		httpStatus := 0
		code := "cursor_plugin_error"
		if errors.Is(err, openai.ErrInvalidRequest) {
			httpStatus = http.StatusBadRequest
		}
		var loop *openai.ToolLoopError
		if errors.As(err, &loop) {
			code = "cursor_tool_loop_detected"
		}
		return marshalEnvelope(envelope{OK: false, Error: &envelopeError{
			Code:       code,
			Message:    err.Error(),
			HTTPStatus: httpStatus,
		}}), false
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return marshalEnvelope(envelope{OK: false, Error: &envelopeError{Code: "cursor_plugin_error", Message: "encode plugin result"}}), false
	}
	return marshalEnvelope(envelope{OK: true, Result: raw}), true
}

func (handler *Handler) dispatch(ctx context.Context, method string, request []byte) (any, error) {
	switch method {
	case "plugin.register", "plugin.reconfigure":
		return registration(), nil
	case "management.register":
		return managementRegistration(), nil
	case "management.handle":
		return handler.handleManagement(ctx, request)
	case "request.intercept_before":
		return handler.passRequest(request)
	case "request.intercept_after":
		return handler.observeRequestAuth(request)
	case "request.complete":
		return handler.completeRequest(request)
	case "auth.identifier", "executor.identifier":
		return map[string]string{"identifier": "cursor"}, nil
	case "auth.parse":
		return handler.parseAuth(request)
	case "auth.login.start":
		return handler.startLogin(ctx)
	case "auth.login.poll":
		return handler.pollLogin(ctx, request)
	case "auth.refresh":
		return handler.refreshAuth(ctx, request)
	case "model.static":
		return modelResponse([]string{"auto"}), nil
	case "model.for_auth":
		return handler.modelsForAuth(ctx, request)
	case "executor.execute":
		return handler.execute(ctx, request)
	case "executor.execute_stream":
		return handler.executeStream(ctx, request)
	case "executor.count_tokens":
		return countTokens(request)
	case "executor.http_request":
		return nil, errors.New("Cursor plugin HTTP passthrough is unsupported")
	default:
		return nil, errors.New("unknown plugin method")
	}
}

// pluginLogoURL is the display icon shown by management clients (CPA Manager Plus).
const pluginLogoURL = "https://ptht05hbb1ssoooe.public.blob.vercel-storage.com/assets/brand/brand-logo-5.svg"

// PluginVersion is the released plugin version. It can be overridden at runtime by main.
var PluginVersion = "0.6.1"

func registration() map[string]any {
	return map[string]any{
		"schema_version": 3,
		"metadata": map[string]any{
			"Name":             "cursor-oauth",
			"Version":          PluginVersion,
			"Author":           "kilolonion",
			"GitHubRepository": "https://github.com/xinghaix/CLIProxyAPI-Plugins-Store",
			"Logo":             pluginLogoURL,
			"ConfigFields":     []string{},
		},
		"capabilities": map[string]any{
			"auth_provider":            true,
			"model_provider":           true,
			"management_api":           true,
			"request_interceptor":      true,
			"request_lifecycle_plugin": true,
			"usage_plugin":             false,
			"executor":                 true,
			"executor_model_scope":     "oauth",
			"executor_input_formats":   []string{"chat-completions"},
			"executor_output_formats":  []string{"chat-completions"},
		},
	}
}

func marshalEnvelope(response envelope) []byte {
	raw, err := json.Marshal(response)
	if err != nil {
		return []byte(`{"ok":false,"error":{"code":"cursor_plugin_error","message":"encode envelope"}}`)
	}
	return raw
}
