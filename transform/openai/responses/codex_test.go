package responses

import (
	"bytes"
	"testing"
)

func TestCodexNativeReasoningAndLimits(t *testing.T) {
	raw := []byte(`{"model":"model","input":[{"type":"reasoning","id":"r","encrypted_content":"opaque","summary":[]}],"max_tokens":100,"store":true}`)
	got, err := PrepareCodexRequest(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got, []byte(`"encrypted_content":"opaque"`)) || bytes.Contains(got, []byte(`"content":`)) || bytes.Contains(got, []byte(`"max_tokens"`)) {
		t.Fatalf("reasoning altered: %s", got)
	}
}

func TestCodexCollectorNeverInventsTerminal(t *testing.T) {
	for _, events := range [][]string{
		{`{"type":"response.output_text.delta","delta":"partial"}`, `[DONE]`},
		{`{"type":"response.failed","response":{"error":{"message":"private upstream error"}}}`},
		{`{"type":"response.completed","response":{"status":"incomplete","output":[]}}`},
		{`{"type":"response.incomplete","response":{"status":"incomplete","output":[],"incomplete_details":{"reason":"max_output_tokens"}}}`},
		{`{"type":"response.completed","response":{"status":"completed"}}`},
		{`{"type":"response.completed","response":{"status":"completed","output":null}}`},
		{`{"type":"response.completed","response":{"status":"completed","output":{}}}`},
	} {
		collector := CodexResponseCollector{}
		for _, event := range events {
			_ = collector.AddData([]byte(event))
		}
		if body, err := collector.Result(); err == nil || body != nil {
			t.Fatal("incomplete stream became successful JSON")
		}
	}
	collector := CodexResponseCollector{Limit: 4}
	if collector.AddData([]byte(`{"type":"response.created"}`)) == nil {
		t.Fatal("unbounded stream accepted")
	}
}

func TestNativeIncompleteCollectorValidatesTerminalAndOutput(t *testing.T) {
	for _, tc := range []struct {
		name, response string
		accepted       bool
	}{
		{"token limit", `"status":"incomplete","output":[],"incomplete_details":{"reason":"max_output_tokens"}`, true},
		{"content filter", `"status":"incomplete","output":[],"incomplete_details":{"reason":"content_filter"}`, true},
		{"unknown reason", `"status":"incomplete","output":[],"incomplete_details":{"reason":"unknown"}`, false},
		{"wrong status", `"status":"completed","output":[],"incomplete_details":{"reason":"max_output_tokens"}`, false},
		{"upstream error", `"status":"incomplete","output":[],"error":{"message":"failed"},"incomplete_details":{"reason":"max_output_tokens"}`, false},
		{"null output", `"status":"incomplete","output":null,"incomplete_details":{"reason":"max_output_tokens"}`, false},
		{"object output", `"status":"incomplete","output":{},"incomplete_details":{"reason":"max_output_tokens"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			collector := CodexResponseCollector{AllowIncomplete: true}
			_ = collector.AddData([]byte(`{"type":"response.incomplete","response":{` + tc.response + `}}`))
			body, err := collector.Result()
			if (err == nil) != tc.accepted || !tc.accepted && body != nil {
				t.Fatalf("accepted=%t, body=%s, err=%v", tc.accepted, body, err)
			}
		})
	}
}
