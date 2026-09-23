package responsemodel

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

var original = []byte(`{"model":"billing-alias","messages":[]}`)

func raw(source, dest, id, model string) pluginapi.ResponseTransformRequest {
	body := fmt.Sprintf(`{"responseId":%q,"modelVersion":%q,"candidates":[]}`, id, model)
	if source == "antigravity" {
		body = `{"response":` + body + "}"
	}
	return pluginapi.ResponseTransformRequest{FromFormat: source, ToFormat: dest, Model: "billing-alias", OriginalRequest: original, Body: []byte(body)}
}
func response(format, id, header string) pluginapi.ResponseInterceptRequest {
	field := "id"
	if format == "gemini" {
		field = "responseId"
	}
	return pluginapi.ResponseInterceptRequest{RequestID: "execution-" + header, SourceFormat: format, OriginalRequest: original, ResponseHeaders: http.Header{"X-Request-ID": {header}}, Metadata: map[string]any{"selected_auth_id": "auth"}, Body: []byte(fmt.Sprintf(`{%q:%q,"model":"NOT-UPSTREAM"}`, field, id))}
}
func check(t *testing.T, got []Observation, key, model string, ambiguous bool) {
	t.Helper()
	if len(got) != 1 {
		t.Fatalf("want one observation, got %+v", got)
	}
	o := got[0]
	if o.Key != key || o.Ambiguous != ambiguous {
		t.Fatalf("wrong observation %+v", o)
	}
	if !ambiguous && (o.Model != model || len(o.EvidenceID) != 64) {
		t.Fatalf("wrong evidence %+v", o)
	}
}
func none(t *testing.T, got []Observation) {
	t.Helper()
	if len(got) != 0 {
		t.Fatalf("unexpected observations: %+v", got)
	}
}
func TestCorrelationKey(t *testing.T) {
	h := http.Header{"x-request-id": {" exact "}}
	key := CorrelationKey("auth", h)
	if len(key) != 64 {
		t.Fatal(key)
	}
	if key != CorrelationKey("auth", http.Header{"X-Request-ID": {" exact "}, "Request-ID": {"other"}}) {
		t.Fatal("priority/case")
	}
	if key == CorrelationKey("auth", http.Header{"X-Request-ID": {"exact"}}) {
		t.Fatal("trimmed value")
	}
	if key == CorrelationKey("other", h) {
		t.Fatal("auth not scoped")
	}
	if key == CorrelationKey("auth", http.Header{"Request-ID": {" exact "}}) {
		t.Fatal("name not scoped")
	}
	if key != CorrelationKey("auth", http.Header{"X-Request-ID": {" exact "}, "x-request-id": {" exact "}}) {
		t.Fatal("equal case duplicates")
	}
	for _, h := range []http.Header{nil, {"X-Request-ID": {"a", "a"}}, {"X-Request-ID": {"a,b"}}, {"X-Request-ID": {"a"}, "x-request-id": {"b"}}, {"Trace-ID": {"a"}}, {"X-Session-ID": {"a"}}, {"X-Request-ID": {""}}, {"X-Request-ID": {strings.Repeat("a", maxID+1)}}, {"X-Request-ID": {"ok"}, "Request-ID": {"a", "b"}}} {
		if CorrelationKey("auth", h) != "" {
			t.Fatalf("accepted invalid header: %#v", h)
		}
	}
	if CorrelationKey("", h) != "" {
		t.Fatal("accepted missing auth")
	}
	for _, name := range []string{"Request-ID", "X-Goog-Request-ID"} {
		if CorrelationKey("auth", http.Header{name: {"native"}}) == "" {
			t.Fatal(name)
		}
	}
}
func TestActualServiceTierObservationAndStreamingConflict(t *testing.T) {
	key := CorrelationKey("auth", http.Header{"X-Request-ID": {"response-tier"}})
	tracker := New()
	responseBody := response("openai", "response-tier", "response-tier")
	responseBody.Body = []byte(`{"id":"response-tier","model":"gpt-5.6-sol","service_tier":"priority"}`)
	observed := tracker.ObserveResponse(responseBody)
	if len(observed) != 1 || observed[0].Key != key || observed[0].ServiceTier != "priority" || observed[0].ServiceTierAmbiguous {
		t.Fatalf("actual tier was not captured: %+v", observed)
	}

	streamTracker := New()
	stream := response("openai", "response-stream", "response-stream")
	stream.Body = []byte(`{"id":"response-stream","model":"gpt-5.6-sol"}`)
	none(t, streamTracker.ObserveResponse(stream))
	stream.Body = []byte(`{"id":"response-stream","model":"gpt-5.6-sol","service_tier":"default"}`)
	observed = streamTracker.ObserveResponse(stream)
	if len(observed) != 1 || observed[0].ServiceTier != "default" {
		t.Fatalf("late stream tier was not emitted: %+v", observed)
	}
	stream.Body = []byte(`{"id":"response-stream","model":"gpt-5.6-sol","service_tier":"priority"}`)
	observed = streamTracker.ObserveResponse(stream)
	if len(observed) != 1 || !observed[0].ServiceTierAmbiguous || observed[0].ServiceTier != "" {
		t.Fatalf("conflicting tiers were not quarantined: %+v", observed)
	}

	unknownTracker := New()
	unknown := response("openai", "response-unknown", "response-unknown")
	unknown.Body = []byte(`{"id":"response-unknown","service_tier":"made-up"}`)
	none(t, unknownTracker.ObserveResponse(unknown))
}

