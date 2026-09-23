package upstream

import (
	"bytes"
	"encoding/json"
	"strings"
)

func Completion(chunks []byte) (bool, string) {
	completed := false
	failed := false
	var deltas, final strings.Builder
	for _, line := range bytes.Split(chunks, []byte("\n")) {
		line = bytes.TrimSpace(line)
		line = bytes.TrimPrefix(line, []byte("data:"))
		line = bytes.TrimSpace(line)
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		var event map[string]any
		if err := json.Unmarshal(line, &event); err != nil {
			continue
		}
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
	}
	text := final.String()
	if deltas.Len() > 0 {
		text = deltas.String()
	}
	excerpt := strings.TrimSpace(text)
	return completed && !failed && excerpt != "", excerpt
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
