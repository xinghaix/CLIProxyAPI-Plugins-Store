// Package responsemodel joins verified upstream model evidence to native HTTP
// request-header identities. It never changes responses or billing identities.
// Only Gemini/Antigravity -> Gemini, Claude, OpenAI Chat and Responses are
// supported. Missing evidence, fragmented SSE, synthetic Imagen and unsupported
// protocols fail closed. No universal WebSocket correlation is implied.
package responsemodel

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

const (
	maxPayload = 1 << 20
	maxID      = 512
	maxEntries = 4096
	maxKeys    = 16
	retention  = 10 * time.Minute
)

// Observation is evidence only. Ambiguous revokes any earlier evidence for Key.
type Observation struct {
	Key        string
	Model      string
	EvidenceID string
	Ambiguous  bool
}

func digest(parts ...string) string {
	h := sha256.New()
	var n [8]byte
	for _, p := range parts {
		binary.BigEndian.PutUint64(n[:], uint64(len(p)))
		h.Write(n[:])
		h.Write([]byte(p))
	}
	return hex.EncodeToString(h.Sum(nil))
}
func valid(s string) bool {
	return len(s) > 0 && len(s) <= maxID && strings.TrimSpace(s) != "" && !strings.ContainsAny(s, "\r\n\x00")
}

// CorrelationKey hashes auth, canonical header name, and the exact header value
// using length-delimited fields. Priority is X-Request-ID, Request-ID, then
// X-Goog-Request-ID. Any malformed recognized header rejects the entire set.
// Equal duplicate casing is tolerated; multiple values or conflicting casing
// are not. Trace/session and request BODY IDs are never header identities.
func CorrelationKey(authID string, headers http.Header) string {
	if !valid(authID) {
		return ""
	}
	names := []string{"X-Request-Id", "Request-Id", "X-Goog-Request-Id"}
	values := make([]string, len(names))
	for k, vs := range headers {
		for i, name := range names {
			if strings.EqualFold(k, name) {
				if len(vs) != 1 || !valid(vs[0]) || strings.Contains(vs[0], ",") {
					return ""
				}
				if values[i] != "" && values[i] != vs[0] {
					return ""
				}
				values[i] = vs[0]
			}
		}
	}
	for i, v := range values {
		if v != "" {
			return digest(authID, names[i], v)
		}
	}
	return ""
}

type entry struct {
	evidence, model, scope string
	ambiguous              bool
	keys                   map[string]bool
	expires                time.Time
}
type bridge struct {
	scope, key, identity, format, authHash, headerHash string
	ambiguous                                          bool
	expires                                            time.Time
}
type keyState struct {
	identity, emitted string
	ambiguous         bool
	expires           time.Time
}

// Tracker is safe for concurrent use. New must be used to initialize it.
// Each cache is capped at 4096 entries with a ten-minute inactivity TTL. At capacity
// new identities are dropped (not evicted), preserving existing quarantines.
// At most sixteen associated keys are retained per response identity; further
// keys receive immediate ambiguity observations without being retained.
type Tracker struct {
	mu            sync.Mutex
	entries       map[string]*entry
	bridges       map[string]*bridge
	keys          map[string]*keyState
	now           func() time.Time
	limit         int
	nextPrune     time.Time
	scopes        map[string]time.Time
	poisonedUntil time.Time
}

