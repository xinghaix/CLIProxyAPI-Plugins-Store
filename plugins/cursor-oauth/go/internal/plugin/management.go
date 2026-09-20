package plugin

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

const (
	managementStatusPath   = "/plugins/cursor/status"
	managementDisabledPath = "/plugins/cursor/disabled-models"
)

type managementRegistrationResponse struct {
	Routes    []managementRoute    `json:"routes"`
	Resources []managementResource `json:"resources"`
}

type managementRoute struct {
	Method string `json:"Method"`
	Path   string `json:"Path"`
}

type managementResource struct {
	Path        string `json:"Path"`
	Menu        string `json:"Menu"`
	Description string `json:"Description"`
}

type managementRequest struct {
	Method string      `json:"Method"`
	Path   string      `json:"Path"`
	Body   []byte      `json:"Body"`
	Query  http.Header `json:"-"`
}

type managementResponse struct {
	StatusCode int         `json:"StatusCode"`
	Headers    http.Header `json:"Headers"`
	Body       []byte      `json:"Body"`
}

func managementRegistration() managementRegistrationResponse {
	return managementRegistrationResponse{
		Routes: []managementRoute{
			{Method: http.MethodGet, Path: managementStatusPath},
			{Method: http.MethodPut, Path: managementDisabledPath},
		},
		Resources: []managementResource{{
			Path:        "/status",
			Menu:        "Cursor",
			Description: "Cursor status, usage and model controls / Cursor 状态、用量与模型管理。",
		}},
	}
}

func (handler *Handler) handleManagement(ctx context.Context, raw []byte) (any, error) {
	var request managementRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		return nil, err
	}
	path := strings.TrimSpace(request.Path)
	switch {
	case strings.Contains(path, "/v0/resource/plugins/") && strings.HasSuffix(path, "/status"):
		return managementPageResponse(), nil
	case strings.HasSuffix(path, "/status") && strings.EqualFold(request.Method, http.MethodGet):
		return handler.managementStatus(ctx)
	case strings.HasSuffix(path, "/disabled-models") && strings.EqualFold(request.Method, http.MethodPut):
		return handler.updateDisabledModels(ctx, request.Body)
	default:
		return managementResponse{StatusCode: http.StatusNotFound, Headers: jsonHeaders(), Body: []byte(`{"error":"not found"}`)}, nil
	}
}

func jsonHeaders() http.Header {
	return http.Header{"Content-Type": []string{"application/json; charset=utf-8"}}
}
