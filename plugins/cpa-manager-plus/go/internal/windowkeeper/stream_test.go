package windowkeeper

import "testing"

func TestCompletionRequiresSuccessfulTerminalEvent(t *testing.T) {
	jammedOK := `event: response.createddata: {"type":"response.created","response":{"id":"resp_1"}}event: response.completeddata: {"type":"response.completed","response":{"id":"resp_1","output":[{"content":[{"type":"output_text","text":"OK"}]}]}}`
	normalNewline := "event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_1\"}}\n\nevent: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"output\":[{\"content\":[{\"type\":\"output_text\",\"text\":\"OK\"}]}]}}\n\n"
	failedJammed := `event: response.createddata: {"type":"response.created"}event: response.faileddata: {"type":"response.failed"}`
	// Host may HTML-escape quotes in stream payloads (&#34;); must still succeed.
	escapedOK := "data: {&#34;type&#34;:&#34;response.completed&#34;,&#34;response&#34;:{&#34;output&#34;:[{&#34;content&#34;:[{&#34;type&#34;:&#34;output_text&#34;,&#34;text&#34;:&#34;OK&#34;}]}]}}"
	escapedJammed := `event: response.completeddata: {&#34;type&#34;:&#34;response.completed&#34;,&#34;response&#34;:{&#34;output&#34;:[{&#34;content&#34;:[{&#34;type&#34;:&#34;output_text&#34;,&#34;text&#34;:&#34;OK&#34;}]}]}}`
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
		{name: "html-escaped-completed-ok", data: escapedOK, ok: true, text: "OK"},
		{name: "html-escaped-jammed-completed-ok", data: escapedJammed, ok: true, text: "OK"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ok, text := Completion([]byte(tc.data))
			if ok != tc.ok || text != tc.text {
				t.Fatalf("Completion = (%v, %q), want (%v, %q)", ok, text, tc.ok, tc.text)
			}
		})
	}
}

func TestNormalizeDetailTextUnescapesQuotes(t *testing.T) {
	in := `{&#34;model&#34;:&#34;gpt-5.4&#34;}`
	got := NormalizeDetailText(in)
	want := `{"model":"gpt-5.4"}`
	if got != want {
		t.Fatalf("NormalizeDetailText = %q, want %q", got, want)
	}
	if NormalizeDetailText(want) != want {
		t.Fatalf("NormalizeDetailText should be idempotent on raw JSON")
	}
}

func TestClassifyStreamFailure(t *testing.T) {
	failedSSE := `data: {"type":"response.failed","response":{"error":{"message":"boom"}}}`
	quotaSSE := `data: {"type":"response.failed"}` + "\n" + `data: {"type":"error","message":"The usage limit has been reached"}`
	configHint := `{"error":{"type":"invalid_request","message":"bad model"}}`
	for _, tc := range []struct {
		name, kind string
		status     int
		chunks     string
		excerpt    string
	}{
		{name: "status-401", status: 401, kind: ErrKindAuth},
		{name: "status-429", status: 429, kind: ErrKindQuota},
		{name: "status-400", status: 400, kind: ErrKindConfig},
		{name: "sse-failed-retry", status: 200, chunks: failedSSE, kind: ErrKindRetry},
		{name: "usage-limit-quota", status: 200, chunks: quotaSSE, excerpt: "The usage limit has been reached", kind: ErrKindQuota},
		{name: "invalid-request-config", status: 200, chunks: configHint, kind: ErrKindConfig},
		{name: "empty-200-retry", status: 200, chunks: "", kind: ErrKindRetry},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyStreamFailure(tc.status, []byte(tc.chunks), tc.excerpt)
			if got != tc.kind {
				t.Fatalf("ClassifyStreamFailure = %q, want %q", got, tc.kind)
			}
		})
	}
}

func TestFinishFromSendNormalizesEscapedBodies(t *testing.T) {
	sent := SendResult{
		Status:   200,
		ReqBody:  `{&#34;model&#34;:&#34;gpt-5.4&#34;}`,
		RespBody: `data: {&#34;type&#34;:&#34;response.completed&#34;}`,
		Excerpt:  `OK`,
	}
	fin := FinishFromSend("succeeded", sent, "", sent.Excerpt)
	if fin.ReqBody != `{"model":"gpt-5.4"}` {
		t.Fatalf("ReqBody = %q", fin.ReqBody)
	}
	if fin.RespBody != `data: {"type":"response.completed"}` {
		t.Fatalf("RespBody = %q", fin.RespBody)
	}
}
