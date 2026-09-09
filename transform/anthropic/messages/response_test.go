package messages_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/transform/anthropic/messages"
)

const chatToolReply = `{"id":"chatcmpl-1","object":"chat.completion","model":"m","created":123,"choices":[{"index":0,"message":{"role":"assistant","content":"Reading now.","tool_calls":[{"id":"call-1","type":"function","function":{"name":"Read","arguments":"{\"file_path\":\"sample.txt\",\"offset\":9007199254740993}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":100,"completion_tokens":20,"total_tokens":120,"prompt_tokens_details":{"cached_tokens":30}}}`

func TestFromChatResponseTextAndTool(t *testing.T) {
	t.Parallel()
	out, err := messages.FromChatResponse([]byte(chatToolReply))
	if err != nil {
		t.Fatal(err)
	}
	requireJSON(t, out, `{"id":"chatcmpl-1","type":"message","role":"assistant","model":"m","content":[{"type":"text","text":"Reading now."},{"type":"tool_use","id":"call-1","name":"Read","input":{"file_path":"sample.txt","offset":9007199254740993}}],"stop_reason":"tool_use","stop_sequence":null,"usage":{"input_tokens":70,"output_tokens":20,"cache_read_input_tokens":30}}`)
}

func TestFromChatResponseFinishReasons(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		reason, want string
		tools        bool
	}{
		{"stop", "end_turn", false},
		{"length", "max_tokens", false},
		{"tool_calls", "tool_use", true},
		{"stop", "tool_use", true},     // Known complete tools + stale ordinary stop.
		{"length", "max_tokens", true}, // Never repair a length outcome to tool_use.
	} {
		t.Run(tc.reason+tc.want, func(t *testing.T) {
			reply := jsonObject(t, decodeJSON(t, []byte(chatToolReply)))
			choice := jsonObject(t, jsonArray(t, reply["choices"])[0])
			choice["finish_reason"] = tc.reason
			if !tc.tools {
				delete(jsonObject(t, choice["message"]), "tool_calls")
			}
			out, err := messages.FromChatResponse(marshalJSON(t, reply))
			if err != nil {
				t.Fatal(err)
			}
			got := jsonObject(t, decodeJSON(t, out))
			if got["stop_reason"] != tc.want {
				t.Fatalf("stop_reason=%v; want %s", got["stop_reason"], tc.want)
			}
		})
	}
}

func TestFromChatResponseUnknownUsageIsNotZero(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ usage, want string }{
		{`null`, `{}`},
		{`{}`, `{}`},
		{`{"total_tokens":50}`, `{}`},
		{`{"prompt_tokens":0,"completion_tokens":0}`, `{"input_tokens":0,"output_tokens":0}`},
		{`{"completion_tokens":7}`, `{"output_tokens":7}`},
		{`{"prompt_tokens":20}`, `{"input_tokens":20}`},
		{`{"prompt_tokens_details":{"cached_tokens":4}}`, `{"cache_read_input_tokens":4}`},
		{`{"prompt_tokens":20,"prompt_tokens_details":{"cached_tokens":0}}`, `{"input_tokens":20,"cache_read_input_tokens":0}`},
	} {
		t.Run(tc.usage, func(t *testing.T) {
			reply := jsonObject(t, decodeJSON(t, []byte(chatToolReply)))
			reply["usage"] = json.RawMessage(tc.usage)
			out, err := messages.FromChatResponse(marshalJSON(t, reply))
			if err != nil {
				t.Fatal(err)
			}
			requireJSON(t, marshalJSON(t, jsonObject(t, decodeJSON(t, out))["usage"]), tc.want)
		})
	}
}

func TestFromChatResponseRejectsInvalidToolArguments(t *testing.T) {
	t.Parallel()
	for _, arguments := range []string{"", "{", "null", "[]", "7", `"text"`, `{"x":1}{}`, `{"x":}`} {
		t.Run(arguments, func(t *testing.T) {
			reply := jsonObject(t, decodeJSON(t, []byte(chatToolReply)))
			choice := jsonObject(t, jsonArray(t, reply["choices"])[0])
			message := jsonObject(t, choice["message"])
			call := jsonObject(t, jsonArray(t, message["tool_calls"])[0])
			jsonObject(t, call["function"])["arguments"] = arguments
			out, err := messages.FromChatResponse(marshalJSON(t, reply))
			if err == nil || len(out) != 0 {
				t.Fatalf("bad arguments were repaired into success: %s (err %v)", out, err)
			}
		})
	}
}

