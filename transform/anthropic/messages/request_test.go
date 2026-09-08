package messages_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/transform/anthropic/messages"
)

// Synthetic values with the schema captured from Claude Code 2.1.263. No
// provider, credentials, filesystem contents, or CLI runtime is needed here.
const nativeReadRequest = `{
  "model":"test-model", "max_tokens":32000, "stream":true,
  "thinking":{"type":"adaptive","display":"omitted"},
  "output_config":{"effort":"high"},
  "context_management":{"edits":[{"type":"clear_thinking_20251015","keep":"all"}]},
  "metadata":{"user_id":"synthetic-test-user"},
  "system":[{"type":"text","text":"Use the provided tools.","cache_control":{"type":"ephemeral","ttl":"1h"}}],
  "messages":[{"role":"user","content":[{"type":"text","text":"Read sample.txt."},{"type":"text","text":"Then summarize it."}]}],
  "tools":[{"name":"Read","description":"Read one text file","input_schema":{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","properties":{"file_path":{"type":"string"},"offset":{"type":"integer","maximum":9007199254740993}},"required":["file_path"],"additionalProperties":false}}]
}`

func readRequest(t *testing.T) map[string]any {
	t.Helper()
	return jsonObject(t, decodeJSON(t, []byte(nativeReadRequest)))
}

func TestToChatRequestRealClaudeCodeReadShape(t *testing.T) {
	t.Parallel()
	out, err := messages.ToChatRequest([]byte(nativeReadRequest))
	if err != nil {
		t.Fatal(err)
	}
	requireJSON(t, out, `{
  "model":"test-model", "max_tokens":32000, "stream":true,
  "stream_options":{"include_usage":true}, "reasoning_effort":"high",
  "messages":[
    {"role":"system","content":[{"type":"text","text":"Use the provided tools."}]},
    {"role":"user","content":[{"type":"text","text":"Read sample.txt."},{"type":"text","text":"Then summarize it."}]}
  ],
  "tools":[{"type":"function","function":{"name":"Read","description":"Read one text file","parameters":{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","properties":{"file_path":{"type":"string"},"offset":{"type":"integer","maximum":9007199254740993}},"required":["file_path"],"additionalProperties":false}}}]
}`)
}

func TestToChatRequestAdaptiveControls(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, thinking, config, context, effort string
	}{
		{"first real capture", `{"type":"adaptive"}`, `{"effort":"high"}`, `{"edits":[{"type":"clear_thinking_20251015","keep":"all"}]}`, "high"},
		{"adaptive default", `{"type":"adaptive"}`, `{}`, `{}`, "high"},
		{"low", `{"type":"adaptive","display":"omitted"}`, `{"effort":"low"}`, `{"edits":[]}`, "low"},
		{"medium", `{"type":"adaptive"}`, `{"effort":"medium"}`, `null`, "medium"},
		{"xhigh preserved", `{"type":"adaptive"}`, `{"effort":"xhigh"}`, `{}`, "xhigh"},
		{"explicit disabled", `{"type":"disabled"}`, `{}`, `{}`, "none"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := readRequest(t)
			req["thinking"], req["output_config"], req["context_management"] = json.RawMessage(tc.thinking), json.RawMessage(tc.config), json.RawMessage(tc.context)
			out, err := messages.ToChatRequest(marshalJSON(t, req))
			if err != nil {
				t.Fatal(err)
			}
			got := jsonObject(t, decodeJSON(t, out))
			if got["reasoning_effort"] != tc.effort {
				t.Errorf("reasoning_effort = %v, want %s", got["reasoning_effort"], tc.effort)
			}
			for _, field := range []string{"thinking", "output_config", "context_management"} {
				if _, ok := got[field]; ok {
					t.Errorf("native control %s leaked into Chat body", field)
				}
			}
		})
	}
}

