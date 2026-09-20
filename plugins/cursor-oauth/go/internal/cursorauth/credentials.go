package cursorauth

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type jwtPayload struct {
	Subject json.RawMessage `json:"sub"`
	Email   string          `json:"email"`
	Expiry  int64           `json:"exp"`
}

func credentialsFromTokens(accessToken, refreshToken string, now time.Time) (Credentials, error) {
	if accessToken == "" || refreshToken == "" {
		return Credentials{}, errors.New("Cursor credentials require access and refresh tokens")
	}
	payload := parseJWTPayload(accessToken)
	if payload.Expiry == 0 {
		payload = parseJWTPayload(refreshToken)
	}
	expiresAt := now.Add(time.Hour).UTC()
	if payload.Expiry > 0 {
		expiresAt = time.Unix(payload.Expiry, 0).Add(-5 * time.Minute).UTC()
	}
	return Credentials{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    expiresAt,
		AccountID:    parseSubject(payload.Subject),
		Email:        strings.ToLower(strings.TrimSpace(payload.Email)),
		Type:         "cursor",
	}, nil
}

func ParseCredentials(raw []byte) (Credentials, error) {
	var credentials Credentials
	if err := json.Unmarshal(raw, &credentials); err != nil {
		return Credentials{}, fmt.Errorf("decode Cursor credentials: %w", err)
	}
	if credentials.Type != "cursor" || credentials.AccessToken == "" || credentials.RefreshToken == "" {
		return Credentials{}, errors.New("Cursor credentials are incomplete")
	}
	return credentials, nil
}

func MarshalCredentials(credentials Credentials) ([]byte, error) {
	raw, err := json.Marshal(credentials)
	if err != nil {
		return nil, fmt.Errorf("encode Cursor credentials: %w", err)
	}
	return raw, nil
}

func parseJWTPayload(token string) jwtPayload {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return jwtPayload{}
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return jwtPayload{}
	}
	var payload jwtPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return jwtPayload{}
	}
	return payload
}

func parseSubject(raw json.RawMessage) string {
	var subject string
	if err := json.Unmarshal(raw, &subject); err == nil {
		return subject
	}
	var number int64
	if err := json.Unmarshal(raw, &number); err == nil {
		return strconv.FormatInt(number, 10)
	}
	return ""
}