func New() *Tracker {
	return &Tracker{entries: make(map[string]*entry), bridges: make(map[string]*bridge), keys: make(map[string]*keyState), now: time.Now, limit: maxEntries, scopes: make(map[string]time.Time)}
}
func (t *Tracker) prune(now time.Time) {
	if now.Before(t.nextPrune) {
		return
	}
	t.nextPrune = now.Add(retention)
	for s, expiry := range t.scopes {
		if !now.Before(expiry) {
			delete(t.scopes, s)
		} else if expiry.Before(t.nextPrune) {
			t.nextPrune = expiry
		}
	}
	for k, v := range t.entries {
		if now.Before(v.expires) && v.expires.Before(t.nextPrune) {
			t.nextPrune = v.expires
		}
		if !now.Before(v.expires) {
			delete(t.entries, k)
		}
	}
	for k, v := range t.bridges {
		if now.Before(v.expires) && v.expires.Before(t.nextPrune) {
			t.nextPrune = v.expires
		}
		if !now.Before(v.expires) {
			delete(t.bridges, k)
		}
	}
	for k, v := range t.keys {
		if now.Before(v.expires) && v.expires.Before(t.nextPrune) {
			t.nextPrune = v.expires
		}
		if !now.Before(v.expires) {
			delete(t.keys, k)
		}
	}
}
func client(f string) bool {
	return f == "openai" || f == "openai-response" || f == "claude" || f == "gemini"
}
func scope(body []byte, format string) string {
	if len(body) == 0 || len(body) > maxPayload || !client(format) {
		return ""
	}
	return digest(string(body), format)
}
func (t *Tracker) get(id, scope string, now time.Time) *entry {
	if e := t.entries[id]; e != nil {
		t.touch(e, now)
		return e
	}
	if len(t.entries) >= t.limit {
		return nil
	}
	e := &entry{keys: make(map[string]bool), expires: now.Add(retention), scope: scope, ambiguous: t.scopePoisoned(scope, now)}
	t.entries[id] = e
	return e
}

// touch retains active evidence and every already-associated revocation target.
// It stores no payloads and never grows the bounded association set.
func (t *Tracker) touch(e *entry, now time.Time) {
	if e == nil {
		return
	}
	e.expires = now.Add(retention)
	for key := range e.keys {
		if k := t.keys[key]; k != nil {
			k.expires = e.expires
		}
	}
}
func observations(e *entry) []Observation {
	if e == nil || (!e.ambiguous && e.evidence == "") {
		return nil
	}
	out := make([]Observation, 0, len(e.keys))
	for key := range e.keys {
		out = append(out, Observation{Key: key, Model: e.model, EvidenceID: e.evidence, Ambiguous: e.ambiguous})
	}
	return out
}
func (t *Tracker) scopePoisoned(scope string, now time.Time) bool {
	return now.Before(t.poisonedUntil) || now.Before(t.scopes[scope])
}

// With no inspectable native ID, every existing association in this exact
// request-hash/client-format scope is unsafe. Identical requests may therefore
// be conservatively quarantined together. Saturation falls back to bounded,
// temporary global quarantine rather than forgetting a poison marker.
func (t *Tracker) poisonScope(scope string, now time.Time) []Observation {
	if _, exists := t.scopes[scope]; exists || len(t.scopes) < t.limit {
		t.scopes[scope] = now.Add(retention)
	} else {
		t.poisonedUntil = now.Add(retention)
	}
	var out []Observation
	for _, e := range t.entries {
		if t.scopePoisoned(e.scope, now) {
			e.ambiguous = true
			t.touch(e, now)
			out = append(out, observations(e)...)
		}
	}
	for _, b := range t.bridges {
		if t.scopePoisoned(b.scope, now) {
			b.ambiguous = true
			out = append(out, Observation{Key: b.key, Ambiguous: true})
		}
	}
	return out
}
func (t *Tracker) poison(id string) []Observation {
	if e := t.entries[id]; e != nil {
		e.ambiguous = true
		return observations(e)
	}
	return nil
}