func TestToChatRequestTextAndSampling(t *testing.T) {
	t.Parallel()
	out, err := messages.ToChatRequest([]byte(`{"model":"m","max_tokens":8,"system":"exact\n system ","messages":[{"role":"user","content":" question "}],"temperature":0,"top_p":1,"stream":false,"stop_sequences":[],"metadata":{},"cache_control":{"type":"ephemeral"}}`))
	if err != nil {
		t.Fatal(err)
	}
	requireJSON(t, out, `{"model":"m","max_tokens":8,"messages":[{"role":"system","content":"exact\n system "},{"role":"user","content":" question "}],"temperature":0,"top_p":1,"stream":false}`)
}

func TestToChatRequestToolChoice(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		choice, want string
		parallel     *bool
	}{
		{`{"type":"auto"}`, `"auto"`, nil},
		{`{"type":"any"}`, `"required"`, nil},
		{`{"type":"tool","name":"Read"}`, `{"type":"function","function":{"name":"Read"}}`, nil},
		{`{"type":"none"}`, `"none"`, nil},
		{`{"type":"auto","disable_parallel_tool_use":true}`, `"auto"`, new(false)},
		{`{"type":"any","disable_parallel_tool_use":false}`, `"required"`, new(true)},
	} {
		t.Run(tc.choice, func(t *testing.T) {
			req := readRequest(t)
			req["tool_choice"] = json.RawMessage(tc.choice)
			out, err := messages.ToChatRequest(marshalJSON(t, req))
			if err != nil {
				t.Fatal(err)
			}
			got := jsonObject(t, decodeJSON(t, out))
			requireJSON(t, marshalJSON(t, got["tool_choice"]), tc.want)
			parallel, present := got["parallel_tool_calls"]
			if tc.parallel == nil && present {
				t.Fatal("invented a parallel-tool policy")
			}
			if tc.parallel != nil && (!present || parallel != *tc.parallel) {
				t.Fatalf("parallel_tool_calls=%v; want %v", parallel, *tc.parallel)
			}
		})
	}
}

func TestToChatRequestParallelToolContinuation(t *testing.T) {
	t.Parallel()
	out, err := messages.ToChatRequest([]byte(`{
 "model":"m","max_tokens":128,
 "messages":[
  {"role":"user","content":"Read both."},
  {"role":"assistant","content":[
    {"type":"text","text":"Reading both files."},
    {"type":"tool_use","id":"call-a","name":"Read","input":{"file_path":"a.txt","offset":9007199254740993}},
    {"type":"tool_use","id":"call-b","name":"Read","input":{"file_path":"b.txt"}}
  ]},
  {"role":"user","content":[
    {"type":"tool_result","tool_use_id":"call-b","content":[{"type":"text","text":"line one\n"},{"type":"text","text":"第二行"}]},
    {"type":"tool_result","tool_use_id":"call-a","content":"","is_error":false},
    {"type":"text","text":"Compare them."}
  ]}
 ]}`))
	if err != nil {
		t.Fatal(err)
	}
	requireJSON(t, out, `{
 "model":"m","max_tokens":128,
 "messages":[
  {"role":"user","content":"Read both."},
  {"role":"assistant","content":[{"type":"text","text":"Reading both files."}],"tool_calls":[
    {"type":"function","id":"call-a","function":{"name":"Read","arguments":"{\"file_path\":\"a.txt\",\"offset\":9007199254740993}"}},
    {"type":"function","id":"call-b","function":{"name":"Read","arguments":"{\"file_path\":\"b.txt\"}"}}
  ]},
  {"role":"tool","tool_call_id":"call-b","content":[{"type":"text","text":"line one\n"},{"type":"text","text":"第二行"}]},
  {"role":"tool","tool_call_id":"call-a","content":""},
  {"role":"user","content":[{"type":"text","text":"Compare them."}]}
 ]}`)
}

