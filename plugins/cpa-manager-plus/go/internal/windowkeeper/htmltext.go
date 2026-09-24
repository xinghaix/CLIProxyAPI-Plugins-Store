package windowkeeper

import (
	"html"
	"strings"
)

// NormalizeDetailText unescapes HTML entities once so stored/displayed attempt
// bodies keep normal JSON quotes instead of &#34;. Idempotent for raw JSON.
func NormalizeDetailText(text string) string {
	if text == "" || !strings.ContainsAny(text, "&") {
		return text
	}
	return html.UnescapeString(text)
}
