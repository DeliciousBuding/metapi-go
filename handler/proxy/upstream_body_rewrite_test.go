package proxyhandler

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/proxy"
)

// ---------------------------------------------------------------------------
// findTopLevelValue / scanner primitives
// ---------------------------------------------------------------------------

func TestFindTopLevelValue(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		key       string
		wantFound bool
		wantOK    bool
		wantValue string // raw value bytes when found
	}{
		{name: "first key", body: `{"model":"gpt-4o","x":1}`, key: "model", wantFound: true, wantOK: true, wantValue: `"gpt-4o"`},
		{name: "later key", body: `{"a":1,"stream":false}`, key: "stream", wantFound: true, wantOK: true, wantValue: `false`},
		{name: "absent key", body: `{"a":1}`, key: "stream", wantFound: false, wantOK: true},
		{name: "nested key not matched", body: `{"a":{"stream":true}}`, key: "stream", wantFound: false, wantOK: true},
		{name: "key inside string value", body: `{"a":"\"stream\":true","b":2}`, key: "stream", wantFound: false, wantOK: true},
		{name: "braces inside string", body: `{"a":"}}}{{","model":"x"}`, key: "model", wantFound: true, wantOK: true, wantValue: `"x"`},
		{name: "escaped quote in string", body: `{"a":"say \"hi\"","stream":true}`, key: "stream", wantFound: true, wantOK: true, wantValue: `true`},
		{name: "pretty printed", body: "  {\n\t\"model\" : \"gpt-4o\" ,\n\"b\" : [ 1 , 2 ]\n} ", key: "model", wantFound: true, wantOK: true, wantValue: `"gpt-4o"`},
		{name: "escaped top-level key aborts", body: "{\"\\u0073tream\":true}", key: "stream", wantFound: false, wantOK: false},
		{name: "escaped key after target ok", body: "{\"stream\":true,\"\\u0078\":1}", key: "stream", wantFound: true, wantOK: true, wantValue: `true`},
		{name: "non-object array", body: `[1,2,3]`, key: "model", wantFound: false, wantOK: false},
		{name: "malformed missing colon", body: `{"a" 1}`, key: "a", wantFound: false, wantOK: false},
		{name: "malformed dangling comma", body: `{"a":1,,}`, key: "a", wantFound: true, wantOK: true, wantValue: `1`},
		{name: "trailing garbage after early match", body: `{"a":1} garbage`, key: "a", wantFound: true, wantOK: true, wantValue: `1`},
		{name: "trailing garbage without match", body: `{"a":1} garbage`, key: "b", wantFound: false, wantOK: false},
		{name: "empty object", body: `{}`, key: "model", wantFound: false, wantOK: true},
		{name: "whitespace object", body: " \t{}\n", key: "model", wantFound: false, wantOK: true},
		{name: "number formats", body: `{"n":-1.5e10,"stream":true}`, key: "stream", wantFound: true, wantOK: true, wantValue: `true`},
		{name: "literal values", body: `{"a":null,"b":false,"stream":true}`, key: "stream", wantFound: true, wantOK: true, wantValue: `true`},
		{name: "first occurrence wins", body: `{"model":"a","x":1,"model":"b"}`, key: "model", wantFound: true, wantOK: true, wantValue: `"a"`},
		{name: "unterminated string", body: `{"a":"x}`, key: "a", wantFound: false, wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, found, ok := findTopLevelValue([]byte(tt.body), tt.key)
			if ok != tt.wantOK || found != tt.wantFound {
				t.Fatalf("findTopLevelValue = (found=%v, ok=%v), want (found=%v, ok=%v)", found, ok, tt.wantFound, tt.wantOK)
			}
			if found && string([]byte(tt.body)[s.start:s.end]) != tt.wantValue {
				t.Fatalf("value span = %q, want %q", []byte(tt.body)[s.start:s.end], tt.wantValue)
			}
		})
	}
}

