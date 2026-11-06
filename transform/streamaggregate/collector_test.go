package streamaggregate

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCollectorRejectsIncompleteAndErrorStreams(t *testing.T) {
	for _, tc := range []struct {
		name, protocol string
		data           []string
	}{
		{"chat-no-done", "chat", []string{`{"choices":[{"index":0,"delta":{"content":"partial"},"finish_reason":"stop"}]}`}},
		{"chat-no-finish", "chat", []string{`{"choices":[{"index":0,"delta":{"content":"partial"},"finish_reason":null}]}`, `[DONE]`}},
		{"chat-error", "chat", []string{`{"choices":[{"index":0,"delta":{"content":"partial"},"finish_reason":"stop"}]}`, `{"error":{"message":"failed"}}`, `[DONE]`}},
		{"chat-after-terminal", "chat", []string{`{"choices":[{"index":0,"delta":{"content":"partial"},"finish_reason":"stop"}]}`, `[DONE]`, `{"choices":[]}`}},
		{"chat-incomplete-tool-arguments", "chat", []string{`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"t","type":"function","function":{"name":"echo","arguments":"{\"n\":"}}]},"finish_reason":"tool_calls"}]}`, `[DONE]`}},
		{"chat-missing-tool-id", "chat", []string{`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"type":"function","function":{"name":"echo","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`, `[DONE]`}},
		{"chat-missing-tool-name", "chat", []string{`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"t","type":"function","function":{"arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`, `[DONE]`}},
		{"completion-no-done", "completions", []string{`{"choices":[{"index":0,"text":"partial","finish_reason":"stop"}]}`}},
		{"responses-incomplete", "responses", []string{`{"type":"response.incomplete","response":{"status":"incomplete","output":[]}}`}},
		{"responses-delta-only", "responses", []string{`{"type":"response.output_text.delta","delta":"partial"}`}},
		{"messages-open-block", "messages", []string{`{"type":"message_start","message":{"id":"m","content":[]}}`, `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":"partial"}}`, `{"type":"message_delta","delta":{"stop_reason":"end_turn"}}`, `{"type":"message_stop"}`}},
		{"messages-no-stop", "messages", []string{`{"type":"message_start","message":{"id":"m","content":[]}}`, `{"type":"message_delta","delta":{"stop_reason":"end_turn"}}`}},
		{"gemini-no-finish", "gemini", []string{`{"candidates":[{"index":0,"content":{"parts":[{"text":"partial"}]}}]}`}},
		{"gemini-one-unfinished", "gemini", []string{`{"candidates":[{"index":0,"finishReason":"STOP"},{"index":1,"content":{"parts":[{"text":"partial"}]}}]}`}},
		{"gemini-error-after-finish", "gemini", []string{`{"candidates":[{"index":0,"finishReason":"STOP"}]}`, `{"error":{"code":503}}`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, err := New(tc.protocol, 4096)
			if err != nil {
				t.Fatal(err)
			}
			for _, data := range tc.data {
				_ = c.Add("", []byte(data))
			}
			if raw, err := c.Result(); err == nil || raw != nil {
				t.Fatalf("partial stream became successful JSON: %s %v", raw, err)
			}
		})
	}
}

func TestCollectorPreservesNativeLimitedTerminals(t *testing.T) {
	for _, tc := range []struct{ protocol, frame, marker string }{
		{"responses", `{"type":"response.incomplete","response":{"id":"r","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[],"usage":{"input_tokens":2,"output_tokens":3}}}`, `"reason":"max_output_tokens"`},
		{"responses", `{"type":"response.incomplete","response":{"id":"r","status":"incomplete","incomplete_details":{"reason":"content_filter"},"output":[]}}`, `"reason":"content_filter"`},
		{"gemini", `{"promptFeedback":{"blockReason":"SAFETY","safetyRatings":[{"category":"HARM_CATEGORY_HATE_SPEECH","probability":"HIGH"}]},"usageMetadata":{"promptTokenCount":2},"modelVersion":"m"}`, `"blockReason":"SAFETY"`},
	} {
		t.Run(tc.protocol+tc.marker, func(t *testing.T) {
			c, _ := New(tc.protocol, 4096)
			if err := c.Add("", []byte(tc.frame)); err != nil {
				t.Fatal(err)
			}
			body, err := c.Result()
			if err != nil || !json.Valid(body) || !strings.Contains(string(body), tc.marker) {
				t.Fatalf("valid native terminal changed: %s %v", body, err)
			}
		})
	}
}

func TestCollectorPreservesNativeExtensionsAndIntegerTokens(t *testing.T) {
	c, _ := New("chat", 4096)
	frames := []string{
		`{"id":"c","created":9007199254740993,"vendor":{"n":9007199254740993},"choices":[{"index":0,"delta":{"role":"assistant","content":"a","tool_calls":[{"index":0,"id":"call","type":"function","function":{"name":"f","arguments":"{\"n\":"}}]},"logprobs":{"content":[{"token":"a","logprob":-0.1}]},"finish_reason":null}]}`,
		`{"choices":[{"index":0,"delta":{"content":"b","tool_calls":[{"index":0,"function":{"arguments":"9007199254740993}"}}]},"logprobs":{"content":[{"token":"b","logprob":-0.2}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5,"extra":9007199254740993}}`,
		`[DONE]`,
	}
	for _, frame := range frames {
		if err := c.Add("", []byte(frame)); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := c.Result()
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(raw) || !strings.Contains(string(raw), `"arguments":"{\"n\":9007199254740993}"`) || !strings.Contains(string(raw), `"content":"ab"`) || strings.Count(string(raw), "9007199254740993") != 4 {
		t.Fatalf("native fields or large integers changed: %s", raw)
	}
	var response struct {
		Choices []struct {
			Logprobs struct{ Content []json.RawMessage }
		}
	}
	_ = json.Unmarshal(raw, &response)
	if len(response.Choices) != 1 || len(response.Choices[0].Logprobs.Content) != 2 {
		t.Fatalf("logprobs lost deltas: %s", raw)
	}
}

func TestCollectorEnforcesAggregateByteLimit(t *testing.T) {
	for _, protocol := range []string{"chat", "completions", "responses", "messages", "gemini"} {
		t.Run(protocol, func(t *testing.T) {
			c, _ := New(protocol, 16)
			if err := c.Add("", []byte(`{"padding":"`+strings.Repeat("x", 32)+`"}`)); err == nil {
				t.Fatal("input exceeded byte budget")
			}
			if raw, err := c.Result(); err == nil || raw != nil {
				t.Fatal("overflow returned successful body")
			}
		})
	}
}