func TestFromChatResponseRejectsInvalidOrUnsupportedOutput(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		mutate func(map[string]any, map[string]any, map[string]any)
	}{
		{"error envelope", func(r, c, m map[string]any) { r["error"] = map[string]any{"message": "SHOULD_NOT_LEAK"} }},
		{"wrong object", func(r, c, m map[string]any) { r["object"] = "response" }},
		{"no ID", func(r, c, m map[string]any) { delete(r, "id") }},
		{"no model", func(r, c, m map[string]any) { r["model"] = "" }},
		{"multiple choices", func(r, c, m map[string]any) { r["choices"] = []any{c, c} }},
		{"wrong choice index", func(r, c, m map[string]any) { c["index"] = 1 }},
		{"no finish", func(r, c, m map[string]any) { delete(c, "finish_reason") }},
		{"filter", func(r, c, m map[string]any) { c["finish_reason"] = "content_filter" }},
		{"legacy function finish", func(r, c, m map[string]any) { c["finish_reason"] = "function_call" }},
		{"tool finish without tools", func(r, c, m map[string]any) { delete(m, "tool_calls") }},
		{"nonassistant", func(r, c, m map[string]any) { m["role"] = "user" }},
		{"refusal", func(r, c, m map[string]any) { m["refusal"] = "refused" }},
		{"audio", func(r, c, m map[string]any) { m["audio"] = map[string]any{"data": "..."} }},
		{"citations", func(r, c, m map[string]any) { m["annotations"] = []any{map[string]any{"type": "url_citation"}} }},
		{"structured content", func(r, c, m map[string]any) { m["content"] = []any{map[string]any{"type": "image"}} }},
		{"reasoning not answer", func(r, c, m map[string]any) {
			delete(m, "content")
			m["reasoning_content"] = "private"
			delete(m, "tool_calls")
			c["finish_reason"] = "stop"
		}},
		{"nonstring reasoning", func(r, c, m map[string]any) { m["reasoning_content"] = []any{} }},
		{"duplicate tools", func(r, c, m map[string]any) {
			call := jsonArray(t, m["tool_calls"])[0]
			m["tool_calls"] = []any{call, call}
		}},
		{"empty tool ID", func(r, c, m map[string]any) { jsonObject(t, jsonArray(t, m["tool_calls"])[0])["id"] = "" }},
		{"nonfunction tool", func(r, c, m map[string]any) { jsonObject(t, jsonArray(t, m["tool_calls"])[0])["type"] = "custom" }},
		{"empty tool name", func(r, c, m map[string]any) {
			call := jsonObject(t, jsonArray(t, m["tool_calls"])[0])
			jsonObject(t, call["function"])["name"] = ""
		}},
		{"negative usage", func(r, c, m map[string]any) { r["usage"] = map[string]any{"completion_tokens": -1} }},
		{"string usage", func(r, c, m map[string]any) { r["usage"] = map[string]any{"prompt_tokens": "12"} }},
		{"inconsistent cache", func(r, c, m map[string]any) {
			r["usage"] = map[string]any{"prompt_tokens": 1, "prompt_tokens_details": map[string]any{"cached_tokens": 2}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reply := jsonObject(t, decodeJSON(t, []byte(chatToolReply)))
			choice := jsonObject(t, jsonArray(t, reply["choices"])[0])
			message := jsonObject(t, choice["message"])
			tc.mutate(reply, choice, message)
			out, err := messages.FromChatResponse(marshalJSON(t, reply))
			if err == nil || len(out) != 0 {
				t.Fatalf("bad response succeeded: %s (err %v)", out, err)
			}
			if strings.Contains(err.Error(), "SHOULD_NOT_LEAK") {
				t.Fatal("upstream error body leaked")
			}
		})
	}
}

func TestFromChatResponseEmptyContentDoesNotAddTextToTools(t *testing.T) {
	t.Parallel()
	reply := jsonObject(t, decodeJSON(t, []byte(chatToolReply)))
	choice := jsonObject(t, jsonArray(t, reply["choices"])[0])
	jsonObject(t, choice["message"])["content"] = ""
	out, err := messages.FromChatResponse(marshalJSON(t, reply))
	if err != nil {
		t.Fatal(err)
	}
	content := jsonArray(t, jsonObject(t, decodeJSON(t, out))["content"])
	if len(content) != 1 || jsonObject(t, content[0])["type"] != "tool_use" {
		t.Fatalf("empty Chat content produced a spurious native text block: %s", out)
	}
}