func TestToolResponseCanBeUsedForNextRequest(t *testing.T) {
	t.Parallel()
	reply, err := messages.FromChatResponse([]byte(`{"id":"chatcmpl-r","model":"m","choices":[{"index":0,"message":{"role":"assistant","content":null,"tool_calls":[{"id":"tool-r","type":"function","function":{"name":"Read","arguments":"{\"file_path\":\"sample.txt\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":12,"completion_tokens":5}}`))
	if err != nil {
		t.Fatal(err)
	}
	content := jsonObject(t, decodeJSON(t, reply))["content"]
	req := map[string]any{"model": "m", "max_tokens": 128, "messages": []any{
		map[string]any{"role": "user", "content": "Read sample.txt."},
		map[string]any{"role": "assistant", "content": content},
		map[string]any{"role": "user", "content": []any{map[string]any{"type": "tool_result", "tool_use_id": "tool-r", "content": "The file says hello."}}},
	}}
	out, err := messages.ToChatRequest(marshalJSON(t, req))
	if err != nil {
		t.Fatal(err)
	}
	transcript := jsonArray(t, jsonObject(t, decodeJSON(t, out))["messages"])
	if len(transcript) != 3 {
		t.Fatalf("spurious/omitted messages: %s", out)
	}
	call := jsonObject(t, jsonArray(t, jsonObject(t, transcript[1])["tool_calls"])[0])
	result := jsonObject(t, transcript[2])
	if call["id"] != "tool-r" || result["tool_call_id"] != call["id"] || result["role"] != "tool" || result["content"] != "The file says hello." {
		t.Fatalf("tool correlation was lost: %s", out)
	}
}