func TestObjectBraces(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantOK    bool
		wantEmpty bool
	}{
		{name: "simple", body: `{"a":1}`, wantOK: true, wantEmpty: false},
		{name: "empty", body: `{}`, wantOK: true, wantEmpty: true},
		{name: "empty with space", body: `{ }`, wantOK: true, wantEmpty: true},
		{name: "padded", body: "\n {\"a\":1} \t", wantOK: true, wantEmpty: false},
		{name: "not object", body: `[1]`, wantOK: false},
		{name: "empty body", body: ``, wantOK: false},
		{name: "missing close", body: `{"a":1`, wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, empty, ok := objectBraces([]byte(tt.body))
			if ok != tt.wantOK || (ok && empty != tt.wantEmpty) {
				t.Fatalf("objectBraces(%q) = (empty=%v, ok=%v), want (empty=%v, ok=%v)", tt.body, empty, ok, tt.wantEmpty, tt.wantOK)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// swapModelInJSON
// ---------------------------------------------------------------------------

func TestSwapModelInJSON_ShortCircuitNoMapping(t *testing.T) {
	body := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}],"stream":true}`)
	out := swapModelInJSON(body, "gpt-4o")
	if !sameByteSlice(out, body) {
		t.Fatalf("expected zero-copy passthrough, got %s", out)
	}
	if n := testing.AllocsPerRun(10, func() { _ = swapModelInJSON(body, "gpt-4o") }); n != 0 {
		t.Fatalf("no-mapping swap allocates %.1f times, want 0", n)
	}
}

func TestSwapModelInJSON_MappedSplicesOnlyModel(t *testing.T) {
	body := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"keep <me> & bytes"}],"n":1.5}`)
	out := swapModelInJSON(body, "gpt-4o-upstream")
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("output not valid JSON: %v (%s)", err, out)
	}
	if got["model"] != "gpt-4o-upstream" {
		t.Fatalf("model = %v, want gpt-4o-upstream", got["model"])
	}
	// Non-model bytes stay verbatim (no HTML re-escaping, no key reordering).
	if !strings.Contains(string(out), `"keep <me> & bytes"`) {
		t.Fatalf("content bytes not preserved verbatim: %s", out)
	}
	if !strings.Contains(string(out), `"messages":[`) || !strings.Contains(string(out), `"n":1.5`) {
		t.Fatalf("unexpected rewrite beyond model: %s", out)
	}
}

func TestSwapModelInJSON_EdgeCases(t *testing.T) {
	// Empty body synthesizes a model-only object.
	if got := string(swapModelInJSON(nil, "m1")); got != `{"model":"m1"}` {
		t.Fatalf("empty body = %s", got)
	}
	// Body without top-level model gets the key inserted.
	out := swapModelInJSON([]byte(`{"contents":[]}`), "gemini-x")
	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("insert output invalid: %v (%s)", err, out)
	}
	if parsed["model"] != "gemini-x" {
		t.Fatalf("model = %v, want gemini-x", parsed["model"])
	}
	// Nested "model" only: top-level key still inserted.
	out = swapModelInJSON([]byte(`{"messages":[{"model":"inner"}]}`), "top")
	if err := json.Unmarshal(out, &parsed); err != nil || parsed["model"] != "top" {
		t.Fatalf("nested-only model: %v (%s)", err, out)
	}
	// Non-string model value is replaced.
	out = swapModelInJSON([]byte(`{"model":123,"x":1}`), "m2")
	if err := json.Unmarshal(out, &parsed); err != nil || parsed["model"] != "m2" {
		t.Fatalf("non-string model: %v (%s)", err, out)
	}
	// Malformed body: legacy fallback returns original bytes (unmarshal fails).
	bad := []byte(`{not json`)
	if got := swapModelInJSON(bad, "m3"); !sameByteSlice(got, bad) {
		t.Fatalf("malformed body changed: %s", got)
	}
	// Escaped top-level key: scanner declines, legacy fallback still rewrites.
	esc := []byte("{\"\\u006dodel\":\"old\",\"x\":1}")
	out = swapModelInJSON(esc, "new")
	if err := json.Unmarshal(out, &parsed); err != nil || parsed["model"] != "new" {
		t.Fatalf("escaped key model: %v (%s)", err, out)
	}
}

// ---------------------------------------------------------------------------
// rewriteUpstreamStreamFlags fast path vs legacy decode chain equivalence
// ---------------------------------------------------------------------------

// legacyStreamChain mirrors the pre-fix dispatch chain semantics.
func legacyStreamChain(body []byte, force, clientStream, injectUsage bool) ([]byte, bool, bool) {
	forced := false
	if force {
		body, forced = legacyApplyStreamPreference(body)
	}
	expect := false
	if injectUsage && (clientStream || forced) && len(body) > 0 {
		body, expect = legacyApplyStreamIncludeUsage(body)
	}
	return body, forced, expect
}

func jsonDeepEqual(t *testing.T, a, b []byte) {
	t.Helper()
	var da, db any
	if err := json.Unmarshal(a, &da); err != nil {
		t.Fatalf("invalid JSON A: %v (%s)", err, a)
	}
	if err := json.Unmarshal(b, &db); err != nil {
		t.Fatalf("invalid JSON B: %v (%s)", err, b)
	}
	if !reflect.DeepEqual(da, db) {
		t.Fatalf("semantic mismatch:\n fast:   %s\n legacy: %s", a, b)
	}
}

