package messages

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPrepareVertexRequest(t *testing.T) {
	body := []byte(`{"model":"claude-sonnet-4-5@20250929","stream":true,"max_tokens":100,"thinking":{"type":"enabled","budget_tokens":32},"future_field":{"opaque":9007199254740993},"messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`)
	out, err := PrepareVertexRequest(body)
	if err != nil {
		t.Fatal(err)
	}
	var after map[string]json.RawMessage
	if err := json.Unmarshal(out, &after); err != nil {
		t.Fatal(err)
	}
	if after["model"] != nil {
		t.Fatal("model must move to the URL")
	}
	if string(after["anthropic_version"]) != `"vertex-2023-10-16"` {
		t.Fatalf("anthropic_version: %s", after["anthropic_version"])
	}
	if string(after["stream"]) != "true" {
		t.Fatal("stream selector lost")
	}
	if !strings.Contains(string(after["future_field"]), "9007199254740993") {
		t.Fatal("unknown fields must pass through exactly")
	}
	if after["thinking"] == nil || after["messages"] == nil {
		t.Fatal("native fields dropped")
	}
}

func TestPrepareVertexRequestInvalid(t *testing.T) {
	for _, body := range []string{"", "[]", "null", "not-json"} {
		if _, err := PrepareVertexRequest([]byte(body)); err == nil {
			t.Fatalf("invalid body accepted: %s", body)
		}
	}
}
