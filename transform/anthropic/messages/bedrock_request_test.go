package messages

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestPrepareBedrockRequest(t *testing.T) {
	body := []byte(`{"model":"anthropic.claude","stream":true,"anthropic_version":"2023-06-01","anthropic_beta":["existing-beta","header-beta"],"max_tokens":100,"thinking":{"type":"enabled","budget_tokens":32},"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"private","signature":"opaque"},{"type":"tool_use","id":"tool_1","name":"lookup","input":{"key":"value"}}]}],"tools":[{"type":"web_search_20250305","name":"web_search","max_uses":2}],"cache_control":{"type":"ephemeral"},"future_field":{"opaque":9007199254740993}}`)
	result, err := PrepareBedrockRequest(body, []string{"header-beta, new-beta", "new-beta"})
	if err != nil {
		t.Fatal(err)
	}
	var before, after map[string]json.RawMessage
	_ = json.Unmarshal(body, &before)
	_ = json.Unmarshal(result, &after)
	if after["model"] != nil || after["stream"] != nil || string(after["anthropic_version"]) != `"bedrock-2023-05-31"` {
		t.Fatalf("selectors/version = %s", result)
	}
	var beta []string
	_ = json.Unmarshal(after["anthropic_beta"], &beta)
	if !reflect.DeepEqual(beta, []string{"existing-beta", "header-beta", "new-beta", "web-search-2025-03-05"}) {
		t.Fatalf("beta = %v", beta)
	}
	for _, key := range []string{"thinking", "messages", "tools", "cache_control", "future_field", "max_tokens"} {
		if string(before[key]) != string(after[key]) {
			t.Errorf("%s was changed: %s", key, after[key])
		}
	}
}

func TestPrepareBedrockRequestInvalid(t *testing.T) {
	for _, body := range []string{`null`, `[]`, `{`, `{"anthropic_beta":"flag"}`, `{"anthropic_beta":[1]}`, `{"tools":"search"}`} {
		if _, err := PrepareBedrockRequest([]byte(body), nil); err == nil {
			t.Errorf("accepted %s", body)
		}
	}
}

func TestPrepareBedrockRequestDoesNotInventBeta(t *testing.T) {
	body, err := PrepareBedrockRequest([]byte(`{"model":"m","stream":false,"messages":[],"anthropic_beta":null}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	_ = json.Unmarshal(body, &value)
	if _, ok := value["anthropic_beta"]; ok {
		t.Fatalf("unexpected beta: %s", body)
	}
}