func TestRewriteUpstreamStreamFlags_MatchesLegacyChain(t *testing.T) {
	bodies := map[string]string{
		"plain streaming chat":     `{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"hi"}]}`,
		"stream false":             `{"model":"gpt-4o","stream":false,"messages":[]}`,
		"no stream key":            `{"model":"gpt-4o","messages":[]}`,
		"stream string true":       `{"stream":"true","model":"x"}`,
		"stream string 1":          `{"stream":"1","model":"x"}`,
		"stream string TRUE":       `{"stream":"TRUE","model":"x"}`,
		"stream number 1":          `{"stream":1,"model":"x"}`,
		"stream null":              `{"stream":null,"model":"x"}`,
		"opts include_usage true":  `{"stream":true,"stream_options":{"include_usage":true}}`,
		"opts include_usage false": `{"stream":true,"stream_options":{"include_usage":false,"foo":"bar"}}`,
		"opts include_usage str":   `{"stream":true,"stream_options":{"include_usage":"yes"}}`,
		"opts include_usage zero":  `{"stream":true,"stream_options":{"include_usage":0}}`,
		"opts empty object":        `{"stream":true,"stream_options":{}}`,
		"opts non-object":          `{"stream":true,"stream_options":"weird"}`,
		"opts null":                `{"stream":true,"stream_options":null}`,
		"opts array":               `{"stream":true,"stream_options":[1,2]}`,
		"nested stream keys":       `{"model":"x","messages":[{"stream":"in-msg","stream_options":{"include_usage":true}}]}`,
		"pretty printed":           "{\n  \"model\": \"x\",\n  \"stream\": false,\n  \"messages\": []\n}",
		"escapes in strings":       `{"model":"x","messages":[{"content":"{\"stream\": false } and \\\" }"}],"stream":true}`,
		"empty object":             `{}`,
	}
	flags := []struct {
		name        string
		force       bool
		client      bool
		injectUsage bool
	}{
		{"inject only", false, true, true},
		{"force only", true, false, false},
		{"force + inject", true, true, true},
		{"force + non-stream client", true, false, true},
		{"nothing", false, false, false},
		{"inject non-stream client", false, false, true},
	}
	for bName, body := range bodies {
		for _, f := range flags {
			t.Run(bName+"/"+f.name, func(t *testing.T) {
				in := []byte(body)
				fast, fForced, fExpect, ok := rewriteUpstreamStreamFlags(in, scanBodyStreamHints(in), f.force, f.client, f.injectUsage)
				if !ok {
					t.Fatalf("fast path declined a regular body")
				}
				lOut, lForced, lExpect := legacyStreamChain(in, f.force, f.client, f.injectUsage)
				if fForced != lForced {
					t.Fatalf("forced = %v, legacy %v (fast=%s legacy=%s)", fForced, lForced, fast, lOut)
				}
				if fExpect != lExpect {
					t.Fatalf("expectUsage = %v, legacy %v (fast=%s legacy=%s)", fExpect, lExpect, fast, lOut)
				}
				jsonDeepEqual(t, fast, lOut)
			})
		}
	}
}

func TestRewriteUpstreamStreamFlags_EmptyBody(t *testing.T) {
	// Force on empty body synthesizes a stream-only object (legacy parity).
	out, forced, expect, ok := rewriteUpstreamStreamFlags(nil, scanBodyStreamHints(nil), true, false, false)
	if !ok || !forced || expect || string(out) != `{"stream":true}` {
		t.Fatalf("empty+force = (%s, %v, %v, %v)", out, forced, expect, ok)
	}
	// Inject on empty body: unchanged, no expectation.
	out, forced, expect, ok = rewriteUpstreamStreamFlags(nil, scanBodyStreamHints(nil), false, true, true)
	if !ok || forced || expect || len(out) != 0 {
		t.Fatalf("empty+inject = (%s, %v, %v, %v)", out, forced, expect, ok)
	}
}

func TestRewriteUpstreamStreamFlags_IrregularDeclined(t *testing.T) {
	for _, body := range []string{`{not json`, `{"stream" 1}`, `[1,2]`, `{"stream":true`} {
		in := []byte(body)
		_, _, _, ok := rewriteUpstreamStreamFlags(in, scanBodyStreamHints(in), true, true, true)
		if ok {
			t.Fatalf("fast path accepted irregular body %q", body)
		}
	}
}

// Wrapper passthrough guarantees: unchanged bodies keep identical bytes.
func TestStreamWrappers_PassthroughIdentity(t *testing.T) {
	in := []byte(`{"model":"x","stream":true,"messages":[]}`)
	// No force gate -> identical slice.
	if out, forced := applyUpstreamStreamPreference(in, "", "/v1/responses", proxy.SiteProtocolPreference{}); forced || !sameByteSlice(out, in) {
		t.Fatalf("preference passthrough altered body (forced=%v)", forced)
	}
	// include_usage already true -> identical slice, expect=true.
	inOpts := []byte(`{"stream":true,"stream_options":{"include_usage":true,"foo":1}}`)
	out, expect := applyUpstreamStreamIncludeUsage(inOpts, "openai", "/v1/chat/completions", true)
	if !expect || !sameByteSlice(out, inOpts) {
		t.Fatalf("include_usage noop altered body (expect=%v)", expect)
	}
}
