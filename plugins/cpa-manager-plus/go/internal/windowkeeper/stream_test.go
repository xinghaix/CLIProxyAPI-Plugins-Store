package windowkeeper

import "testing"

func TestCompletionRequiresSuccessfulTerminalEvent(t *testing.T) {
	jammedOK := `event: response.createddata: {"type":"response.created","response":{"id":"resp_1"}}event: response.completeddata: {"type":"response.completed","response":{"id":"resp_1","output":[{"content":[{"type":"output_text","text":"OK"}]}]}}`
	normalNewline := "event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_1\"}}\n\nevent: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"output\":[{\"content\":[{\"type\":\"output_text\",\"text\":\"OK\"}]}]}}\n\n"
	failedJammed := `event: response.createddata: {"type":"response.created"}event: response.faileddata: {"type":"response.failed"}`
	for _, tc := range []struct {
		name, data string
		ok         bool
		text       string
	}{
		{name: "delta-and-complete", data: "data: {\"type\":\"response.output_text.delta\",\"delta\":\"OK\"}\ndata: {\"type\":\"response.completed\",\"response\":{\"output\":[{\"content\":[{\"type\":\"output_text\",\"text\":\"OK\"}]}]}}", ok: true, text: "OK"},
		{name: "completed-only", data: "data: {\"type\":\"response.completed\",\"response\":{\"output\":[{\"content\":[{\"type\":\"output_text\",\"text\":\"Hello\"}]}]}}", ok: true, text: "Hello"},
		{name: "failed", data: "data: {\"type\":\"response.completed\",\"response\":{\"output\":[{\"content\":[{\"type\":\"output_text\",\"text\":\"OK\"}]}]}}\ndata: {\"type\":\"response.failed\"}", ok: false, text: "OK"},
		{name: "partial", data: "data: {\"type\":\"response.output_text.delta\",\"delta\":\"OK\"}", ok: false, text: "OK"},
		{name: "jammed-sse-completed-ok", data: jammedOK, ok: true, text: "OK"},
		{name: "normal-newline-sse-still-ok", data: normalNewline, ok: true, text: "OK"},
		{name: "jammed-sse-response-failed", data: failedJammed, ok: false, text: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ok, text := Completion([]byte(tc.data))
			if ok != tc.ok || text != tc.text {
				t.Fatalf("Completion = (%v, %q), want (%v, %q)", ok, text, tc.ok, tc.text)
			}
		})
	}
}
