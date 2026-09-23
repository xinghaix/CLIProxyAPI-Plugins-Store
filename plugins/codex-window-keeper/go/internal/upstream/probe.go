package upstream

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/codex-window-keeper/go/internal/usage"
)

type Doer func(method, target string, header map[string]string, body []byte) (int, []byte, error)

type StatusError struct{ Code int }

func (e StatusError) Error() string   { return fmt.Sprintf("upstream status %d", e.Code) }
func (e StatusError) StatusCode() int { return e.Code }

func ProbeUsage(ctx context.Context, do Doer, baseURL, managementKey, authIndex, accountID string, now time.Time) (usage.Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return usage.Snapshot{}, err
	}
	if strings.TrimSpace(managementKey) == "" {
		return usage.Snapshot{}, fmt.Errorf("management key is not configured")
	}
	if do == nil {
		return usage.Snapshot{}, fmt.Errorf("http client is not configured")
	}
	header := map[string]string{"Authorization": "Bearer $TOKEN$", "Content-Type": "application/json", "User-Agent": "codex_cli_rs/0.76.0"}
	if strings.TrimSpace(accountID) != "" {
		header["Chatgpt-Account-Id"] = accountID
	}
	payload, _ := json.Marshal(map[string]any{"authIndex": authIndex, "method": http.MethodGet, "url": "https://chatgpt.com/backend-api/wham/usage", "header": header})
	status, body, err := do(http.MethodPost, strings.TrimRight(baseURL, "/")+"/v0/management/api-call", map[string]string{"Authorization": "Bearer " + managementKey, "Content-Type": "application/json"}, payload)
	if err != nil {
		return usage.Snapshot{}, err
	}
	if status < 200 || status >= 300 {
		return usage.Snapshot{}, StatusError{Code: status}
	}
	if err := ctx.Err(); err != nil {
		return usage.Snapshot{}, err
	}
	var wrapped map[string]json.RawMessage
	if err := json.Unmarshal(body, &wrapped); err != nil {
		return usage.Snapshot{}, err
	}
	var upstreamStatus int
	_ = json.Unmarshal(wrapped["status_code"], &upstreamStatus)
	if upstreamStatus == 0 {
		_ = json.Unmarshal(wrapped["statusCode"], &upstreamStatus)
	}
	if upstreamStatus >= 400 {
		return usage.Snapshot{}, StatusError{Code: upstreamStatus}
	}
	usageBody := wrapped["body"]
	if len(usageBody) > 0 && usageBody[0] == '"' {
		var bodyText string
		if err := json.Unmarshal(usageBody, &bodyText); err != nil {
			return usage.Snapshot{}, err
		}
		usageBody = []byte(bodyText)
	}
	return usage.Parse(usageBody, now)
}
