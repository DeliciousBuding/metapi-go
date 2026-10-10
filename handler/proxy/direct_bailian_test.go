package proxyhandler

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestDirectBailianRequestReasoningAndToolHistory(t *testing.T) {
	body := []byte(`{
		"model":"qwen-test", "reasoning_effort":"none", "enable_thinking":true,
		"seed":9007199254740993, "metadata":{"number":1.2300000000000000001e+20},
		"messages":[
			{"role":"user","content":"查一下"},
			{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"namespace__lookup","arguments":"{\"id\":9007199254740993}"},"vendor":{"n":9007199254740995}}]},
			{"role":"Assistant","content":null,"tool_calls":[{"id":"call_2","type":"function","function":{"name":"lookup_2","arguments":"{}"}}]},
			{"role":"assistant","content":[],"tool_calls":[{"id":"call_3","type":"function","function":{"name":"lookup_3","arguments":"{}"}}]},
			{"role":"tool","tool_call_id":"call_1","content":"one"},
			{"role":"tool","tool_call_id":"call_2","content":"two"},
			{"role":"tool","tool_call_id":"call_3","content":"three"}
		]
	}`)
	original := bytes.Clone(body)
	got, err := prepareDirectBailianRequest(body)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(body, original) {
		t.Fatal("request normalization mutated its input")
	}
	doc, _ := directBailianObject(got)
	if string(doc["enable_thinking"]) != "false" || doc["reasoning_effort"] != nil {
		t.Fatalf("thinking was not disabled: %s", got)
	}
	for _, exact := range []string{"9007199254740993", "9007199254740995", "1.2300000000000000001e+20", "namespace__lookup"} {
		if !bytes.Contains(got, []byte(exact)) {
			t.Fatalf("lost original numeric lexeme or name %s: %s", exact, got)
		}
	}
	var messages []map[string]json.RawMessage
	if err := json.Unmarshal(doc["messages"], &messages); err != nil || len(messages) != 5 {
		t.Fatalf("pure consecutive calls not merged: %s (%v)", got, err)
	}
	var calls []map[string]json.RawMessage
	_ = json.Unmarshal(messages[1]["tool_calls"], &calls)
	if len(calls) != 3 || directBailianString(calls[0]["id"]) != "call_1" || directBailianString(calls[2]["id"]) != "call_3" {
		t.Fatalf("tool call order or identity changed: %s", messages[1]["tool_calls"])
	}
	if directBailianString(messages[2]["tool_call_id"]) != "call_1" || directBailianString(messages[4]["tool_call_id"]) != "call_3" {
		t.Fatal("tool result messages changed")
	}
}

func TestDirectBailianRequestKeepsMixedMessageBoundaries(t *testing.T) {
	for _, field := range []string{
		`"content":"text"`, `"content":" "`, `"content":[{"type":"text","text":""}]`,
		`"content":[{"type":"thinking","thinking":"reason"}]`, `"reasoning_content":"reason"`,
		`"reasoning_content":""`, `"reasoning_signature":null`, `"name":"speaker"`, `"tool_call_id":"tool"`,
		`"refusal":""`, `"message_index":0`, `"cache_control":{"type":"ephemeral"}`,
		`"vendor":{"n":9007199254740993}`, `"audio":{"id":"audio-1"}`,
	} {
		t.Run(field, func(t *testing.T) {
			message := `{"role":"assistant","tool_calls":[{"id":"first","function":{"name":"search","arguments":"{}"}}]}`
			mixed := strings.TrimSuffix(message, "}") + "," + field + "}"
			body := []byte(`{"model":"test", "messages":[` + message + "," + mixed + "," + message + `]}`)
			got, err := prepareDirectBailianRequest(body)
			if err != nil || !bytes.Equal(got, body) {
				t.Fatalf("mixed message changed or merged: %s (%v)", got, err)
			}
		})
	}
}

func TestDirectBailianRequestPreservesExplicitNativeOptions(t *testing.T) {
	for _, body := range []string{
		`{"model":"test","messages":[],"enable_thinking":false}`,
		`{"model":"test","messages":[],"reasoning_effort":"high","enable_thinking":true}`,
		`{"model":"test","messages":[{"role":"assistant","tool_calls":[]},{"role":"assistant","tool_calls":[]}]}`,
	} {
		got, err := prepareDirectBailianRequest([]byte(body))
		if err != nil || string(got) != body {
			t.Fatalf("non-none effort or native option changed: %s (%v)", got, err)
		}
	}
	for _, body := range []string{"null", "[]", "{", `{"messages":null}`, `{"messages":{}}`, "{\"x\":\"\xff\"}"} {
		if _, err := prepareDirectBailianRequest([]byte(body)); err == nil {
			t.Fatalf("invalid request accepted: %q", body)
		}
	}
}

