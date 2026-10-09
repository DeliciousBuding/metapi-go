package messages_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/transform/anthropic/messages"
)

const nativeToolReply = `{"id":"msg-1","type":"message","role":"assistant","model":"claude-test","content":[{"type":"text","text":"Reading."},{"type":"tool_use","id":"tool-1","name":"Read","input":{"offset":9007199254740993,"file":"a.txt"}}],"stop_reason":"tool_use","stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":7,"cache_read_input_tokens":20,"cache_creation_input_tokens":30,"cache_creation":{"ephemeral_5m_input_tokens":25,"ephemeral_1h_input_tokens":5}}}`

func TestFromChatRequestTextImagesAndControls(t *testing.T) {
	t.Parallel()
	out, err := messages.FromChatRequest([]byte(`{
		"model":"claude-test","max_completion_tokens":256,"temperature":0.2,"top_p":0.9,"stop":["END"],"n":1,"user":"client-1",
		"stream":true,"stream_options":{"include_usage":true},"reasoning_effort":"none","cache_control":{"type":"ephemeral"},
		"messages":[
			{"role":"system","content":[{"type":"text","text":"Read carefully.","cache_control":{"type":"ephemeral"}}]},
			{"role":"developer","content":"Answer briefly."},
			{"role":"user","content":[{"type":"text","text":"Compare:"},{"type":"image_url","image_url":{"url":"https://example.com/a.png","detail":"auto"}},{"type":"image_url","image_url":{"url":"data:image/png;base64,AQID"},"cache_control":{"type":"ephemeral"}}]}
		]}`))
	if err != nil {
		t.Fatal(err)
	}
	requireJSON(t, out, `{"model":"claude-test","max_tokens":256,"temperature":0.2,"top_p":0.9,"stop_sequences":["END"],"metadata":{"user_id":"client-1"},"stream":true,"thinking":{"type":"disabled"},"cache_control":{"type":"ephemeral"},"system":[{"type":"text","text":"Read carefully.","cache_control":{"type":"ephemeral"}},{"type":"text","text":"Answer briefly."}],"messages":[{"role":"user","content":[{"type":"text","text":"Compare:"},{"type":"image","source":{"type":"url","url":"https://example.com/a.png"}},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"AQID"},"cache_control":{"type":"ephemeral"}}]}]}`)
	minimal, err := messages.FromChatRequest([]byte(`{"model":"m","messages":[{"role":"user","content":"Hi"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if jsonObject(t, decodeJSON(t, minimal))["max_tokens"] != json.Number("4096") {
		t.Fatal("missing Chat limit did not receive the documented Messages default")
	}
}

func TestMessagesToolReplyCanContinueThroughChat(t *testing.T) {
	t.Parallel()
	reply, err := messages.ToChatResponse([]byte(nativeToolReply))
	if err != nil {
		t.Fatal(err)
	}
	response := jsonObject(t, decodeJSON(t, reply))
	choice := jsonObject(t, jsonArray(t, response["choices"])[0])
	req := map[string]any{
		"model": "claude-test", "tools": []any{map[string]any{"type": "function", "function": map[string]any{"name": "Read", "parameters": map[string]any{"type": "object"}}}},
		"tool_choice": map[string]any{"type": "function", "function": map[string]any{"name": "Read"}}, "parallel_tool_calls": false,
		"messages": []any{
			map[string]any{"role": "user", "content": "Read a.txt"}, choice["message"],
			map[string]any{"role": "tool", "tool_call_id": "tool-1", "content": "Contents"},
			map[string]any{"role": "user", "content": "Continue"},
		},
	}
	out, err := messages.FromChatRequest(marshalJSON(t, req))
	if err != nil {
		t.Fatal(err)
	}
	requireJSON(t, out, `{"model":"claude-test","max_tokens":4096,"tools":[{"name":"Read","input_schema":{"type":"object"}}],"tool_choice":{"type":"tool","name":"Read","disable_parallel_tool_use":true},"messages":[{"role":"user","content":[{"type":"text","text":"Read a.txt"}]},{"role":"assistant","content":[{"type":"text","text":"Reading."},{"type":"tool_use","id":"tool-1","name":"Read","input":{"offset":9007199254740993,"file":"a.txt"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"tool-1","content":[{"type":"text","text":"Contents"}]},{"type":"text","text":"Continue"}]}]}`)
}

func TestFromChatRequestRejectsLossyOrInvalidInputs(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, fields string }{
		{"reasoning", `"reasoning_effort":"high"`},
		{"signed thinking", `"thinking":{"type":"adaptive"}`},
		{"continuity", `"previous_response_id":"r1"`},
		{"multiple candidates", `"n":2`},
		{"unsupported temperature", `"temperature":1.5`},
		{"conflicting limits", `"max_tokens":5,"max_completion_tokens":6`},
		{"zero limit", `"max_tokens":0`},
		{"bad stop", `"stop":[null]`},
		{"empty stop", `"stop":""`},
		{"unknown stream option", `"stream_options":{"include_obfuscation":true}`},
		{"strict tools", `"tools":[{"type":"function","function":{"name":"Read","strict":true}}]`},
		{"hosted tools", `"tools":[{"type":"web_search"}]`},
		{"undeclared tool", `"tool_choice":{"type":"function","function":{"name":"Missing"}}`},
		{"no required tools", `"tool_choice":"required"`},
		{"schema output", `"response_format":{"type":"json_schema"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := messages.FromChatRequest([]byte(`{"model":"m","messages":[{"role":"user","content":"Hi"}],` + tc.fields + `}`))
			if err == nil || len(out) != 0 {
				t.Fatalf("unsupported request released output: %s %v", out, err)
			}
		})
	}
	for _, history := range []string{
		`[{"role":"user","content":"Hi","tool_call_id":"x"}]`,
		`[{"role":"assistant","content":"Hi"}]`,
		`[{"role":"user","content":"Hi"},{"role":"system","content":"Late"}]`,
		`[{"role":"user","content":"Hi"},{"role":"assistant","content":null,"reasoning_content":"secret"},{"role":"user","content":"Go"}]`,
		`[{"role":"tool","tool_call_id":"unknown","content":"output"}]`,
		`[{"role":"user","content":[{"type":"image_url","image_url":{"url":"https://example.com/a.png","detail":"high"}}]}]`,
		`[{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,?bad"}}]}]`,
		`[{"role":"user","content":"Go"},{"role":"assistant","tool_calls":[{"type":"function","id":"a","function":{"name":"Read","arguments":"[]"}}]},{"role":"tool","tool_call_id":"a","content":"Go"}]`,
		`[{"role":"user","content":"Go"},{"role":"assistant","tool_calls":[{"type":"function","id":"a","function":{"name":"Read","arguments":"{}"}}]},{"role":"user","content":"Go"}]`,
	} {
		if out, err := messages.FromChatRequest([]byte(`{"model":"m","messages":` + history + `}`)); err == nil || len(out) != 0 {
			t.Fatalf("lossy history accepted: %s", history)
		}
	}
}

func TestToChatResponsePreservesToolArgumentsAndCacheUsage(t *testing.T) {
	t.Parallel()
	out, err := messages.ToChatResponse([]byte(nativeToolReply))
	if err != nil {
		t.Fatal(err)
	}
	response := jsonObject(t, decodeJSON(t, out))
	if n, err := response["created"].(json.Number).Int64(); err != nil || n <= 0 {
		t.Fatal("missing Chat creation timestamp")
	}
	delete(response, "created")
	requireJSON(t, marshalJSON(t, response), `{"id":"msg-1","model":"claude-test","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"Reading.","tool_calls":[{"type":"function","id":"tool-1","function":{"name":"Read","arguments":"{\"offset\":9007199254740993,\"file\":\"a.txt\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":60,"completion_tokens":7,"total_tokens":67,"prompt_tokens_details":{"cached_tokens":20,"cache_creation_tokens":30,"cache_creation":{"ephemeral_5m_input_tokens":25,"ephemeral_1h_input_tokens":5}}}}`)
}

func TestToChatResponseUsageAndStopReasons(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ usage, want string }{
		{`null`, `null`},
		{`{}`, `{}`},
		{`{"output_tokens":7}`, `{"completion_tokens":7}`},
		{`{"cache_read_input_tokens":4}`, `{"prompt_tokens_details":{"cached_tokens":4}}`},
		{`{"input_tokens":0,"output_tokens":0,"cache_read_input_tokens":4,"cache_creation_input_tokens":2}`, `{"prompt_tokens":6,"completion_tokens":0,"total_tokens":6,"prompt_tokens_details":{"cached_tokens":4,"cache_creation_tokens":2}}`},
	} {
		value := jsonObject(t, decodeJSON(t, []byte(nativeToolReply)))
		value["usage"] = decodeJSON(t, []byte(tc.usage))
		out, err := messages.ToChatResponse(marshalJSON(t, value))
		if err != nil {
			t.Fatal(err)
		}
		requireJSON(t, marshalJSON(t, jsonObject(t, decodeJSON(t, out))["usage"]), tc.want)
	}
	for _, tc := range []struct{ native, chat string }{{"end_turn", "stop"}, {"max_tokens", "length"}, {"refusal", "content_filter"}, {"stop_sequence", "stop"}} {
		value := jsonObject(t, decodeJSON(t, []byte(nativeToolReply)))
		value["content"] = []any{map[string]any{"type": "text", "text": "Done"}}
		value["stop_reason"] = tc.native
		if tc.native == "stop_sequence" {
			value["stop_sequence"] = "END"
		}
		out, err := messages.ToChatResponse(marshalJSON(t, value))
		if err != nil {
			t.Fatal(err)
		}
		choice := jsonObject(t, jsonArray(t, jsonObject(t, decodeJSON(t, out))["choices"])[0])
		if choice["finish_reason"] != tc.chat || tc.native == "stop_sequence" && choice["stop_sequence"] != "END" {
			t.Fatal("stop reason or matched sequence lost")
		}
	}
}

