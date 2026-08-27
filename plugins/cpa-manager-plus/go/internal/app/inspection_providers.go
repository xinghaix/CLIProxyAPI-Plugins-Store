package app

import "strings"

// inspectionProviders is the ordered set of credential file types the local
// inspector can probe for health/quota.
var inspectionProviders = []string{
	"codex",
	"xai",
	"claude",
	"kimi",
	"antigravity",
	"gemini-cli",
	"vertex",
}

func supportedInspectionProvider(provider string) bool {
	provider = strings.ToLower(strings.TrimSpace(provider))
	for _, item := range inspectionProviders {
		if item == provider {
			return true
		}
	}
	return false
}