func bailianTestFrame(raw string) []byte { return []byte("data: " + raw + "\n\n") }

func bailianTestChunk(choices string) []byte {
	return bailianTestFrame(`{"id":"bailian-id","object":"chat.completion.chunk","created":9007199254740993,"model":"qwen-test","choices":` + choices + `}`)
}

func bailianTestTool(index int, arguments, id string) []byte {
	call := map[string]any{"index": 0, "type": "function", "function": map[string]any{"name": "namespace__lookup", "arguments": arguments}}
	if id != "" {
		call["id"] = id
	}
	choices := []any{map[string]any{"index": index, "delta": map[string]any{"tool_calls": []any{call}}}}
	return bailianTestChunk(string(directBailianJSON(choices)))
}

func bailianTestTransform(t *testing.T, stream *directBailianStream, frames ...[]byte) []byte {
	t.Helper()
	var result []byte
	for _, frame := range frames {
		out, err := stream.TransformEvent(frame)
		if err != nil {
			t.Fatal(err)
		}
		result = append(result, out...)
	}
	return result
}

func TestDirectBailianStreamTextPassesThroughExactly(t *testing.T) {
	stream := newDirectBailianStream()
	frames := [][]byte{
		[]byte(": heartbeat\n\n"),
		[]byte("id: first\r\nevent: message\r\nretry: 1000\r\ndata: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hello\",\"reasoning_content\":\"thought\",\"x\":9007199254740993}}]}\r\n\r\n"),
		bailianTestChunk(`[{"index":0,"delta":{"content":" world"},"finish_reason":"length"}]`),
		bailianTestFrame(`{"choices":[],"usage":{"completion_tokens":23,"prompt_tokens":11,"completion_tokens_details":{"reasoning_tokens":7}}}`),
		bailianTestFrame(`[DONE]`),
	}
	for _, frame := range frames {
		got := bailianTestTransform(t, stream, frame)
		if !bytes.Equal(got, frame) {
			t.Fatalf("text-only stream changed: %q", got)
		}
	}
	if _, err := stream.Finish(); err != nil {
		t.Fatal(err)
	}
}

func TestDirectBailianStreamFiltersOnlyRedundantCompleteObject(t *testing.T) {
	stream := newDirectBailianStream()
	bailianTestTransform(t, stream, bailianTestTool(0, `{"filter":`, "call-0"))
	nested := bailianTestTool(0, "{}", "")
	if got := bailianTestTransform(t, stream, nested); !bytes.Equal(got, nested) {
		t.Fatalf("legitimate nested empty object was stripped: %s", got)
	}
	bailianTestTransform(t, stream, bailianTestTool(0, "}", ""))
	redundant := bailianTestChunk(`[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call-0","type":"function","vendor":{"n":9007199254740993},"function":{"name":"namespace__lookup","arguments":" {} ","metadata":{"n":9007199254740995}}}]}}]`)
	got := bailianTestTransform(t, stream, redundant)
	for _, expected := range []string{`"arguments":""`, `"name":"namespace__lookup"`, `"id":"call-0"`, `9007199254740993`, `9007199254740995`} {
		if !bytes.Contains(got, []byte(expected)) {
			t.Fatalf("redundant arguments or metadata incorrect, missing %s: %s", expected, got)
		}
	}
	initialEmpty := bailianTestTool(1, "{}", "call-1")
	if got := bailianTestTransform(t, stream, initialEmpty); !bytes.Equal(got, initialEmpty) {
		t.Fatal("initial empty object from another choice was stripped")
	}
	finish := bailianTestChunk(`[{"index":0,"delta":{},"finish_reason":"tool_calls"},{"index":1,"delta":{},"finish_reason":"tool_calls"}]`)
	bailianTestTransform(t, stream, finish, bailianTestFrame(`[DONE]`))
	if _, err := stream.Finish(); err != nil || stream.stateBytes != 0 {
		t.Fatalf("terminal or state release failed: bytes=%d, %v", stream.stateBytes, err)
	}
}