func TestToChatResponseRejectsLossyOrInvalidOutput(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ field, value string }{
		{"content", `[{"type":"thinking","thinking":"secret","signature":"signed"}]`},
		{"content", `[{"type":"redacted_thinking","data":"opaque"}]`},
		{"content", `[{"type":"text","text":"Hi","signature":"signed"}]`},
		{"content", `[{"type":"text","text":"Hi","citations":[{"type":"web_search_result_location"}]}]`},
		{"content", `[{"type":"tool_use","id":"a","name":"Read","input":[]}]`},
		{"content", `[{"type":"tool_use","id":"a","name":"Read","input":{}},{"type":"tool_use","id":"a","name":"Read","input":{}}]`},
		{"content", `[{"type":"tool_use","id":"a","name":"Read","input":{}},{"type":"text","text":"After"}]`},
		{"container", `{"id":"container-1"}`},
		{"context_management", `{"applied_edits":[]}`},
		{"stop_reason", `"pause_turn"`},
		{"stop_reason", `null`},
		{"stop_reason", `"end_turn"`},
		{"usage", `{"input_tokens":9223372036854775807,"cache_read_input_tokens":1}`},
		{"usage", `{"input_tokens":-1}`},
		{"error", `{"message":"SECRET_BODY"}`},
	} {
		value := jsonObject(t, decodeJSON(t, []byte(nativeToolReply)))
		value[tc.field] = decodeJSON(t, []byte(tc.value))
		out, err := messages.ToChatResponse(marshalJSON(t, value))
		if err == nil || len(out) != 0 || strings.Contains(err.Error(), "SECRET_BODY") {
			t.Fatalf("bad %s accepted or leaked: %s %v", tc.field, out, err)
		}
	}
}

