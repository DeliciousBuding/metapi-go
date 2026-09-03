package proxy

import (
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
)

func setupFailureCfg(keywords []string, emptyContentFail bool) {
	cfg, rt := config.Load(map[string]string{
		"PORT":                     "8080",
		"PROXY_EMPTY_CONTENT_FAIL": boolToString(emptyContentFail),
	})
	if len(keywords) > 0 {
		rt.ProxyErrorKeywords = keywords
	}
	rt.ProxyEmptyContentFailEnabled = emptyContentFail
	config.Set(cfg)
	config.SetRuntime(rt)
}

func boolToString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// judgeBuffered is the buffered-shaped call of the single content judge: these
// keyword / empty-content cases only have a raw body and a parsed usage
// summary, which is exactly the fact set the buffered dispatch path hands over.
// The verdict is a struct (not a pointer), so "no failure" reads as
// !result.Failed — there is no second nil-shaped vocabulary for the same idea.
func judgeBuffered(rawText string, usage *UsageSummary) UpstreamVerdict {
	return JudgeUpstreamContent(UpstreamContentFacts{RawText: rawText, Usage: usage})
}

func TestJudgeUpstreamContent_KeywordMatching(t *testing.T) {
	t.Run("matches error keyword", func(t *testing.T) {
		setupFailureCfg([]string{"overloaded", "blocked"}, false)

		result := judgeBuffered("The server is overloaded, please try later", nil)
		if !result.Failed {
			t.Fatal("expected failure detected via keyword")
		}
		if result.Status != 502 {
			t.Errorf("expected status 502, got %d", result.Status)
		}
		if result.Reason == "" {
			t.Error("expected reason message")
		}
	})

	t.Run("case-insensitive matching", func(t *testing.T) {
		setupFailureCfg([]string{"OVERLOADED"}, false)

		result := judgeBuffered("Server is overloaded", nil)
		if !result.Failed {
			t.Fatal("expected case-insensitive match")
		}
	})

	t.Run("multiple keywords, matches first", func(t *testing.T) {
		setupFailureCfg([]string{"error", "blocked", "unavailable"}, false)

		result := judgeBuffered("This API is blocked", nil)
		if !result.Failed {
			t.Fatal("expected keyword match")
		}
	})

	t.Run("no keyword match", func(t *testing.T) {
		setupFailureCfg([]string{"overloaded"}, false)

		result := judgeBuffered("Everything is fine", nil)
		if result.Failed {
			t.Errorf("expected no failure, got reason: %s", result.Reason)
		}
	})

	t.Run("empty keyword list", func(t *testing.T) {
		setupFailureCfg([]string{}, false)

		result := judgeBuffered("Some error occurred", nil)
		if result.Failed {
			t.Errorf("expected no failure with empty keywords, got reason: %s", result.Reason)
		}
	})

	t.Run("empty keyword (whitespace only) skips", func(t *testing.T) {
		setupFailureCfg([]string{"  ", "blocked"}, false)

		result := judgeBuffered("The user is blocked", nil)
		if !result.Failed {
			t.Fatal("expected match for non-empty keyword")
		}
	})

	t.Run("empty text input", func(t *testing.T) {
		setupFailureCfg([]string{"error"}, false)

		result := judgeBuffered("", nil)
		if result.Failed {
			t.Errorf("expected no failure for empty text, got: %s", result.Reason)
		}
	})

	t.Run("whitespace-only text input", func(t *testing.T) {
		setupFailureCfg([]string{"error"}, false)

		result := judgeBuffered("   \n  ", nil)
		if result.Failed {
			t.Errorf("expected no failure for whitespace text, got: %s", result.Reason)
		}
	})
}