func TestDirectBailianStreamBuffersPerChoiceWithoutLosingMixedFields(t *testing.T) {
	stream := newDirectBailianStream()
	bailianTestTransform(t, stream, bailianTestTool(0, `{"filter":`, "call-0"))
	mixed := bailianTestChunk(`[{"index":0,"delta":{"content":[{"type":"text","text":"choice zero","annotations":[{"n":9007199254740993}]},{"type":"image_url","image_url":{"url":"https://example.invalid/image"}}],"reasoning_content":"keep thinking","reasoning_signature":"keep signature","vendor_delta":{"n":9007199254740995}},"logprobs":{"content":[{"token":"choice zero","logprob":-0.1234567890123456789}]},"vendor_choice":"keep choice"},{"index":1,"delta":{"content":"choice one immediate"}}]`)
	got := bailianTestTransform(t, stream, mixed)
	if bytes.Contains(got, []byte("choice zero")) || !bytes.Contains(got, []byte("choice one immediate")) {
		t.Fatalf("text was not isolated by choice: %s", got)
	}
	for _, expected := range []string{"keep thinking", "keep signature", "9007199254740995", "keep choice"} {
		if !bytes.Contains(got, []byte(expected)) {
			t.Fatalf("mixed delta field lost: %s", got)
		}
	}
	bailianTestTransform(t, stream, bailianTestTool(1, "{}", "call-1"))
	got = bailianTestTransform(t, stream, bailianTestChunk(`[{"index":1,"delta":{"content":"choice one buffered"}}]`))
	if bytes.Contains(got, []byte("choice one buffered")) {
		t.Fatalf("choice one text was not delayed: %s", got)
	}
	finish := bailianTestFrame(`{"id":"bailian-id","object":"chat.completion.chunk","created":9007199254740993,"model":"qwen-test","vendor":{"n":9007199254740997},"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{}}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":11,"completion_tokens":23}}`)
	got = bailianTestTransform(t, stream, finish)
	lastTool := bytes.Index(got, []byte(`"arguments":"{}}"`))
	text := bytes.Index(got, []byte("choice zero"))
	terminal := bytes.Index(got, []byte(`"finish_reason":"tool_calls"`))
	if lastTool < 0 || text <= lastTool || terminal <= text || bytes.Contains(got, []byte("choice one buffered")) {
		t.Fatalf("finish did not follow last tool and its own text: %s", got)
	}
	for _, expected := range []string{"9007199254740993", "https://example.invalid/image", "-0.1234567890123456789", "9007199254740997"} {
		if !bytes.Contains(got, []byte(expected)) {
			t.Fatalf("deferred content, identity or extension lost, missing %s: %s", expected, got)
		}
	}
	if bytes.Count(got, []byte(`"usage"`)) != 1 || bytes.Count(got, []byte(`"vendor"`)) != 1 {
		t.Fatalf("usage or original metadata duplicated: %s", got)
	}
	got = bailianTestTransform(t, stream, bailianTestChunk(`[{"index":1,"delta":{},"finish_reason":"length"}]`), bailianTestFrame(`[DONE]`))
	if !bytes.Contains(got, []byte("choice one buffered")) || !bytes.Contains(got, []byte(`"finish_reason":"length"`)) || bytes.Contains(got, []byte("choice zero")) {
		t.Fatalf("choice one completion was mixed or rewritten: %s", got)
	}
	if _, err := stream.Finish(); err != nil || stream.stateBytes != 0 {
		t.Fatalf("terminal or state release failed: bytes=%d, %v", stream.stateBytes, err)
	}
}