// ObserveRaw consumes the BEFORE-translator body, never the configured Model.
// EvidenceID hashes native response ID plus request hash and source format.
func (t *Tracker) ObserveRaw(r pluginapi.ResponseTransformRequest) (out []Observation) {
	s := scope(r.OriginalRequest, r.ToFormat)
	if s == "" || (r.FromFormat != "gemini" && r.FromFormat != "antigravity") {
		return nil
	}
	docs, inspectable := parsedDocuments(r.Body)
	t.mu.Lock()
	defer t.mu.Unlock()
	defer func() { out = t.publish(out) }()
	now := t.now()
	t.prune(now)
	if !inspectable {
		return t.poisonScope(s, now)
	}
	for _, doc := range docs {
		if r.FromFormat == "antigravity" {
			doc = object(doc, "response")
		}
		native, model := text(doc, "responseId"), text(doc, "modelVersion")
		if !valid(native) || strings.HasPrefix(strings.ToLower(native), "imagen-") || strings.HasPrefix(strings.ToLower(model), "imagen") {
			continue
		}
		if _, ok := doc["predictions"]; ok {
			continue
		}
		if _, ok := doc["generatedImages"]; ok {
			continue
		}
		translated := native
		if r.ToFormat == "openai-response" && !strings.HasPrefix(native, "resp_") {
			translated = "resp_" + native
		}
		id := digest(s, translated)
		e := t.entries[id]
		evidence := digest("raw", digest(string(r.OriginalRequest)), r.FromFormat, native)
		// Streaming providers may emit model metadata only in the first chunk.
		// Unavailable metadata is neutral; it never replaces or invents evidence.
		value := doc["modelVersion"]
		_, isString := value.(string)
		unavailable := value == nil || (isString && len(model) <= maxID && strings.TrimSpace(model) == "" && strings.IndexFunc(model, func(r rune) bool { return r < 32 || r == 127 }) < 0)
		if unavailable {
			if e != nil && e.evidence == evidence {
				t.touch(e, now)
			}
			continue
		}
		if !valid(model) {
			out = append(out, t.poison(id)...)
			for _, other := range t.entries {
				if other.evidence == evidence {
					other.ambiguous = true
					out = append(out, observations(other)...)
				}
			}
			continue
		}
		model = strings.TrimSpace(model)
		if strings.HasPrefix(strings.ToLower(model), "imagen") {
			continue
		}
		if e != nil && e.evidence == evidence && e.model == model {
			t.touch(e, now)
			out = append(out, observations(e)...)
			continue
		}
		conflict := e != nil && e.evidence != "" && (e.evidence != evidence || e.model != model)
		// Evidence spans destinations: conflicting source models revoke every
		// associated destination, not merely this callback's translated format.
		for _, other := range t.entries {
			if other.evidence == evidence && (other.model != model || other.ambiguous) {
				conflict = true
			}
		}
		if conflict {
			for _, other := range t.entries {
				if other.evidence == evidence || (e != nil && e.evidence != "" && other.evidence == e.evidence) {
					other.ambiguous = true
					out = append(out, observations(other)...)
				}
			}
		}
		e = t.get(id, s, now)
		if e == nil {
			continue
		}
		if conflict {
			e.ambiguous = true
		}
		if e.evidence == "" {
			e.evidence = evidence
			e.model = model
		}
		out = append(out, observations(e)...)
	}
	return out
}

// ObserveResponse bridges a translated response ID to a native HEADER key.
// Downstream model strings are deliberately ignored.
func (t *Tracker) ObserveResponse(r pluginapi.ResponseInterceptRequest) []Observation {
	if r.StatusCode != 0 && (r.StatusCode < 200 || r.StatusCode >= 300) {
		return nil
	}
	return t.observe(r.RequestID, r.SourceFormat, r.OriginalRequest, r.ResponseHeaders, r.Metadata, r.Body, false)
}

