package plugin

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cursor-oauth/go/internal/cursorapi"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cursor-oauth/go/internal/cursorauth"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cursor-oauth/go/internal/cursorproto"

	"github.com/stretchr/testify/require"
)

func Test_Handler_ModelsForAuth_hides_models_disabled_by_cursor_plugin(t *testing.T) {
	handler := NewHandler(Dependencies{Cursor: fakeModelCursorClient{models: []string{"auto", "claude-4-sonnet", "gpt-5"}}})
	storage, err := cursorauth.MarshalCredentials(cursorauth.Credentials{
		AccessToken:    "access",
		RefreshToken:   "refresh",
		Type:           "cursor",
		DisabledModels: []string{"cursor/gpt-5"},
	})
	require.NoError(t, err)
	rawRequest, err := json.Marshal(authModelRequest{StorageJSON: storage})
	require.NoError(t, err)

	result, err := handler.modelsForAuth(context.Background(), rawRequest)

	require.NoError(t, err)
	rawResult, err := json.Marshal(result)
	require.NoError(t, err)
	require.Contains(t, string(rawResult), `"ID":"cursor/auto"`)
	require.Contains(t, string(rawResult), `"SupportedInputModalities":["text","image"]`)
	require.Contains(t, string(rawResult), `"SupportedOutputModalities":["text","image"]`)
	require.Contains(t, string(rawResult), `"ID":"cursor/claude-4-sonnet"`)
	require.NotContains(t, string(rawResult), `"ID":"cursor/gpt-5"`)
}

type fakeModelCursorClient struct {
	models []string
}

func (client fakeModelCursorClient) Run(context.Context, cursorapi.RunInput, func(cursorproto.ServerEvent) error) (cursorapi.RunResult, error) {
	return cursorapi.RunResult{}, nil
}

func (client fakeModelCursorClient) DiscoverModels(context.Context, string) ([]string, error) {
	return append([]string(nil), client.models...), nil
}
