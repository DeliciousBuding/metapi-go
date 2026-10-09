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
		{`{"type":"response.completed","response":{"status":"completed"}}`},
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
