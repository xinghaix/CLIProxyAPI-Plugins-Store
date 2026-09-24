package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/store"
)

// OAuthCredential is a read-only auth-file summary focused on OAuth accounts.
type OAuthCredential struct {
	RowKey        string                      `json:"rowKey"`
	FileName      string                      `json:"fileName"`
	DisplayName   string                      `json:"displayName"`
	Email         string                      `json:"email,omitempty"`
	Provider      string                      `json:"provider"`
	AuthID        string                      `json:"authId,omitempty"`
	AuthIndex     string                      `json:"authIndex,omitempty"`
	AuthType      string                      `json:"authType"`
	AccountID     string                      `json:"accountId,omitempty"`
	Status        string                      `json:"status,omitempty"`
	Disabled      bool                        `json:"disabled"`
	Unavailable   bool                        `json:"unavailable,omitempty"`
	Note          string                      `json:"note,omitempty"`
	Path          string                      `json:"path,omitempty"`
	Priority      *int                        `json:"priority,omitempty"`
	Weight        *int                        `json:"weight,omitempty"`
	Source        string                      `json:"source,omitempty"`
	Label         string                      `json:"label,omitempty"`
	ProjectID     string                      `json:"projectId,omitempty"`
	ModTime       string                      `json:"modTime,omitempty"`
	CreatedAt     string                      `json:"createdAt,omitempty"`
	UpdatedAt     string                      `json:"updatedAt,omitempty"`
	LastRefresh   string                      `json:"lastRefresh,omitempty"`
	StatusMessage string                      `json:"statusMessage,omitempty"`
	Metadata      *store.InspectionAuthMetadata `json:"metadata,omitempty"`
}

// ListOAuthCredentials returns CPA auth-files filtered to OAuth (and oauth2).
// API-key credentials are omitted unless includeAPIKeys is true.
func (r *Runtime) ListOAuthCredentials(ctx context.Context, includeAPIKeys bool) ([]OAuthCredential, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	r.mu.Lock()
	list := r.authList
	r.mu.Unlock()
	if list == nil {
		return nil, fmt.Errorf("auth list is not available")
	}
	auths, err := list()
	if err != nil {
		return nil, err
	}
	out := make([]OAuthCredential, 0, len(auths))
	for _, auth := range auths {
		authType := inspectionAuthType(auth)
		if !includeAPIKeys && authType != "oauth" {
			continue
		}
		meta := inspectionAuthMetadataFromEntry(auth, authType)
		provider := strings.ToLower(strings.TrimSpace(firstNonEmpty(auth.Provider, auth.Type)))
		fileName := firstNonEmpty(auth.Name, auth.ID)
		display := firstNonEmpty(auth.Email, auth.ProjectID, auth.Label, auth.Name)
		if display == "" && authType == "oauth" {
			display = auth.Account
		}
		accountID := auth.Account
		if authType == "apikey" {
			accountID = ""
		}
		metaCopy := meta
		out = append(out, OAuthCredential{
			RowKey:        firstNonEmpty(auth.AuthIndex, auth.ID, fileName, provider),
			FileName:      fileName,
			DisplayName:   display,
			Email:         auth.Email,
			Provider:      provider,
			AuthID:        auth.ID,
			AuthIndex:     auth.AuthIndex,
			AuthType:      authType,
			AccountID:     accountID,
			Status:        auth.Status,
			Disabled:      auth.Disabled,
			Unavailable:   meta.Unavailable,
			Note:          meta.Note,
			Path:          meta.Path,
			Priority:      meta.Priority,
			Weight:        meta.Weight,
			Source:        meta.Source,
			Label:         meta.Label,
			ProjectID:     meta.ProjectID,
			ModTime:       meta.ModTime,
			CreatedAt:     meta.CreatedAt,
			UpdatedAt:     meta.UpdatedAt,
			LastRefresh:   meta.LastRefresh,
			StatusMessage: meta.StatusMessage,
			Metadata:      &metaCopy,
		})
	}
	return out, nil
}

// AnalyticsTimeZone returns the plugin-configurable timezone used by Inspection
// scheduling (and monitoring analytics buckets) when the client omits one.
func (r *Runtime) AnalyticsTimeZone() string {
	return strings.TrimSpace(r.CodexInspectionSettings().Schedule.TimeZone)
}