// ObserveStream caches header-init scope by execution RequestID. Later chunks
// may omit OriginalRequest, auth metadata, response headers and SourceFormat.
func (t *Tracker) ObserveStream(r pluginapi.StreamChunkInterceptRequest) []Observation {
	return t.observe(r.RequestID, r.SourceFormat, r.OriginalRequest, r.ResponseHeaders, r.Metadata, r.Body, true)
}
func (t *Tracker) observe(requestID, format string, original []byte, headers http.Header, metadata map[string]any, body []byte, stream bool) (out []Observation) {
	docs, inspectable := parsedDocuments(body)
	auth, authPresent := metadata["selected_auth_id"]
	authString, _ := auth.(string)
	key := CorrelationKey(authString, headers)
	headerHash := CorrelationKey("header", headers)
	t.mu.Lock()
	defer t.mu.Unlock()
	defer func() { out = t.publish(out) }()
	now := t.now()
	t.prune(now)
	var b *bridge
	rid := ""
	if valid(requestID) {
		rid = digest(requestID)
		b = t.bridges[rid]
	}
	if !inspectable {
		if b != nil {
			b.ambiguous = true
			out = append(out, t.poison(b.identity)...)
			out = append(out, Observation{Key: b.key, Ambiguous: true})
		}
		// A supplied authenticated native header identifies a revocation target
		// even on the first callback for a new (or absent) execution RequestID.
		if key != "" {
			if k := t.keys[key]; k != nil {
				out = append(out, t.poison(k.identity)...)
			}
			out = append(out, Observation{Key: key, Ambiguous: true})
			if s := scope(original, format); s != "" {
				out = append(out, t.poisonScope(s, now)...)
			}
		}
		return out
	}
	if stream && rid == "" {
		return nil
	}
	if b != nil && format == "" {
		format = b.format
	}
	s := scope(original, format)
	if b != nil {
		changed := (s != "" && s != b.scope) || (key != "" && key != b.key) || format != b.format
		if len(original) > 0 && s == "" {
			changed = true
		}
		if authPresent && (!valid(authString) || digest(authString) != b.authHash) {
			changed = true
		}
		if len(headers) > 0 && headerHash != b.headerHash {
			changed = true
		}
		if changed {
			b.ambiguous = true
			out = append(out, t.poison(b.identity)...)
			out = append(out, Observation{Key: b.key, Ambiguous: true})
			if key != "" {
				out = append(out, Observation{Key: key, Ambiguous: true})
			}
		}
		if !changed && len(docs) > 0 {
			b.expires = now.Add(retention)
			t.touch(t.entries[b.identity], now)
		}
		if s == "" {
			s = b.scope
		}
		if key == "" {
			key = b.key
		}
	}
	if s == "" || key == "" || !client(format) {
		return out
	}
	if b == nil && rid != "" {
		b = &bridge{scope: s, key: key, format: format, authHash: digest(authString), headerHash: headerHash, expires: now.Add(retention)}
		if len(t.bridges) >= t.limit {
			b.ambiguous = true
		} else {
			t.bridges[rid] = b
		}
	}
	for _, doc := range docs {
		for _, native := range downstreamIDs(doc, format) {
			if !valid(native) {
				continue
			}
			id := digest(s, native)
			// Detect changes even when a full cache cannot admit the new identity.
			if b != nil && b.identity != "" && b.identity != id {
				b.ambiguous = true
				out = append(out, t.poison(b.identity)...)
				out = append(out, Observation{Key: key, Ambiguous: true})
			}
			ks := t.keys[key]
			if ks != nil && ks.identity != "" && ks.identity != id {
				ks.ambiguous = true
				out = append(out, t.poison(ks.identity)...)
				out = append(out, Observation{Key: key, Ambiguous: true})
			}
			e := t.get(id, s, now)
			if e == nil {
				continue
			}
			if b != nil {
				if b.ambiguous {
					e.ambiguous = true
				}
				b.identity = id
			}
			// Multiple different header identities for one native response are unsafe.
			if len(e.keys) > 0 && !e.keys[key] {
				e.ambiguous = true
				out = append(out, observations(e)...)
			}
			if ks == nil {
				if len(t.keys) >= t.limit {
					continue
				}
				ks = &keyState{identity: id, expires: now.Add(retention)}
				t.keys[key] = ks
			} else if ks.identity == "" {
				ks.identity = id
			}
			if ks.ambiguous {
				e.ambiguous = true
			}
			if len(e.keys) < maxKeys || e.keys[key] {
				e.keys[key] = true
			} else {
				e.ambiguous = true
				out = append(out, Observation{Key: key, Ambiguous: true})
			}
			out = append(out, observations(e)...)
		}
	}
	return out
}

// publish coalesces a callback before emission, so a conflict in its final
// frame cannot emit an earlier positive result. Revocations are sticky and
// each key emits at most one positive observation and one revocation per TTL.
func (t *Tracker) publish(in []Observation) []Observation {
	merged := make(map[string]Observation)
	for _, o := range in {
		if o.Key != "" {
			old, ok := merged[o.Key]
			if !ok || !old.Ambiguous {
				merged[o.Key] = o
			}
		}
	}
	var out []Observation
	for key, o := range merged {
		k := t.keys[key]
		if k == nil {
			if len(t.keys) >= t.limit {
				continue
			}
			k = &keyState{expires: t.now().Add(retention)}
			t.keys[key] = k
		}
		if o.Ambiguous {
			k.ambiguous = true
		}
		if k.ambiguous {
			o.Ambiguous = true
			o.Model = ""
		}
		signature := digest(o.Model, o.EvidenceID)
		if o.Ambiguous {
			signature = "ambiguous"
			o.EvidenceID = ""
		}
		if k.emitted == signature {
			continue
		}
		if k.emitted != "" && signature != "ambiguous" {
			k.ambiguous = true
			o.Ambiguous = true
			o.Model = ""
			o.EvidenceID = ""
			signature = "ambiguous"
		}
		k.emitted = signature
		out = append(out, o)
	}
	return out
}
func downstreamIDs(doc map[string]any, format string) []string {
	switch format {
	case "openai":
		return []string{text(doc, "id")}
	case "gemini":
		return []string{text(doc, "responseId")}
	case "claude":
		if text(doc, "type") == "message_start" {
			return []string{text(object(doc, "message"), "id")}
		}
		return []string{text(doc, "id")}
	case "openai-response":
		if response := object(doc, "response"); response != nil {
			return []string{text(response, "id")}
		}
		return []string{text(doc, "id")}
	}
	return nil
}
func text(m map[string]any, k string) string           { s, _ := m[k].(string); return s }
func object(m map[string]any, k string) map[string]any { v, _ := m[k].(map[string]any); return v }