func TestExactJoinsBothOrdersAndFormats(t *testing.T) {
	for _, source := range []string{"gemini", "antigravity"} {
		for _, format := range []string{"openai", "openai-response", "claude", "gemini"} {
			for _, reverse := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/reverse=%v", source, format, reverse), func(t *testing.T) {
					tr := New()
					id := "native-1"
					translated := id
					if format == "openai-response" {
						translated = "resp_" + id
					}
					a, b := raw(source, format, id, "actual-version"), response(format, translated, "header-not-body-id")
					key := CorrelationKey("auth", b.ResponseHeaders)
					if reverse {
						none(t, tr.ObserveResponse(b))
						check(t, tr.ObserveRaw(a), key, "actual-version", false)
					} else {
						none(t, tr.ObserveRaw(a))
						check(t, tr.ObserveResponse(b), key, "actual-version", false)
					}
					none(t, tr.ObserveRaw(a))
					none(t, tr.ObserveResponse(b))
					if string(a.OriginalRequest) != string(original) || b.Model != "" {
						t.Fatal("inputs mutated")
					}
				})
			}
		}
	}
}
func TestStreamHeaderCacheAndOmissions(t *testing.T) {
	for _, format := range []string{"openai", "openai-response", "claude", "gemini"} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%v", format, reverse), func(t *testing.T) {
				tr := New()
				a := raw("antigravity", format, "native", "actual")
				id := "native"
				if format == "openai-response" {
					id = "resp_native"
				}
				r := response(format, id, "h")
				init := pluginapi.StreamChunkInterceptRequest{RequestID: "stream", SourceFormat: format, OriginalRequest: original, ResponseHeaders: r.ResponseHeaders, Metadata: r.Metadata, ChunkIndex: pluginapi.StreamChunkHeaderInitIndex}
				none(t, tr.ObserveStream(init))
				body := string(r.Body)
				if format == "claude" {
					body = `{"type":"message_start","message":{"id":"native","model":"fake"}}`
				}
				if format == "openai-response" {
					body = `{"type":"response.created","response":{"id":"resp_native","model":"fake"}}`
				}
				chunk := pluginapi.StreamChunkInterceptRequest{RequestID: "stream", Body: []byte("event: message\ndata: " + body + "\n\n")}
				key := CorrelationKey("auth", r.ResponseHeaders)
				if reverse {
					none(t, tr.ObserveStream(chunk))
					check(t, tr.ObserveRaw(a), key, "actual", false)
				} else {
					none(t, tr.ObserveRaw(a))
					check(t, tr.ObserveStream(chunk), key, "actual", false)
				}
				none(t, tr.ObserveStream(chunk))
				none(t, tr.ObserveRaw(a))
			})
		}
	}
}
func TestRawConflictsRevoke(t *testing.T) {
	for _, kind := range []string{"model", "prefix", "source"} {
		t.Run(kind, func(t *testing.T) {
			tr := New()
			a := raw("gemini", "openai-response", "native", "actual")
			r := response("openai-response", "resp_native", "h")
			tr.ObserveRaw(a)
			check(t, tr.ObserveResponse(r), CorrelationKey("auth", r.ResponseHeaders), "actual", false)
			switch kind {
			case "model":
				a = raw("gemini", "openai-response", "native", "different")
			case "prefix":
				a = raw("gemini", "openai-response", "resp_native", "actual")
			case "source":
				a = raw("antigravity", "openai-response", "native", "actual")
			}
			check(t, tr.ObserveRaw(a), CorrelationKey("auth", r.ResponseHeaders), "", true)
			none(t, tr.ObserveRaw(a))
			none(t, tr.ObserveResponse(r))
		})
	}
}
func TestDifferentRequestsNeverFingerprintOnly(t *testing.T) {
	tr := New()
	tr.ObserveRaw(raw("gemini", "openai", "one", "actual-one"))
	r := response("openai", "two", "h2")
	none(t, tr.ObserveResponse(r))
	check(t, tr.ObserveRaw(raw("gemini", "openai", "two", "actual-two")), CorrelationKey("auth", r.ResponseHeaders), "actual-two", false)
	r = response("openai", "one", "h1")
	check(t, tr.ObserveResponse(r), CorrelationKey("auth", r.ResponseHeaders), "actual-one", false)
	r = response("claude", "one", "h3")
	none(t, tr.ObserveResponse(r))
	r = response("openai", "one", "h4")
	r.OriginalRequest = []byte(`{"different":true}`)
	none(t, tr.ObserveResponse(r))
}
func TestMultipleHeaderKeysAndBounds(t *testing.T) {
	tr := New()
	tr.ObserveRaw(raw("gemini", "openai", "one", "actual"))
	r := response("openai", "one", "h0")
	check(t, tr.ObserveResponse(r), CorrelationKey("auth", r.ResponseHeaders), "actual", false)
	r2 := response("openai", "one", "h1")
	got := tr.ObserveResponse(r2)
	if len(got) != 2 {
		t.Fatalf("did not revoke both keys: %+v", got)
	}
	for _, o := range got {
		if !o.Ambiguous {
			t.Fatal(o)
		}
	}
	for i := 2; i < maxKeys+10; i++ {
		r = response("openai", "one", fmt.Sprint("h", i))
		check(t, tr.ObserveResponse(r), CorrelationKey("auth", r.ResponseHeaders), "", true)
		none(t, tr.ObserveResponse(r))
	}
	for _, e := range tr.entries {
		if len(e.keys) > maxKeys {
			t.Fatal("unbounded keys")
		}
	}
}
func TestBridgeIdentityChanges(t *testing.T) {
	for _, kind := range []string{"id", "auth", "header", "scope", "format", "invalid-auth", "invalid-header"} {
		t.Run(kind, func(t *testing.T) {
			tr := New()
			tr.ObserveRaw(raw("gemini", "openai", "one", "actual"))
			r := response("openai", "one", "h")
			init := pluginapi.StreamChunkInterceptRequest{RequestID: "s", SourceFormat: "openai", OriginalRequest: original, Metadata: r.Metadata, ResponseHeaders: r.ResponseHeaders, Body: r.Body}
			check(t, tr.ObserveStream(init), CorrelationKey("auth", r.ResponseHeaders), "actual", false)
			change := pluginapi.StreamChunkInterceptRequest{RequestID: "s", Body: r.Body}
			switch kind {
			case "id":
				change.Body = []byte(`{"id":"two"}`)
			case "auth":
				change.Metadata = map[string]any{"selected_auth_id": "other"}
			case "header":
				change.ResponseHeaders = http.Header{"X-Request-ID": {"other"}}
			case "scope":
				change.OriginalRequest = []byte("different")
			case "format":
				change.SourceFormat = "claude"
			case "invalid-auth":
				change.Metadata = map[string]any{"selected_auth_id": 42}
			case "invalid-header":
				change.ResponseHeaders = http.Header{"X-Request-ID": {"a", "b"}}
			}
			check(t, tr.ObserveStream(change), CorrelationKey("auth", r.ResponseHeaders), "", true)
			none(t, tr.ObserveStream(change))
		})
	}
}
func TestMissingUnsupportedSyntheticAndMalformed(t *testing.T) {
	for _, body := range []string{`{}`, `{"responseId":"one"}`, `{"modelVersion":"actual"}`, `{"responseId":"imagen-123","modelVersion":"actual"}`, `{"responseId":"one","modelVersion":"imagen-3"}`, `{"responseId":"one","modelVersion":"actual","predictions":[]}`, `{"responseId":"one","modelVersion":"actual","generatedImages":[]}`, `{"responseId":42,"modelVersion":"actual"}`, `{"responseId":"one","responseId":"two","modelVersion":"actual"}`, `{"responseId":"one","modelVersion":"actual"}garbage`, `data: {"responseId":"one"`, strings.Repeat("x", maxPayload+1), strings.Repeat("data: {}\n\n", 65)} {
		tr := New()
		a := raw("gemini", "openai", "one", "actual")
		a.Body = []byte(body)
		none(t, tr.ObserveRaw(a))
		for _, o := range tr.ObserveResponse(response("openai", "one", "h")) {
			if !o.Ambiguous {
				t.Fatalf("invalid raw became trusted: %+v", o)
			}
		}
	}
	for _, kind := range []string{"source", "dest", "original", "auth", "header", "body-id", "downstream-only"} {
		t.Run(kind, func(t *testing.T) {
			tr := New()
			a := raw("gemini", "openai", "one", "actual")
			r := response("openai", "one", "h")
			switch kind {
			case "source":
				a.FromFormat = "openai"
			case "dest":
				a.ToFormat = "openai-responses"
			case "original":
				a.OriginalRequest = nil
			case "auth":
				r.Metadata = nil
			case "header":
				r.ResponseHeaders = nil
			case "body-id":
				r.Body = []byte(`{"model":"actual"}`)
			case "downstream-only":
				a.Body = nil
			}
			none(t, tr.ObserveRaw(a))
			none(t, tr.ObserveResponse(r))
		})
	}
}
func TestCacheCapacityExpiryAndCapacityConflict(t *testing.T) {
	tr := New()
	tr.limit = 2
	now := time.Unix(10, 0)
	tr.now = func() time.Time { return now }
	for i := 0; i < 8; i++ {
		id := fmt.Sprint(i)
		tr.ObserveRaw(raw("gemini", "openai", id, "actual"))
		tr.ObserveResponse(response("openai", id, id))
	}
	if len(tr.entries) > 2 || len(tr.bridges) > 2 || len(tr.keys) > 2 {
		t.Fatal("unbounded maps")
	}
	// An identity change must revoke retained evidence even if the new entry
	// cannot be inserted due to the capacity bound.
	r := response("openai", "new", "0")
	check(t, tr.ObserveResponse(r), CorrelationKey("auth", r.ResponseHeaders), "", true)
	now = now.Add(retention + time.Second)
	tr.ObserveRaw(raw("gemini", "openai", "fresh", "actual"))
	if len(tr.entries) != 1 || len(tr.bridges) != 0 || len(tr.keys) != 0 {
		t.Fatal("expiry did not prune")
	}
	none(t, tr.ObserveResponse(response("openai", "1", "1")))
}
func TestFullBridgeCacheStillRevokesExistingKey(t *testing.T) {
	tr := New()
	tr.limit = 2
	tr.ObserveRaw(raw("gemini", "openai", "one", "actual"))
	r := response("openai", "one", "header")
	check(t, tr.ObserveResponse(r), CorrelationKey("auth", r.ResponseHeaders), "actual", false)
	init := pluginapi.StreamChunkInterceptRequest{RequestID: "fill", SourceFormat: "openai", OriginalRequest: original, ResponseHeaders: http.Header{"X-Request-ID": {"fill"}}, Metadata: r.Metadata}
	none(t, tr.ObserveStream(init))
	if len(tr.bridges) != tr.limit {
		t.Fatal("did not saturate bridges")
	}
	r.RequestID = "new-execution"
	r.Body = []byte(`{"id":"different"}`)
	check(t, tr.ObserveResponse(r), CorrelationKey("auth", r.ResponseHeaders), "", true)
	none(t, tr.ObserveResponse(r))
}
func TestStatusCodeAndBoundedJSON(t *testing.T) {
	for _, status := range []int{199, 300, 400, 500} {
		tr := New()
		tr.ObserveRaw(raw("gemini", "openai", "one", "actual"))
		r := response("openai", "one", "h")
		r.StatusCode = status
		none(t, tr.ObserveResponse(r))
	}
	for _, body := range []string{`{"a":` + strings.Repeat("[", 34) + "0" + strings.Repeat("]", 34) + "}", `{"a":[` + strings.Repeat("0,", 65536) + "0]}", `{"a":true,"nested":{"id":"one","id":"two"}}`} {
		if len(documents([]byte(body))) != 0 {
			t.Fatal("accepted invalid/deep JSON")
		}
	}
}
func TestActiveStreamRetainsEvidenceBeyondTTL(t *testing.T) {
	tr := New()
	now := time.Unix(100, 0)
	tr.now = func() time.Time { return now }
	a := raw("gemini", "claude", "one", "actual")
	tr.ObserveRaw(a)
	r := response("claude", "one", "h")
	init := pluginapi.StreamChunkInterceptRequest{RequestID: "active", SourceFormat: "claude", OriginalRequest: original, ResponseHeaders: r.ResponseHeaders, Metadata: r.Metadata, Body: r.Body}
	check(t, tr.ObserveStream(init), CorrelationKey("auth", r.ResponseHeaders), "actual", false)
	// Typical Claude content deltas have no message ID and omit all header-init scope.
	delta := pluginapi.StreamChunkInterceptRequest{RequestID: "active", Body: []byte(`{"type":"content_block_delta","delta":{"type":"text_delta","text":"hello"}}`)}
	for i := 0; i < 8; i++ {
		now = now.Add(retention / 3)
		none(t, tr.ObserveStream(delta))
	}
	check(t, tr.ObserveRaw(raw("gemini", "claude", "one", "conflicting")), CorrelationKey("auth", r.ResponseHeaders), "", true)
	none(t, tr.ObserveRaw(a))
	// Once inactive, cache cleanup alone does not revoke historical observations.
	now = now.Add(retention + time.Second)
	none(t, tr.ObserveStream(delta))
	if len(tr.entries) != 0 || len(tr.bridges) != 0 || len(tr.keys) != 0 {
		t.Fatal("inactive entries retained")
	}
}
func TestRepeatedRawRetainsEvidenceTargets(t *testing.T) {
	tr := New()
	now := time.Unix(100, 0)
	tr.now = func() time.Time { return now }
	a := raw("gemini", "openai", "one", "actual")
	tr.ObserveRaw(a)
	r := response("openai", "one", "h")
	tr.ObserveResponse(r)
	for i := 0; i < 5; i++ {
		now = now.Add(retention / 2)
		none(t, tr.ObserveRaw(a))
	}
	check(t, tr.ObserveRaw(raw("gemini", "openai", "one", "different")), CorrelationKey("auth", r.ResponseHeaders), "", true)
}
func TestUninspectableRawRevokesAndPoisonsScope(t *testing.T) {
	for _, bad := range []string{"{", strings.Repeat("x", maxPayload+1), `{"responseId":"one","responseId":"other","modelVersion":"actual"}`} {
		tr := New()
		a := raw("gemini", "openai", "one", "actual")
		tr.ObserveRaw(a)
		r := response("openai", "one", "h")
		tr.ObserveResponse(r)
		a.Body = []byte(bad)
		check(t, tr.ObserveRaw(a), CorrelationKey("auth", r.ResponseHeaders), "", true)
		none(t, tr.ObserveRaw(a))
		none(t, tr.ObserveRaw(raw("gemini", "openai", "one", "actual")))
		// Even a different native ID cannot restore confidence in the same scope.
		none(t, tr.ObserveRaw(raw("gemini", "openai", "two", "actual")))
		r = response("openai", "two", "h2")
		check(t, tr.ObserveResponse(r), CorrelationKey("auth", r.ResponseHeaders), "", true)
	}
}
func TestModelNormalizationPreservesExactIdentifiers(t *testing.T) {
	tr := New()
	a := raw("gemini", "openai", " native ", "actual-version")
	tr.ObserveRaw(a)
	r := response("openai", " native ", " header ")
	check(t, tr.ObserveResponse(r), CorrelationKey("auth", r.ResponseHeaders), "actual-version", false)
	none(t, tr.ObserveRaw(raw("gemini", "openai", " native ", "  actual-version  ")))
	none(t, tr.ObserveResponse(response("openai", "native", "other-header")))
}
func TestUnavailableModelMetadataIsNeutral(t *testing.T) {
	for _, source := range []string{"gemini", "antigravity"} {
		for _, field := range []string{"", `,"modelVersion":null`, `,"modelVersion":""`, `,"modelVersion":"   "`} {
			tr := New()
			now := time.Unix(100, 0)
			tr.now = func() time.Time { return now }
			a := raw(source, "openai", "one", "actual")
			tr.ObserveRaw(a)
			r := response("openai", "one", "h")
			tr.ObserveResponse(r)
			body := `{"responseId":"one","usageMetadata":{"totalTokenCount":1}` + field + "}"
			if source == "antigravity" {
				body = `{"response":` + body + "}"
			}
			a.Body = []byte(body)
			for i := 0; i < 4; i++ {
				now = now.Add(retention / 2)
				none(t, tr.ObserveRaw(a))
			}
			for _, e := range tr.entries {
				if e.ambiguous || e.model != "actual" {
					t.Fatalf("unavailable metadata changed evidence: %+v", e)
				}
			}
			check(t, tr.ObserveRaw(raw(source, "openai", "one", "different")), CorrelationKey("auth", r.ResponseHeaders), "", true)
		}
	}
}
func TestInvalidModelKnownIDRevokes(t *testing.T) {
	for _, model := range []string{`42`, `true`, fmt.Sprintf("%q", strings.Repeat("x", maxID+1)), `"bad\nmodel"`} {
		tr := New()
		tr.ObserveRaw(raw("gemini", "openai", "one", "actual"))
		r := response("openai", "one", "h")
		tr.ObserveResponse(r)
		a := raw("gemini", "openai", "one", "actual")
		a.Body = []byte(`{"responseId":"one","modelVersion":` + model + "}")
		check(t, tr.ObserveRaw(a), CorrelationKey("auth", r.ResponseHeaders), "", true)
		none(t, tr.ObserveRaw(raw("gemini", "openai", "one", "actual")))
	}
}
func TestUninspectableDownstreamAndNeutralFrames(t *testing.T) {
	tr := New()
	a := raw("gemini", "openai", "one", "actual")
	tr.ObserveRaw(a)
	r := response("openai", "one", "h")
	tr.ObserveResponse(r)
	for _, body := range []string{"", "   ", "data: [DONE]", "[DONE]", ": heartbeat"} {
		a.Body = []byte(body)
		none(t, tr.ObserveRaw(a))
		none(t, tr.ObserveStream(pluginapi.StreamChunkInterceptRequest{RequestID: r.RequestID, Body: []byte(body)}))
	}
	a = raw("gemini", "openai", "one", "actual")
	none(t, tr.ObserveRaw(a))
	check(t, tr.ObserveStream(pluginapi.StreamChunkInterceptRequest{RequestID: r.RequestID, Body: []byte("{")}), CorrelationKey("auth", r.ResponseHeaders), "", true)
	none(t, tr.ObserveRaw(a))
	none(t, tr.ObserveResponse(r))
}
func TestMalformedFirstDownstreamCallbackRevokesSuppliedKey(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, bad := range []string{"{", strings.Repeat("x", maxPayload+1)} {
			tr := New()
			a := raw("gemini", "openai", "one", "actual")
			tr.ObserveRaw(a)
			r := response("openai", "one", "h")
			tr.ObserveResponse(r)
			key := CorrelationKey("auth", r.ResponseHeaders)
			r.RequestID = "never-seen-before"
			r.Body = []byte(bad)
			var got []Observation
			if stream {
				got = tr.ObserveStream(pluginapi.StreamChunkInterceptRequest{RequestID: r.RequestID, SourceFormat: r.SourceFormat, OriginalRequest: r.OriginalRequest, ResponseHeaders: r.ResponseHeaders, Metadata: r.Metadata, Body: r.Body})
			} else {
				got = tr.ObserveResponse(r)
			}
			check(t, got, key, "", true)
			none(t, tr.ObserveRaw(a))
			none(t, tr.ObserveResponse(response("openai", "one", "h")))
		}
	}
	// A genuine empty header-init has no unreadable evidence and stays neutral.
	tr := New()
	r := response("openai", "one", "h")
	tr.ObserveRaw(raw("gemini", "openai", "one", "actual"))
	tr.ObserveResponse(r)
	none(t, tr.ObserveStream(pluginapi.StreamChunkInterceptRequest{RequestID: "new", SourceFormat: r.SourceFormat, OriginalRequest: r.OriginalRequest, ResponseHeaders: r.ResponseHeaders, Metadata: r.Metadata, ChunkIndex: pluginapi.StreamChunkHeaderInitIndex}))
}
func TestScopePoisonCapacityAndExpiry(t *testing.T) {
	tr := New()
	tr.limit = 2
	now := time.Unix(100, 0)
	tr.now = func() time.Time { return now }
	for i := 0; i < 4; i++ {
		a := raw("gemini", "openai", "one", "actual")
		a.OriginalRequest = []byte(fmt.Sprint("request", i))
		a.Body = []byte("{")
		none(t, tr.ObserveRaw(a))
	}
	if len(tr.scopes) > tr.limit || !now.Before(tr.poisonedUntil) {
		t.Fatal("poison saturation not fail-closed")
	}
	a := raw("gemini", "openai", "one", "actual")
	none(t, tr.ObserveRaw(a))
	r := response("openai", "one", "h")
	check(t, tr.ObserveResponse(r), CorrelationKey("auth", r.ResponseHeaders), "", true)
	now = now.Add(retention + time.Second)
	none(t, tr.ObserveRaw(a))
	check(t, tr.ObserveResponse(r), CorrelationKey("auth", r.ResponseHeaders), "actual", false)
}
func TestConcurrentAccess(t *testing.T) {
	tr := New()
	var wg sync.WaitGroup
	for i := 0; i < 48; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprint(i)
			for j := 0; j < 8; j++ {
				tr.ObserveRaw(raw("gemini", "openai", id, "actual"))
				tr.ObserveResponse(response("openai", id, id))
				tr.ObserveStream(pluginapi.StreamChunkInterceptRequest{RequestID: "execution-" + id, Body: []byte(fmt.Sprintf(`{"id":%q}`, id))})
			}
		}(i)
	}
	wg.Wait()
}
func TestSameBatchConflictCoalesced(t *testing.T) {
	tr := New()
	r := response("openai", "one", "h")
	none(t, tr.ObserveResponse(r))
	a := raw("gemini", "openai", "one", "actual")
	a.Body = []byte("data: " + string(a.Body) + "\n\ndata: " + string(raw("gemini", "openai", "one", "other").Body) + "\n\n")
	check(t, tr.ObserveRaw(a), CorrelationKey("auth", r.ResponseHeaders), "", true)
}
func TestConflictAcrossDestinations(t *testing.T) {
	tr := New()
	for _, f := range []string{"openai", "claude"} {
		tr.ObserveRaw(raw("gemini", f, "one", "actual"))
		tr.ObserveResponse(response(f, "one", f))
	}
	got := tr.ObserveRaw(raw("gemini", "openai", "one", "other"))
	if len(got) != 2 {
		t.Fatalf("want both revoked: %+v", got)
	}
	for _, o := range got {
		if !o.Ambiguous {
			t.Fatal(o)
		}
	}
}
