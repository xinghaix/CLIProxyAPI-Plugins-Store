package windowkeeper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

func Completion(chunks []byte) (bool, string) {
	completed := false
	failed := false
	var deltas, final strings.Builder
	forEachSSEDataJSON(chunks, func(event map[string]any) {
		eventType, _ := event["type"].(string)
		switch eventType {
		case "response.completed":
			completed = true
			collectOutputText(event["response"], &final)
		case "response.output_text.delta":
			if delta, ok := event["delta"].(string); ok {
				deltas.WriteString(delta)
			}
		case "response.failed", "response.incomplete", "error":
			failed = true
		}
	})
	text := final.String()
	if deltas.Len() > 0 {
		text = deltas.String()
	}
	excerpt := strings.TrimSpace(text)
	return completed && !failed && excerpt != "", excerpt
}

// forEachSSEDataJSON scans the buffer for every `data:` payload and decodes
// consecutive JSON objects. Works for standard newline-delimited SSE and for
// jammed streams that concatenate event/data frames without newlines.
// Optional `event:` lines are ignored; non-JSON after `data:` is skipped.
func forEachSSEDataJSON(chunks []byte, fn func(map[string]any)) {
	remaining := chunks
	for {
		idx := bytes.Index(remaining, []byte("data:"))
		if idx < 0 {
			return
		}
		remaining = remaining[idx+len("data:"):]
		remaining = bytes.TrimLeft(remaining, " \t")
		if len(remaining) == 0 {
			return
		}
		dec := json.NewDecoder(bytes.NewReader(remaining))
		var event map[string]any
		if err := dec.Decode(&event); err != nil {
			// Non-JSON after data: (e.g. [DONE]) — keep scanning for the next data:.
			continue
		}
		fn(event)
		n := int(dec.InputOffset())
		if n <= 0 {
			return
		}
		remaining = remaining[n:]
	}
}

func collectOutputText(value any, out *strings.Builder) {
	switch typed := value.(type) {
	case map[string]any:
		if typed["type"] == "output_text" {
			if text, ok := typed["text"].(string); ok {
				out.WriteString(text)
			}
		}
		for _, key := range []string{"response", "output", "content", "message", "item"} {
			collectOutputText(typed[key], out)
		}
	case []any:
		for _, child := range typed {
			collectOutputText(child, out)
		}
	}
}

// ExcerptFromUpstream builds a short attempt excerpt from an upstream error body or message.
// Prefers JSON "detail", then nested error.message / top-level "message"/"error" strings,
// otherwise a trimmed raw snippet (max 120 runes).
func ExcerptFromUpstream(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
		if detail, ok := parsed["detail"].(string); ok && strings.TrimSpace(detail) != "" {
			return clipExcerpt(detail)
		}
		switch errVal := parsed["error"].(type) {
		case string:
			if strings.TrimSpace(errVal) != "" {
				return clipExcerpt(errVal)
			}
		case map[string]any:
			if msg, ok := errVal["message"].(string); ok && strings.TrimSpace(msg) != "" {
				return clipExcerpt(msg)
			}
		}
		if msg, ok := parsed["message"].(string); ok && strings.TrimSpace(msg) != "" {
			return clipExcerpt(msg)
		}
	}
	return clipExcerpt(raw)
}

func clipExcerpt(text string) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) > 120 {
		return string(runes[:120])
	}
	return string(runes)
}

const (
	maxDetailRunes = 8192
	maxHeaderRunes = 4096
)

// clipDetail truncates large request/response payloads for attempt storage.
func clipDetail(text string, maxRunes int) string {
	if maxRunes <= 0 {
		maxRunes = maxDetailRunes
	}
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= maxRunes {
		return string(runes)
	}
	return string(runes[:maxRunes]) + "…(truncated)"
}

// FormatHeadersJSON serializes headers for attempt detail, redacting secrets.
func FormatHeadersJSON(headers map[string][]string) string {
	if len(headers) == 0 {
		return ""
	}
	safe := make(map[string][]string, len(headers))
	for k, vals := range headers {
		lk := strings.ToLower(k)
		if lk == "authorization" || lk == "cookie" || lk == "set-cookie" || lk == "x-api-key" || strings.Contains(lk, "token") {
			safe[k] = []string{"[redacted]"}
			continue
		}
		safe[k] = vals
	}
	raw, err := json.Marshal(safe)
	if err != nil {
		return clipDetail(fmt.Sprintf("%v", safe), maxHeaderRunes)
	}
	return clipDetail(string(raw), maxHeaderRunes)
}

// FinishFromSend builds AttemptFinish from a SendResult (and optional overrides).
func FinishFromSend(status string, sent SendResult, kind, excerpt string) AttemptFinish {
	return AttemptFinish{
		Status:      status,
		HTTPStatus:  sent.Status,
		Kind:        kind,
		Excerpt:     excerpt,
		ResponseID:  sent.ResponseID,
		ReqHeaders:  clipDetail(sent.ReqHeaders, maxHeaderRunes),
		ReqBody:     clipDetail(sent.ReqBody, maxDetailRunes),
		RespHeaders: clipDetail(sent.RespHeaders, maxHeaderRunes),
		RespBody:    clipDetail(sent.RespBody, maxDetailRunes),
	}
}
