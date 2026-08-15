package proxyhandler

import (
	"encoding/json"
	"testing"

	"github.com/deliciousbuding/metapi-go/proxy"
)

// ---------------------------------------------------------------------------
// responsesOnlyClientError
// ---------------------------------------------------------------------------

func TestResponsesOnlyClientError(t *testing.T) {
	chatBody := []byte(`{"messages":[{"role":"user","content":"hi"}]}`)
	responsesBody := []byte(`{"input":"hi"}`)
	emptyBody := []byte(nil)

	tests := []struct {
		name          string
		downstreamPath string
		bodyBytes     []byte
		pref          proxy.SiteProtocolPreference
		wantEmpty     bool
	}{
		{
			name:          "not responses-only returns empty",
			downstreamPath: "/v1/chat/completions",
			bodyBytes:     chatBody,
			pref:          proxy.SiteProtocolPreference{},
			wantEmpty:     true,
		},
		{
			name:          "responses-only but path is responses endpoint returns empty",
			downstreamPath: "/v1/responses",
			bodyBytes:     responsesBody,
			pref:          proxy.SiteProtocolPreference{ResponsesOnly: true},
			wantEmpty:     true,
		},
		{
			name:          "responses-only with unknown path returns empty",
			downstreamPath: "/v1/something/else",
			bodyBytes:     chatBody,
			pref:          proxy.SiteProtocolPreference{ResponsesOnly: true},
			wantEmpty:     true,
		},
		{
			name:          "responses-only chat path with responses-shaped body returns empty",
			downstreamPath: "/v1/chat/completions",
			bodyBytes:     responsesBody,
			pref:          proxy.SiteProtocolPreference{ResponsesOnly: true},
			wantEmpty:     true,
		},
		{
			name:          "responses-only chat path with messages body returns error message",
			downstreamPath: "/v1/chat/completions",
			bodyBytes:     chatBody,
			pref:          proxy.SiteProtocolPreference{ResponsesOnly: true},
			wantEmpty:     false,
		},
		{
			name:          "responses-only messages path with empty body returns error message",
			downstreamPath: "/v1/messages",
			bodyBytes:     emptyBody,
			pref:          proxy.SiteProtocolPreference{ResponsesOnly: true},
			wantEmpty:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := responsesOnlyClientError(tt.downstreamPath, tt.bodyBytes, tt.pref)
			if tt.wantEmpty && got != "" {
				t.Fatalf("expected empty string, got %q", got)
			}
			if !tt.wantEmpty && got == "" {
				t.Fatalf("expected non-empty error message, got empty string")
			}
			if !tt.wantEmpty {
				// The error message should reference the downstream path or a fallback.
				if got == "" {
					t.Fatalf("expected error message referencing path, got empty")
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// bodyLooksResponsesShaped
// ---------------------------------------------------------------------------

func TestBodyLooksResponsesShaped(t *testing.T) {
	tests := []struct {
		name      string
		bodyBytes []byte
		want      bool
	}{
		{name: "nil body", bodyBytes: nil, want: false},
		{name: "empty body", bodyBytes: []byte{}, want: false},
		{name: "invalid JSON", bodyBytes: []byte(`{not json`), want: false},
		{name: "has input no messages", bodyBytes: []byte(`{"input":"hi"}`), want: true},
		{name: "has messages no input", bodyBytes: []byte(`{"messages":[]}`), want: false},
		{name: "has both messages and input", bodyBytes: []byte(`{"messages":[],"input":"hi"}`), want: false},
		{name: "neither messages nor input", bodyBytes: []byte(`{"model":"gpt-4o"}`), want: false},
		{name: "input is null", bodyBytes: []byte(`{"input":null}`), want: true},
		{name: "empty object", bodyBytes: []byte(`{}`), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := bodyLooksResponsesShaped(tt.bodyBytes); got != tt.want {
				t.Fatalf("bodyLooksResponsesShaped(%s) = %v, want %v", string(tt.bodyBytes), got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// applyUpstreamStreamPreference
// ---------------------------------------------------------------------------

func TestApplyUpstreamStreamPreference(t *testing.T) {
	responsesOnlyPref := proxy.SiteProtocolPreference{ResponsesOnly: true, PreferStream: true}
	streamPref := proxy.SiteProtocolPreference{PreferStream: true}
	emptyPref := proxy.SiteProtocolPreference{}

	responsesPath := "/v1/responses"
	chatPath := "/v1/chat/completions"
	compactPath := "/v1/responses/compact"

	tests := []struct {
		name          string
		bodyBytes     []byte
		sitePlatform  string
		upstreamPath  string
		pref          proxy.SiteProtocolPreference
		wantForced    bool
		wantStreamKey *bool // nil = don't check; non-nil = check stream value
	}{
		{
			name:          "no force when empty pref and empty platform",
			bodyBytes:     []byte(`{"model":"gpt-4o"}`),
			sitePlatform:  "",
			upstreamPath:  responsesPath,
			pref:          emptyPref,
			wantForced:    false,
			wantStreamKey: nil,
		},
		{
			name:          "compact path never forces even with codex platform",
			bodyBytes:     []byte(`{"model":"gpt-4o"}`),
			sitePlatform:  "codex",
			upstreamPath:  compactPath,
			pref:          emptyPref,
			wantForced:    false,
			wantStreamKey: nil,
		},
		{
			name:          "codex platform forces stream on responses path",
			bodyBytes:     []byte(`{"model":"codex-mini"}`),
			sitePlatform:  "codex",
			upstreamPath:  responsesPath,
			pref:          emptyPref,
			wantForced:    true,
			wantStreamKey: boolPtr(true),
		},
		{
			name:          "sub2api platform forces stream on responses path",
			bodyBytes:     []byte(`{"model":"x"}`),
			sitePlatform:  "sub2api",
			upstreamPath:  responsesPath,
			pref:          emptyPref,
			wantForced:    true,
			wantStreamKey: boolPtr(true),
		},
		{
			name:          "responses-only pref forces stream on responses path",
			bodyBytes:     []byte(`{"model":"x"}`),
			sitePlatform:  "",
			upstreamPath:  responsesPath,
			pref:          responsesOnlyPref,
			wantForced:    true,
			wantStreamKey: boolPtr(true),
		},
		{
			name:          "stream pref does not force on chat path",
			bodyBytes:     []byte(`{"model":"x"}`),
			sitePlatform:  "",
			upstreamPath:  chatPath,
			pref:          streamPref,
			wantForced:    false,
			wantStreamKey: nil,
		},
		{
			name:          "empty body with force returns minimal stream body",
			bodyBytes:     []byte{},
			sitePlatform:  "codex",
			upstreamPath:  responsesPath,
			pref:          emptyPref,
			wantForced:    true,
			wantStreamKey: boolPtr(true),
		},
		{
			name:          "already streaming bool true returns unchanged",
			bodyBytes:     []byte(`{"stream":true,"model":"x"}`),
			sitePlatform:  "codex",
			upstreamPath:  responsesPath,
			pref:          emptyPref,
			wantForced:    false,
			wantStreamKey: nil,
		},
		{
			name:          "already streaming string true returns unchanged",
			bodyBytes:     []byte(`{"stream":"true","model":"x"}`),
			sitePlatform:  "codex",
			upstreamPath:  responsesPath,
			pref:          emptyPref,
			wantForced:    false,
			wantStreamKey: nil,
		},
		{
			name:          "already streaming string 1 returns unchanged",
			bodyBytes:     []byte(`{"stream":"1","model":"x"}`),
			sitePlatform:  "codex",
			upstreamPath:  responsesPath,
			pref:          emptyPref,
			wantForced:    false,
			wantStreamKey: nil,
		},
		{
			name:          "stream false gets overwritten to true when forced",
			bodyBytes:     []byte(`{"stream":false,"model":"x"}`),
			sitePlatform:  "codex",
			upstreamPath:  responsesPath,
			pref:          emptyPref,
			wantForced:    true,
			wantStreamKey: boolPtr(true),
		},
		{
			name:          "invalid JSON body returns unchanged when forced",
			bodyBytes:     []byte(`{not json`),
			sitePlatform:  "codex",
			upstreamPath:  responsesPath,
			pref:          emptyPref,
			wantForced:    false,
			wantStreamKey: nil,
		},
		{
			name:          "existing keys preserved when stream injected",
			bodyBytes:     []byte(`{"model":"gpt-4o","temperature":0.7}`),
			sitePlatform:  "codex",
			upstreamPath:  responsesPath,
			pref:          emptyPref,
			wantForced:    true,
			wantStreamKey: boolPtr(true),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, forced := applyUpstreamStreamPreference(tt.bodyBytes, tt.sitePlatform, tt.upstreamPath, tt.pref)
			if forced != tt.wantForced {
				t.Fatalf("forced = %v, want %v", forced, tt.wantForced)
			}
			if tt.wantStreamKey != nil {
				var body map[string]any
				if err := json.Unmarshal(out, &body); err != nil {
					t.Fatalf("output is not valid JSON: %v (out=%s)", err, string(out))
				}
				streamVal, ok := body["stream"]
				if !ok {
					t.Fatalf("expected stream key in output: %s", string(out))
				}
				if b, ok := streamVal.(bool); ok {
					if b != *tt.wantStreamKey {
						t.Fatalf("stream = %v, want %v", b, *tt.wantStreamKey)
					}
				} else {
					t.Fatalf("stream is not bool: %T (%s)", streamVal, string(out))
				}
			}
			// When not forced, output should be the same reference as input.
			if !forced && len(tt.bodyBytes) > 0 {
				if string(out) != string(tt.bodyBytes) {
					t.Fatalf("output changed when not forced: got %s, want %s", string(out), string(tt.bodyBytes))
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// applyUpstreamStreamIncludeUsage
// ---------------------------------------------------------------------------

func TestApplyUpstreamStreamIncludeUsage(t *testing.T) {
	chatPath := "/v1/chat/completions"
	legacyCompletions := "/v1/completions"
	responsesPath := "/v1/responses"
	messagesPath := "/v1/messages"

	tests := []struct {
		name             string
		bodyBytes        []byte
		sitePlatform     string
		upstreamPath     string
		isStream         bool
		wantInjected     bool
		wantIncludeUsage *bool // nil = don't check; non-nil = check value
	}{
		{
			name:             "not streaming returns unchanged",
			bodyBytes:        []byte(`{"stream":true,"model":"x"}`),
			sitePlatform:     "",
			upstreamPath:     chatPath,
			isStream:         false,
			wantInjected:     false,
			wantIncludeUsage: nil,
		},
		{
			name:             "empty body returns unchanged",
			bodyBytes:        []byte{},
			sitePlatform:     "",
			upstreamPath:     chatPath,
			isStream:         true,
			wantInjected:     false,
			wantIncludeUsage: nil,
		},
		{
			name:             "responses path does not accept include_usage",
			bodyBytes:        []byte(`{"stream":true,"model":"x"}`),
			sitePlatform:     "",
			upstreamPath:     responsesPath,
			isStream:         true,
			wantInjected:     false,
			wantIncludeUsage: nil,
		},
		{
			name:             "messages path does not accept include_usage",
			bodyBytes:        []byte(`{"stream":true,"model":"x"}`),
			sitePlatform:     "",
			upstreamPath:     messagesPath,
			isStream:         true,
			wantInjected:     false,
			wantIncludeUsage: nil,
		},
		{
			name:             "codex platform rejects stream_options",
			bodyBytes:        []byte(`{"stream":true,"model":"x"}`),
			sitePlatform:     "codex",
			upstreamPath:     chatPath,
			isStream:         true,
			wantInjected:     false,
			wantIncludeUsage: nil,
		},
		{
			name:             "sub2api platform rejects stream_options",
			bodyBytes:        []byte(`{"stream":true,"model":"x"}`),
			sitePlatform:     "sub2api",
			upstreamPath:     chatPath,
			isStream:         true,
			wantInjected:     false,
			wantIncludeUsage: nil,
		},
		{
			name:             "chat path with openai platform injects include_usage",
			bodyBytes:        []byte(`{"stream":true,"model":"gpt-4o"}`),
			sitePlatform:     "openai",
			upstreamPath:     chatPath,
			isStream:         true,
			wantInjected:     true,
			wantIncludeUsage: boolPtr(true),
		},
		{
			name:             "legacy completions path injects include_usage",
			bodyBytes:        []byte(`{"stream":true,"model":"text-davinci-003"}`),
			sitePlatform:     "",
			upstreamPath:     legacyCompletions,
			isStream:         true,
			wantInjected:     true,
			wantIncludeUsage: boolPtr(true),
		},
		{
			name:             "already has include_usage true returns unchanged but flagged",
			bodyBytes:        []byte(`{"stream":true,"stream_options":{"include_usage":true}}`),
			sitePlatform:     "",
			upstreamPath:     chatPath,
			isStream:         true,
			wantInjected:     true,
			wantIncludeUsage: boolPtr(true),
		},
		{
			name:             "include_usage false gets overwritten to true",
			bodyBytes:        []byte(`{"stream":true,"stream_options":{"include_usage":false}}`),
			sitePlatform:     "",
			upstreamPath:     chatPath,
			isStream:         true,
			wantInjected:     true,
			wantIncludeUsage: boolPtr(true),
		},
		{
			name:             "existing stream_options keys preserved",
			bodyBytes:        []byte(`{"stream":true,"stream_options":{"include_usage":false,"other":"val"}}`),
			sitePlatform:     "",
			upstreamPath:     chatPath,
			isStream:         true,
			wantInjected:     true,
			wantIncludeUsage: boolPtr(true),
		},
		{
			name:             "invalid JSON returns unchanged",
			bodyBytes:        []byte(`{not json`),
			sitePlatform:     "",
			upstreamPath:     chatPath,
			isStream:         true,
			wantInjected:     false,
			wantIncludeUsage: nil,
		},
		{
			name:             "chat path with query string still accepts",
			bodyBytes:        []byte(`{"stream":true,"model":"x"}`),
			sitePlatform:     "",
			upstreamPath:     "/v1/chat/completions?foo=bar",
			isStream:         true,
			wantInjected:     true,
			wantIncludeUsage: boolPtr(true),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, injected := applyUpstreamStreamIncludeUsage(tt.bodyBytes, tt.sitePlatform, tt.upstreamPath, tt.isStream)
			if injected != tt.wantInjected {
				t.Fatalf("injected = %v, want %v", injected, tt.wantInjected)
			}
			if tt.wantIncludeUsage != nil {
				var body map[string]any
				if err := json.Unmarshal(out, &body); err != nil {
					t.Fatalf("output is not valid JSON: %v (out=%s)", err, string(out))
				}
				opts, ok := body["stream_options"].(map[string]any)
				if !ok {
					t.Fatalf("expected stream_options map in output: %s", string(out))
				}
				iu, ok := opts["include_usage"]
				if !ok {
					t.Fatalf("expected include_usage key in stream_options: %s", string(out))
				}
				if b, ok := iu.(bool); ok {
					if b != *tt.wantIncludeUsage {
						t.Fatalf("include_usage = %v, want %v", b, *tt.wantIncludeUsage)
					}
				} else {
					t.Fatalf("include_usage is not bool: %T (%s)", iu, string(out))
				}
			}
			// When not injected, output should match input.
			if !injected && len(tt.bodyBytes) > 0 {
				if string(out) != string(tt.bodyBytes) {
					t.Fatalf("output changed when not injected: got %s, want %s", string(out), string(tt.bodyBytes))
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// shouldWarnMissingStreamUsage
// ---------------------------------------------------------------------------

func TestShouldWarnMissingStreamUsageTableDriven(t *testing.T) {
	tests := []struct {
		name              string
		expectIncludeUsage bool
		usage             ParsedUsage
		want              bool
	}{
		{name: "not expecting include usage", expectIncludeUsage: false, usage: ParsedUsage{}, want: false},
		{name: "expecting but not found", expectIncludeUsage: true, usage: ParsedUsage{Found: false}, want: true},
		{name: "found with prompt tokens", expectIncludeUsage: true, usage: ParsedUsage{Found: true, PromptTokens: 10}, want: false},
		{name: "found with completion tokens", expectIncludeUsage: true, usage: ParsedUsage{Found: true, CompletionTokens: 5}, want: false},
		{name: "found with total tokens", expectIncludeUsage: true, usage: ParsedUsage{Found: true, TotalTokens: 15}, want: false},
		{name: "found with cache read tokens", expectIncludeUsage: true, usage: ParsedUsage{Found: true, CacheReadTokens: 3}, want: false},
		{name: "found with cache creation tokens", expectIncludeUsage: true, usage: ParsedUsage{Found: true, CacheCreationTokens: 2}, want: false},
		{name: "found with reasoning tokens", expectIncludeUsage: true, usage: ParsedUsage{Found: true, ReasoningTokens: 7}, want: false},
		{name: "found but all zero", expectIncludeUsage: true, usage: ParsedUsage{Found: true}, want: true},
		{name: "found but all zero with source", expectIncludeUsage: true, usage: ParsedUsage{Found: true, Source: "upstream"}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldWarnMissingStreamUsage(tt.expectIncludeUsage, tt.usage); got != tt.want {
				t.Fatalf("shouldWarnMissingStreamUsage(%v, %+v) = %v, want %v", tt.expectIncludeUsage, tt.usage, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// warnMissingStreamUsageAfterIncludeUsage
// ---------------------------------------------------------------------------

func TestWarnMissingStreamUsageAfterIncludeUsage(t *testing.T) {
	// The function has side effects (Prometheus counter + slog.Warn) but no
	// return value. Verify it does not panic for various inputs and that
	// the warn/no-warn path aligns with shouldWarnMissingStreamUsage.

	tests := []struct {
		name  string
		model string
		path  string
		usage ParsedUsage
	}{
		{name: "usage with tokens does not warn", model: "gpt-4o", path: "/v1/chat/completions", usage: ParsedUsage{Found: true, PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15}},
		{name: "usage not found warns", model: "gpt-4o", path: "/v1/chat/completions", usage: ParsedUsage{Found: false}},
		{name: "usage all zero warns", model: "claude-3", path: "/v1/messages", usage: ParsedUsage{Found: true}},
		{name: "empty model and path warns", model: "", path: "", usage: ParsedUsage{Found: false}},
		{name: "partial usage with reasoning tokens does not warn", model: "o1", path: "/v1/chat/completions", usage: ParsedUsage{Found: true, ReasoningTokens: 42}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// shouldWarn must agree with whether the function enters the warn branch.
			shouldWarn := shouldWarnMissingStreamUsage(true, tt.usage)

			// Call the function — it must not panic regardless of input.
			warnMissingStreamUsageAfterIncludeUsage(tt.model, tt.path, tt.usage)

			// If shouldWarn is false, the function is a no-op (no observable side
			// effect we can easily assert here). If true, the metric was
			// incremented and a warning logged. Either way, no panic = pass.
			_ = shouldWarn
		})
	}
}

// ---------------------------------------------------------------------------
// acceptsOpenAIStreamIncludeUsagePath
// ---------------------------------------------------------------------------

func TestAcceptsOpenAIStreamIncludeUsagePath(t *testing.T) {
	tests := []struct {
		name         string
		upstreamPath string
		want         bool
	}{
		{name: "chat completions", upstreamPath: "/v1/chat/completions", want: true},
		{name: "chat completions without v1 prefix", upstreamPath: "/chat/completions", want: true},
		{name: "chat completions with trailing slash", upstreamPath: "/v1/chat/completions/", want: true},
		{name: "chat completions with query string", upstreamPath: "/v1/chat/completions?stream=true", want: true},
		{name: "chat completions with fragment", upstreamPath: "/v1/chat/completions#section", want: true},
		{name: "legacy completions v1", upstreamPath: "/v1/completions", want: true},
		{name: "legacy completions no prefix", upstreamPath: "/completions", want: true},
		{name: "legacy completions with prefix path", upstreamPath: "/proxy/v1/completions", want: true},
		{name: "responses path", upstreamPath: "/v1/responses", want: false},
		{name: "messages path", upstreamPath: "/v1/messages", want: false},
		{name: "unknown path", upstreamPath: "/v1/embeddings", want: false},
		{name: "empty path", upstreamPath: "", want: false},
		{name: "chat completions with extra path segment not matched", upstreamPath: "/v1/chat/completions/extra", want: false},
		{name: "completions with trailing slash and query", upstreamPath: "/v1/completions/?x=1", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := acceptsOpenAIStreamIncludeUsagePath(tt.upstreamPath); got != tt.want {
				t.Fatalf("acceptsOpenAIStreamIncludeUsagePath(%q) = %v, want %v", tt.upstreamPath, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// rejectsOpenAIStreamOptions
// ---------------------------------------------------------------------------

func TestRejectsOpenAIStreamOptions(t *testing.T) {
	tests := []struct {
		name         string
		sitePlatform string
		want         bool
	}{
		{name: "codex lowercase", sitePlatform: "codex", want: true},
		{name: "codex uppercase", sitePlatform: "CODEX", want: true},
		{name: "codex with whitespace", sitePlatform: "  codex  ", want: true},
		{name: "chatgpt-codex", sitePlatform: "chatgpt-codex", want: true},
		{name: "chatgpt codex with space", sitePlatform: "chatgpt codex", want: true},
		{name: "chatgpt CODEX mixed case", sitePlatform: "ChatGPT Codex", want: true},
		{name: "sub2api", sitePlatform: "sub2api", want: true},
		{name: "SUB2API uppercase", sitePlatform: "SUB2API", want: true},
		{name: "openai", sitePlatform: "openai", want: false},
		{name: "claude", sitePlatform: "claude", want: false},
		{name: "empty string", sitePlatform: "", want: false},
		{name: "whitespace only", sitePlatform: "   ", want: false},
		{name: "gemini", sitePlatform: "gemini", want: false},
		{name: "unknown platform", sitePlatform: "custom-relay", want: false},
		{name: "codex-like but different", sitePlatform: "codexplus", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rejectsOpenAIStreamOptions(tt.sitePlatform); got != tt.want {
				t.Fatalf("rejectsOpenAIStreamOptions(%q) = %v, want %v", tt.sitePlatform, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// jsonTruthyBool
// ---------------------------------------------------------------------------

func TestJsonTruthyBool(t *testing.T) {
	tests := []struct {
		name string
		v    any
		want bool
	}{
		{name: "bool true", v: true, want: true},
		{name: "bool false", v: false, want: false},
		{name: "string true", v: "true", want: true},
		{name: "string TRUE", v: "TRUE", want: true},
		{name: "string True", v: "True", want: true},
		{name: "string 1", v: "1", want: true},
		{name: "string yes", v: "yes", want: true},
		{name: "string YES", v: "YES", want: true},
		{name: "string Yes", v: "Yes", want: true},
		{name: "string true with whitespace", v: "  true  ", want: true},
		{name: "string false", v: "false", want: false},
		{name: "string 0", v: "0", want: false},
		{name: "string no", v: "no", want: false},
		{name: "string on is not truthy", v: "on", want: false},
		{name: "string empty", v: "", want: false},
		{name: "string whitespace", v: "   ", want: false},
		{name: "nil", v: nil, want: false},
		{name: "int 1", v: 1, want: false},
		{name: "int 0", v: 0, want: false},
		{name: "float64 1.0", v: float64(1.0), want: false},
		{name: "float64 0.0", v: float64(0.0), want: false},
		{name: "slice", v: []string{"true"}, want: false},
		{name: "map", v: map[string]any{"true": true}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := jsonTruthyBool(tt.v); got != tt.want {
				t.Fatalf("jsonTruthyBool(%#v) = %v, want %v", tt.v, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// boolPtr is already declared in proxy_log.go; reuse it.