// documents accepts complete bounded JSON objects or complete SSE data frames.
// Duplicate JSON keys, excessive nesting, malformed/truncated frames and more
// than 64 documents reject the entire callback, rather than guessing identity.
func documents(body []byte) []map[string]any { docs, _ := parsedDocuments(body); return docs }
func parsedDocuments(body []byte) ([]map[string]any, bool) {
	if len(body) > maxPayload {
		return nil, false
	}
	body = bytes.TrimSpace(body)
	if len(body) == 0 || bytes.Equal(body, []byte("[DONE]")) {
		return nil, true
	}
	var parts [][]byte
	if body[0] == '{' {
		parts = [][]byte{body}
	} else {
		var data []byte
		for _, line := range bytes.Split(body, []byte("\n")) {
			line = bytes.TrimSuffix(line, []byte("\r"))
			if len(line) == 0 {
				if len(data) > 0 {
					parts = append(parts, data)
					data = nil
				}
				continue
			}
			if bytes.HasPrefix(line, []byte("data:")) {
				if len(data) > 0 {
					data = append(data, '\n')
				}
				data = append(data, bytes.TrimSpace(line[5:])...)
			} else if !(bytes.HasPrefix(line, []byte("event:")) || bytes.HasPrefix(line, []byte(":")) || bytes.HasPrefix(line, []byte("id:")) || bytes.HasPrefix(line, []byte("retry:"))) {
				return nil, false
			}
			if len(parts) > 64 {
				return nil, false
			}
		}
		if len(data) > 0 {
			parts = append(parts, data)
		}
	}
	if len(parts) > 64 {
		return nil, false
	}
	var out []map[string]any
	for _, p := range parts {
		if bytes.Equal(p, []byte("[DONE]")) {
			continue
		}
		d := json.NewDecoder(bytes.NewReader(p))
		d.UseNumber()
		nodes := 0
		v, err := decode(d, 0, &nodes)
		if err != nil {
			return nil, false
		}
		if _, err = d.Token(); err != io.EOF {
			return nil, false
		}
		m, ok := v.(map[string]any)
		if !ok {
			return nil, false
		}
		out = append(out, m)
	}
	return out, true
}
func decode(d *json.Decoder, depth int, nodes *int) (any, error) {
	*nodes++
	if depth > 32 || *nodes > 65536 {
		return nil, io.ErrUnexpectedEOF
	}
	tok, err := d.Token()
	if err != nil {
		return nil, err
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return tok, nil
	}
	switch delim {
	case '{':
		m := make(map[string]any)
		for d.More() {
			k, e := d.Token()
			if e != nil {
				return nil, e
			}
			s, ok := k.(string)
			if !ok {
				return nil, io.ErrUnexpectedEOF
			}
			if _, exists := m[s]; exists {
				return nil, io.ErrUnexpectedEOF
			}
			v, e := decode(d, depth+1, nodes)
			if e != nil {
				return nil, e
			}
			m[s] = v
		}
		_, err = d.Token()
		return m, err
	case '[':
		var a []any
		for d.More() {
			v, e := decode(d, depth+1, nodes)
			if e != nil {
				return nil, e
			}
			a = append(a, v)
		}
		_, err = d.Token()
		return a, err
	}
	return nil, io.ErrUnexpectedEOF
}
