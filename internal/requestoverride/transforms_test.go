package requestoverride

import (
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

func TestTransformOptionsPreserveNativeProtocolStructure(t *testing.T) {
	options := TransformOptions{ForceArrayInputs: true, ForceArrayInstructions: true, ReplaceDeveloperRoleWithSystem: true}
	chat := []byte(`{"model":"m","messages":[{"role":"developer","content":"rule"},{"role":"user","content":"hello"}],"tools":[{"function":{"parameters":{"const":9007199254740993}}}]}`)
	out, err := BeforeProtocol(chat, "chat", options)
	if err != nil || gjson.GetBytes(out, "messages.0.role").String() != "system" || !strings.Contains(string(out), "9007199254740993") {
		t.Fatalf("chat=%s err=%v", out, err)
	}
	if gjson.GetBytes(chat, "messages.0.role").String() != "developer" {
		t.Fatal("shared request mutated")
	}
	out, err = BeforeProtocol([]byte(`{"instructions":"existing","input":[{"type":"message","role":"developer","content":[{"type":"input_text","text":"rule"}]},{"role":"user","content":"hello"}]}`), "responses", options)
	if err != nil || gjson.GetBytes(out, "instructions").String() != "existing\nrule" || len(gjson.GetBytes(out, "input").Array()) != 1 {
		t.Fatalf("responses=%s err=%v", out, err)
	}
	out, err = AfterProtocol([]byte(`{"model":"m","input":"hello"}`), "responses", options, false)
	if err != nil || !gjson.GetBytes(out, "input").IsArray() || gjson.GetBytes(out, "input.0.content.0.type").String() != "input_text" {
		t.Fatalf("response array=%s err=%v", out, err)
	}
	out, err = AfterProtocol([]byte(`{"system":"rule","messages":[{"role":"user","content":"hello"}]}`), "messages", options, false)
	if err != nil || gjson.GetBytes(out, "system.0.type").String() != "text" {
		t.Fatalf("system array=%s err=%v", out, err)
	}
}

func TestReasoningMappingSourceSerializerAndFallback(t *testing.T) {
	options := TransformOptions{ReasoningEffortMapping: []EffortMapping{{From: "high", To: "max"}, {From: "high", To: "low"}}}
	body := []byte(`{"reasoning_effort":"high"}`)
	out, err := AfterProtocol(body, "chat", options, true)
	if err != nil || gjson.GetBytes(out, "reasoning_effort").String() != "max" {
		t.Fatalf("first mapping did not win: %s %v", out, err)
	}
	out, err = AfterProtocol(body, "chat", options, false)
	if err != nil || string(out) != string(body) {
		t.Fatal("non-primary serializer applied general mapping")
	}
	ctx := Context{ReasoningEffort: "high", RequestHeader: map[string]string{"User-Agent": "claude-cli/2.0"}}
	out, err = ClaudeCodeMapping([]byte(`{"reasoning":{"effort":"high"}}`), "responses", options, ctx)
	if err != nil || gjson.GetBytes(out, "reasoning.effort").String() != "max" {
		t.Fatalf("Claude fallback=%s %v", out, err)
	}
	body = []byte(`{"reasoning":{"effort":"provider-adjusted"}}`)
	out, err = ClaudeCodeMapping(body, "responses", options, ctx)
	if err != nil || string(out) != string(body) {
		t.Fatal("provider override was overwritten")
	}
	ctx.RequestHeader["User-Agent"] = "other-client"
	body = []byte(`{"reasoning":{"effort":"high"}}`)
	out, err = ClaudeCodeMapping(body, "responses", options, ctx)
	if err != nil || string(out) != string(body) {
		t.Fatal("non-Claude fallback changed reasoning")
	}
}
