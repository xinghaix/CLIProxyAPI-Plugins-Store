package cursorauth

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
)

func (service *Service) Refresh(ctx context.Context, current Credentials) (Credentials, error) {
	if current.RefreshToken == "" {
		return Credentials{}, errors.New("Cursor refresh token is required")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, service.endpoints.RefreshURL, bytes.NewReader([]byte("{}")))
	if err != nil {
		return Credentials{}, fmt.Errorf("create Cursor refresh request: %w", err)
	}
	request.Header.Set("authorization", "Bearer "+current.RefreshToken)
	request.Header.Set("content-type", "application/json")
	response, err := service.client.Do(request)
	if err != nil {
		return Credentials{}, fmt.Errorf("refresh Cursor token: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		status := response.StatusCode
		return Credentials{}, errors.Join(fmt.Errorf("Cursor token refresh returned HTTP %d", status), response.Body.Close())
	}
	var tokens tokenResponse
	if err := decodeJSON(response.Body, &tokens); err != nil {
		return Credentials{}, fmt.Errorf("decode Cursor refresh response: %w", err)
	}
	if tokens.AccessToken == "" {
		return Credentials{}, errors.New("Cursor refresh response is missing access token")
	}
	if tokens.RefreshToken == "" {
		tokens.RefreshToken = current.RefreshToken
	}
	refreshed, err := credentialsFromTokens(tokens.AccessToken, tokens.RefreshToken, service.now())
	if err != nil {
		return Credentials{}, err
	}
	refreshed.DisabledModels = append([]string(nil), current.DisabledModels...)
	refreshed.ToolLoopGuardTools = append([]string(nil), current.ToolLoopGuardTools...)
	return refreshed, nil
}