func TestJudgeUpstreamContent_EmptyContent(t *testing.T) {
	t.Run("empty content fail enabled, no tokens, no output", func(t *testing.T) {
		setupFailureCfg([]string{}, true)

		result := judgeBuffered(`{"id":"test"}`, &UsageSummary{
			PromptTokens:     100,
			CompletionTokens: 0,
			TotalTokens:      100,
		})
		if !result.Failed {
			t.Fatal("expected failure for empty content")
		}
		if result.Status != 502 {
			t.Errorf("expected status 502, got %d", result.Status)
		}
	})

	t.Run("empty content fail enabled, has completion tokens", func(t *testing.T) {
		setupFailureCfg([]string{}, true)

		result := judgeBuffered("{}", &UsageSummary{
			CompletionTokens: 50,
			TotalTokens:      150,
		})
		if result.Failed {
			t.Errorf("expected no failure (has completion tokens)")
		}
	})

	t.Run("empty content fail enabled, nil usage", func(t *testing.T) {
		setupFailureCfg([]string{}, true)

		result := judgeBuffered(`{}`, nil)
		if !result.Failed {
			t.Fatal("expected failure (nil usage, no output)")
		}
	})

	t.Run("empty content fail disabled", func(t *testing.T) {
		setupFailureCfg([]string{}, false)

		result := judgeBuffered(`{}`, &UsageSummary{
			CompletionTokens: 0,
		})
		if result.Failed {
			t.Errorf("expected no failure when empty content fail disabled")
		}
	})
}

func TestDetectHasUpstreamOutput(t *testing.T) {
	tests := []struct {
		name string
		text string
		want bool
	}{
		{name: "empty text", text: "", want: false},
		{name: "whitespace only", text: "   \n  ", want: false},
		{name: "JSON with text content", text: `{"choices":[{"message":{"content":"hello"}}]}`, want: true},
		{name: "JSON with no content", text: `{"id":"chat-123","created":123}`, want: false},
		{name: "JSON with output_text", text: `{"output_text":"generated text"}`, want: true},
		{name: "JSON with tool_calls", text: `{"choices":[{"delta":{"tool_calls":[{"function":{"name":"test"}}]}}]}`, want: true},
		{name: "JSON with delta text", text: `{"choices":[{"delta":{"content":"partial"}}]}`, want: true},
		{name: "SSE with [DONE] only", text: "data: [DONE]\n", want: false},
		{name: "SSE with content events", text: "data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\ndata: [DONE]\n", want: true},
		{name: "SSE with non-JSON payload", text: "data: some raw text\n", want: true},
		{name: "plain text (no JSON, no SSE)", text: "This is a plain text response", want: true},
		{name: "JSON with output array containing text", text: `{"output":[{"type":"message","content":[{"type":"output_text","text":"result"}]}]}`, want: true},
		{name: "JSON with content parts", text: `{"content":[{"type":"text","text":"Hello world"}]}`, want: true},
		{name: "JSON with message refusal", text: `{"choices":[{"message":{"refusal":"I cannot do that"}}]}`, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := detectHasUpstreamOutput(tt.text); got != tt.want {
				t.Errorf("detectHasUpstreamOutput(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}
func TestJudgeUpstreamContent_Combined(t *testing.T) {
	t.Run("keyword takes priority over empty content", func(t *testing.T) {
		setupFailureCfg([]string{"quota"}, true)

		result := judgeBuffered("quota exceeded", &UsageSummary{
			CompletionTokens: 0,
		})
		if !result.Failed {
			t.Fatal("expected failure from keyword")
		}
		if result.Status != 502 {
			t.Errorf("expected status 502, got %d", result.Status)
		}
	})

	t.Run("no keyword, has content, empty content check skipped", func(t *testing.T) {
		setupFailureCfg([]string{}, true)

		result := judgeBuffered("This is fine", nil)
		if result.Failed {
			t.Errorf("expected no failure (plain text detected as output)")
		}
	})
}
func TestHasCompletionContentFromPayload(t *testing.T) {
	tests := []struct {
		name    string
		payload any
		want    bool
	}{
		{name: "nil payload", payload: nil, want: false},
		{name: "non-map payload", payload: []string{"test"}, want: false},
		{name: "empty map", payload: map[string]any{}, want: false},
		{name: "direct text field", payload: map[string]any{"text": "hello"}, want: true},
		{name: "empty text field", payload: map[string]any{"text": ""}, want: false},
		{name: "delta text", payload: map[string]any{"delta": "content"}, want: true},
		{name: "function_call key", payload: map[string]any{"function_call": map[string]any{"name": "test"}}, want: true},
		{name: "tool_calls array", payload: map[string]any{"tool_calls": []any{map[string]any{"name": "test"}}}, want: true},
		{name: "outputText field", payload: map[string]any{"outputText": "result"}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasCompletionContentFromPayload(tt.payload); got != tt.want {
				t.Errorf("hasCompletionContentFromPayload(%v) = %v, want %v", tt.payload, got, tt.want)
			}
		})
	}
}