func TestToChatRequestRejectsUnsupportedOrMalformedControls(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, field, raw string }{
		{"missing model", "model", `null`},
		{"empty model", "model", `""`},
		{"zero tokens", "max_tokens", `0`},
		{"negative tokens", "max_tokens", `-1`},
		{"fractional tokens", "max_tokens", `1.5`},
		{"string tokens", "max_tokens", `"32"`},
		{"bad stream", "stream", `"true"`},
		{"null stream", "stream", `null`},
		{"bad temperature", "temperature", `1.1`},
		{"bad top p", "top_p", `false`},
		{"top k", "top_k", `10`},
		{"stop sequence identity unavailable", "stop_sequences", `["stop"]`},
		{"server tool", "tools", `[{"type":"web_search_20250305","name":"web_search","input_schema":{}}]`},
		{"chat tool is not native", "tools", `[{"type":"function","function":{"name":"Read"}}]`},
		{"bad schema", "tools", `[{"name":"Read","input_schema":[]}]`},
		{"missing schema", "tools", `[{"name":"Read"}]`},
		{"unknown tool feature", "tools", `[{"name":"Read","input_schema":{},"defer_loading":true}]`},
		{"duplicate tools", "tools", `[{"name":"Read","input_schema":{}},{"name":"Read","input_schema":{}}]`},
		{"string tool choice", "tool_choice", `"auto"`},
		{"unknown choice", "tool_choice", `{"type":"future"}`},
		{"unknown named tool", "tool_choice", `{"type":"tool","name":"Missing"}`},
		{"choice extra name", "tool_choice", `{"type":"auto","name":"Read"}`},
		{"bad parallel policy", "tool_choice", `{"type":"any","disable_parallel_tool_use":"false"}`},
		{"manual thinking", "thinking", `{"type":"enabled","budget_tokens":4096}`},
		{"adaptive budget", "thinking", `{"type":"adaptive","budget_tokens":4096}`},
		{"signed summary cannot be fabricated", "thinking", `{"type":"adaptive","display":"summarized"}`},
		{"unknown thinking", "thinking", `{"type":"future"}`},
		{"disabled effort conflict", "thinking", `{"type":"disabled"}`},
		{"output effort without thinking", "thinking", `null`},
		{"max is not silently downgraded", "output_config", `{"effort":"max"}`},
		{"unknown effort", "output_config", `{"effort":"future"}`},
		{"output schema", "output_config", `{"effort":"high","format":{"type":"json_schema"}}`},
		{"context future edit", "context_management", `{"edits":[{"type":"future","keep":"all"}]}`},
		{"context drop thinking", "context_management", `{"edits":[{"type":"clear_thinking_20251015","keep":{"type":"thinking_turns","value":1}}]}`},
		{"context drop tools", "context_management", `{"edits":[{"type":"clear_tool_uses_20250919","keep":"all"}]}`},
		{"context unknown control", "context_management", `{"edits":[],"future":true}`},
		{"bad metadata", "metadata", `[]`},
		{"bad cache hint", "cache_control", `"ephemeral"`},
		{"residency", "inference_geo", `"us"`},
		{"MCP server", "mcp_servers", `[]`},
		{"empty messages", "messages", `[]`},
		{"system image", "system", `[{"type":"image","source":{"type":"url","url":"https://example.com/a.png"}}]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := readRequest(t)
			req[tc.field] = json.RawMessage(tc.raw)
			out, err := messages.ToChatRequest(marshalJSON(t, req))
			if err == nil || len(out) != 0 {
				t.Fatalf("unsupported input was forwarded: %s (err %v)", out, err)
			}
		})
	}
	for _, body := range []string{"", "null", "[]", "{", nativeReadRequest + "{}"} {
		if out, err := messages.ToChatRequest([]byte(body)); err == nil || len(out) != 0 {
			t.Errorf("malformed body succeeded: %q", body)
		}
	}
}

func TestToChatRequestRejectsLossyHistory(t *testing.T) {
	t.Parallel()
	validCall := `{"type":"tool_use","id":"a","name":"Read","input":{}}`
	validResult := `{"type":"tool_result","tool_use_id":"a","content":"ok"}`
	for _, tc := range []struct{ name, history string }{
		{"image", `[{"role":"user","content":[{"type":"image","source":{}}]}]`},
		{"thinking history", `[{"role":"assistant","content":[{"type":"thinking","thinking":"private","signature":"signed"}]},{"role":"user","content":"continue"}]`},
		{"redacted thinking", `[{"role":"assistant","content":[{"type":"redacted_thinking","data":"opaque"}]},{"role":"user","content":"continue"}]`},
		{"server tool history", `[{"role":"assistant","content":[{"type":"server_tool_use","id":"a","name":"search","input":{}}]},{"role":"user","content":"continue"}]`},
		{"system role", `[{"role":"system","content":"in-band system"}]`},
		{"assistant prefill", `[{"role":"user","content":"hello"},{"role":"assistant","content":"prefix"}]`},
		{"missing text", `[{"role":"user","content":[{"type":"text"}]}]`},
		{"tool use by user", `[{"role":"user","content":[` + validCall + `]}]`},
		{"unmatched result", `[{"role":"user","content":[` + validResult + `]}]`},
		{"missing result", `[{"role":"assistant","content":[` + validCall + `]},{"role":"user","content":"continue"}]`},
		{"duplicate call", `[{"role":"assistant","content":[` + validCall + `,` + validCall + `]},{"role":"user","content":[` + validResult + `]}]`},
		{"duplicate result", `[{"role":"assistant","content":[` + validCall + `]},{"role":"user","content":[` + validResult + `,` + validResult + `]}]`},
		{"text after call", `[{"role":"assistant","content":[` + validCall + `,{"type":"text","text":"late"}]},{"role":"user","content":[` + validResult + `]}]`},
		{"text before result", `[{"role":"assistant","content":[` + validCall + `]},{"role":"user","content":[{"type":"text","text":"early"},` + validResult + `]}]`},
		{"error flag cannot be erased", `[{"role":"assistant","content":[` + validCall + `]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"a","is_error":true,"content":"failed"}]}]`},
		{"tool result image", `[{"role":"assistant","content":[` + validCall + `]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"a","content":[{"type":"image","source":{}}]}]}]`},
		{"nonobject input", `[{"role":"assistant","content":[{"type":"tool_use","id":"a","name":"Read","input":[]}]},{"role":"user","content":[` + validResult + `]}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := readRequest(t)
			req["messages"] = json.RawMessage(tc.history)
			out, err := messages.ToChatRequest(marshalJSON(t, req), messages.Options{LoadReasoning: func([]string) (string, bool, error) { return "", true, nil }})
			if err == nil || len(out) != 0 {
				t.Fatalf("lossy history succeeded: %s (err %v)", out, err)
			}
			if strings.Contains(err.Error(), "private") || strings.Contains(err.Error(), "opaque") {
				t.Fatalf("request content leaked in error: %v", err)
			}
		})
	}
}