func TestDirectBailianStreamRejectsTruncationAndInvalidLifecycle(t *testing.T) {
	for name, frames := range map[string][][]byte{
		"DONE without choice": {bailianTestFrame(`[DONE]`)},
		"DONE before finish":  {bailianTestTool(0, "{}", "call"), bailianTestFrame(`[DONE]`)},
		"tool without index":  {bailianTestChunk(`[{"index":0,"delta":{"tool_calls":[{"function":{"arguments":"{}"}}]}}]`)},
		"changed tool ID":     {bailianTestTool(0, "{}", "first"), bailianTestTool(0, "{}", "second")},
		"negative choice":     {bailianTestChunk(`[{"index":-1,"delta":{"content":"bad"}}]`)},
		"fractional choice":   {bailianTestChunk(`[{"index":0.5,"delta":{"content":"bad"}}]`)},
		"duplicate choice":    {bailianTestChunk(`[{"index":0,"delta":{}},{"index":0,"delta":{}}]`)},
		"truncated frame":     {[]byte(`data: {"choices":[]}`)},
		"two frames":          {append(bailianTestChunk(`[]`), bailianTestChunk(`[]`)...)},
		"bad JSON":            {bailianTestFrame(`not JSON`)},
		"empty finish":        {bailianTestChunk(`[{"index":0,"delta":{},"finish_reason":""}]`)},
		"data after finish":   {bailianTestChunk(`[{"index":0,"delta":{"content":"done"},"finish_reason":"stop"}]`), bailianTestTool(0, "{}", "call")},
		"data after DONE":     {bailianTestChunk(`[{"index":0,"delta":{"content":"done"},"finish_reason":"stop"}]`), bailianTestFrame(`[DONE]`), bailianTestChunk(`[]`)},
	} {
		t.Run(name, func(t *testing.T) {
			stream := newDirectBailianStream()
			var failure error
			for _, frame := range frames {
				if _, err := stream.TransformEvent(frame); err != nil {
					failure = err
					break
				}
			}
			if failure == nil {
				t.Fatal("invalid lifecycle accepted")
			}
			if _, err := stream.Finish(); err == nil {
				t.Fatal("failure was not sticky")
			}
		})
	}
	stream := newDirectBailianStream()
	bailianTestTransform(t, stream, bailianTestTool(0, "{}", "call"), bailianTestChunk(`[{"index":0,"delta":{"content":"do not fabricate completion"}}]`))
	if out, err := stream.Finish(); err == nil || len(out) != 0 {
		t.Fatalf("truncation flushed content or fabricated terminal: %s, %v", out, err)
	}
}

func TestDirectBailianStreamKeepsProviderErrors(t *testing.T) {
	for _, failure := range [][]byte{
		[]byte("event: error\nid: provider-error\ndata: {\"code\":\"overloaded\",\"request_id\":\"fixture-req\"}\n\n"),
		bailianTestFrame(`{"error":{"message":"overloaded","code":"busy","n":9007199254740993},"usage":{"completion_tokens":7}}`),
	} {
		stream := newDirectBailianStream()
		bailianTestTransform(t, stream, bailianTestTool(0, "{}", "call"), bailianTestChunk(`[{"index":0,"delta":{"content":"pending"}}]`))
		if got := bailianTestTransform(t, stream, failure); !bytes.Equal(got, failure) {
			t.Fatalf("provider error or partial usage changed: %q", got)
		}
		if out, err := stream.TransformEvent(bailianTestFrame(`[DONE]`)); err == nil || len(out) != 0 {
			t.Fatalf("error was hidden by terminal: %s, %v", out, err)
		}
		if out, err := stream.Finish(); err == nil || len(out) != 0 {
			t.Fatal("provider failure was treated as a successful finish")
		}
	}
}

func TestDirectBailianStreamBoundsRetainedStateAndFrame(t *testing.T) {
	for name, setup := range map[string]func(*directBailianStream) []byte{
		"arguments": func(s *directBailianStream) []byte {
			s.stateLimit = 8
			return bailianTestTool(0, strings.Repeat("x", 9), "")
		},
		"text": func(s *directBailianStream) []byte {
			bailianTestTransform(t, s, bailianTestTool(0, "{}", "call"))
			s.stateLimit = 200
			return bailianTestChunk(`[{"index":0,"delta":{"content":"` + strings.Repeat("x", 201) + `"}}]`)
		},
		"frame": func(_ *directBailianStream) []byte {
			return bailianTestFrame(strings.Repeat("x", maxIncrementalSsePendingBytes+1))
		},
		"choice count": func(s *directBailianStream) []byte {
			for i := range directBailianObjectLimit {
				bailianTestTransform(t, s, bailianTestTool(i, "{}", ""))
			}
			return bailianTestTool(directBailianObjectLimit, "{}", "")
		},
		"tool count": func(_ *directBailianStream) []byte {
			calls := make([]any, directBailianObjectLimit+1)
			for i := range calls {
				calls[i] = map[string]any{"index": i, "function": map[string]any{"arguments": "{}"}}
			}
			return bailianTestChunk(string(directBailianJSON([]any{map[string]any{"index": 0, "delta": map[string]any{"tool_calls": calls}}})))
		},
	} {
		t.Run(name, func(t *testing.T) {
			stream := newDirectBailianStream()
			if out, err := stream.TransformEvent(setup(stream)); err == nil || len(out) != 0 {
				t.Fatalf("limit was not enforced: output=%d, %v", len(out), err)
			}
		})
	}
}