func nativeFrame(t *testing.T, event string, value any) []byte {
	t.Helper()
	return []byte("event: " + event + "\ndata: " + string(marshalJSON(t, value)) + "\n\n")
}

func reverseStart(t *testing.T) []byte {
	t.Helper()
	return nativeFrame(t, "message_start", map[string]any{"type": "message_start", "message": map[string]any{"id": "msg-1", "type": "message", "role": "assistant", "model": "claude-test", "content": []any{}, "stop_reason": nil, "usage": map[string]any{"input_tokens": 10, "cache_read_input_tokens": 20, "cache_creation_input_tokens": 30, "output_tokens": 0}}})
}

func reverseBlock(t *testing.T, event string, index int, block any) []byte {
	t.Helper()
	value := map[string]any{"type": event, "index": index}
	if event == "content_block_start" {
		value["content_block"] = block
	} else if event == "content_block_delta" {
		value["delta"] = block
	}
	return nativeFrame(t, event, value)
}

func reverseEnd(t *testing.T, reason string) [][]byte {
	t.Helper()
	return [][]byte{
		nativeFrame(t, "message_delta", map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": reason, "stop_sequence": nil}, "usage": map[string]any{"output_tokens": 7}}),
		nativeFrame(t, "message_stop", map[string]any{"type": "message_stop"}),
	}
}

func consumeChatFrames(t *testing.T, stream *messages.MessagesStream, frames [][]byte) []map[string]any {
	t.Helper()
	var chunks []map[string]any
	done := 0
	for _, frame := range frames {
		out, err := stream.TransformEvent(frame)
		if err != nil {
			t.Fatalf("frame %s: %v", frame, err)
		}
		for _, block := range strings.Split(strings.TrimSpace(string(out)), "\n\n") {
			if block == "" {
				continue
			}
			if !strings.HasPrefix(block, "data: ") || done != 0 {
				t.Fatalf("bad Chat framing/order: %q", block)
			}
			data := strings.TrimPrefix(block, "data: ")
			if data == "[DONE]" {
				done++
				continue
			}
			chunks = append(chunks, jsonObject(t, decodeJSON(t, []byte(data))))
		}
	}
	if _, err := stream.Finish(); err != nil || done != 1 {
		t.Fatalf("missing/invalid terminal: %d %v", done, err)
	}
	return chunks
}

func TestMessagesStreamIncrementalToolJSONAtEveryBoundary(t *testing.T) {
	t.Parallel()
	arguments := `{"offset":9007199254740993,"text":"a\"b\n\u4f60"}`
	for boundary := 0; boundary <= len(arguments); boundary++ {
		t.Run(fmt.Sprint(boundary), func(t *testing.T) {
			frames := [][]byte{
				reverseStart(t),
				reverseBlock(t, "content_block_start", 0, map[string]any{"type": "text", "text": "Read"}),
				reverseBlock(t, "content_block_delta", 0, map[string]any{"type": "text_delta", "text": "ing."}),
				reverseBlock(t, "content_block_stop", 0, nil),
				reverseBlock(t, "content_block_start", 1, map[string]any{"type": "tool_use", "id": "tool-1", "name": "Read", "input": map[string]any{}}),
				reverseBlock(t, "content_block_delta", 1, map[string]any{"type": "input_json_delta", "partial_json": arguments[:boundary]}),
				reverseBlock(t, "content_block_delta", 1, map[string]any{"type": "input_json_delta", "partial_json": arguments[boundary:]}),
				reverseBlock(t, "content_block_stop", 1, nil),
				reverseBlock(t, "content_block_start", 2, map[string]any{"type": "tool_use", "id": "tool-2", "name": "List", "input": map[string]any{}}),
				reverseBlock(t, "content_block_stop", 2, nil),
			}
			frames = append(frames, reverseEnd(t, "tool_use")...)
			chunks := consumeChatFrames(t, messages.NewMessagesStream("fallback"), frames)
			var content strings.Builder
			args, names, ids := make(map[string]string), make(map[string]string), make(map[string]string)
			for i, chunk := range chunks {
				if chunk["id"] != "msg-1" || chunk["model"] != "claude-test" || chunk["object"] != "chat.completion.chunk" {
					t.Fatal("Chat stream identity changed")
				}
				choice := jsonObject(t, jsonArray(t, chunk["choices"])[0])
				if choice["finish_reason"] != nil && i != len(chunks)-1 {
					t.Fatal("finish released before actual native terminal")
				}
				delta := jsonObject(t, choice["delta"])
				if value, ok := delta["content"].(string); ok {
					content.WriteString(value)
				}
				if calls, ok := delta["tool_calls"]; ok {
					for _, raw := range jsonArray(t, calls) {
						call := jsonObject(t, raw)
						index := string(call["index"].(json.Number))
						function := jsonObject(t, call["function"])
						args[index] += function["arguments"].(string)
						if name, ok := function["name"].(string); ok {
							names[index] = name
							ids[index] = call["id"].(string)
						}
					}
				}
			}
			if content.String() != "Reading." || args["0"] != arguments || args["1"] != "{}" || names["0"] != "Read" || names["1"] != "List" || ids["0"] != "tool-1" || ids["1"] != "tool-2" {
				t.Fatalf("lost text or tool identity/arguments: %v %v %v", args, names, ids)
			}
			last := chunks[len(chunks)-1]
			requireJSON(t, marshalJSON(t, last["usage"]), `{"prompt_tokens":60,"completion_tokens":7,"total_tokens":67,"prompt_tokens_details":{"cached_tokens":20,"cache_creation_tokens":30}}`)
			if jsonObject(t, jsonArray(t, last["choices"])[0])["finish_reason"] != "tool_calls" {
				t.Fatal("native tool stop reason was lost")
			}
		})
	}
}

func TestMessagesStreamTruncationAndErrorsNeverSucceed(t *testing.T) {
	t.Parallel()
	textStart := reverseBlock(t, "content_block_start", 0, map[string]any{"type": "text", "text": "Hello"})
	textStop := reverseBlock(t, "content_block_stop", 0, nil)
	valid := append([][]byte{reverseStart(t), textStart, textStop}, reverseEnd(t, "end_turn")...)
	for cutoff := 0; cutoff < len(valid); cutoff++ {
		stream := messages.NewMessagesStream("m")
		for _, frame := range valid[:cutoff] {
			out, err := stream.TransformEvent(frame)
			if err != nil || strings.Contains(string(out), "[DONE]") {
				t.Fatal("prefix unexpectedly failed or terminated")
			}
		}
		if out, err := stream.Finish(); err == nil || len(out) != 0 {
			t.Fatal("truncation became successful")
		}
	}
	toolStart := reverseBlock(t, "content_block_start", 0, map[string]any{"type": "tool_use", "id": "a", "name": "Read", "input": map[string]any{}})
	for name, frames := range map[string][][]byte{
		"no start":                {textStart},
		"double start":            {reverseStart(t), reverseStart(t)},
		"no finish reason":        {reverseStart(t), nativeFrame(t, "message_stop", map[string]any{"type": "message_stop"})},
		"open content":            {reverseStart(t), textStart, reverseEnd(t, "end_turn")[0]},
		"bad index":               {reverseStart(t), reverseBlock(t, "content_block_start", 2, map[string]any{"type": "text", "text": "x"})},
		"overlap":                 {reverseStart(t), textStart, reverseBlock(t, "content_block_start", 1, map[string]any{"type": "text", "text": "x"})},
		"delta without block":     {reverseStart(t), reverseBlock(t, "content_block_delta", 0, map[string]any{"type": "text_delta", "text": "x"})},
		"signed thinking":         {reverseStart(t), reverseBlock(t, "content_block_start", 0, map[string]any{"type": "thinking", "thinking": "secret", "signature": "signed"})},
		"signature delta":         {reverseStart(t), textStart, reverseBlock(t, "content_block_delta", 0, map[string]any{"type": "signature_delta", "signature": "signed"})},
		"incomplete JSON":         {reverseStart(t), toolStart, reverseBlock(t, "content_block_delta", 0, map[string]any{"type": "input_json_delta", "partial_json": "{"}), textStop},
		"nonobject JSON":          {reverseStart(t), toolStart, reverseBlock(t, "content_block_delta", 0, map[string]any{"type": "input_json_delta", "partial_json": "[]"}), textStop},
		"error":                   {reverseStart(t), []byte("event: error\ndata: {\"type\":\"error\",\"error\":{\"message\":\"SECRET_BODY\"}}\n\n")},
		"ping error":              {reverseStart(t), []byte("event: ping\ndata: {\"type\":\"ping\",\"error\":{\"message\":\"SECRET_BODY\"}}\n\n")},
		"wrong event":             {reverseStart(t), []byte("event: ping\ndata: {\"type\":\"message_stop\"}\n\n")},
		"missing frame delimiter": {reverseStart(t), []byte("event: ping\ndata: {\"type\":\"ping\"}\n")},
		"after terminal":          append(append([][]byte{}, valid...), textStart),
	} {
		t.Run(name, func(t *testing.T) {
			stream := messages.NewMessagesStream("m")
			var lastErr error
			for _, frame := range frames {
				out, err := stream.TransformEvent(frame)
				if err != nil {
					lastErr = err
					if len(out) != 0 || strings.Contains(err.Error(), "SECRET_BODY") {
						t.Fatal("error released output or leaked body")
					}
					break
				}
			}
			if lastErr == nil {
				t.Fatal("invalid stream accepted")
			}
			if out, err := stream.TransformEvent(valid[len(valid)-1]); err == nil || len(out) != 0 {
				t.Fatal("later terminal revived errored stream")
			}
			if _, err := stream.Finish(); err == nil {
				t.Fatal("Finish revived errored stream")
			}
		})
	}
}
