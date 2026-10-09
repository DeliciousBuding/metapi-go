package responses

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func bridgeTestObject(t *testing.T, raw []byte) bridgeObject {
	t.Helper()
	obj, err := bridgeDecode(raw)
	if err != nil {
		t.Fatal(err)
	}
	return obj
}
func bridgeTestJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func bridgeTestFrame(t *testing.T, v bridgeObject) []byte {
	t.Helper()
	var b bytes.Buffer
	bridgeEmit(&b, bridgeString(v["type"]), v)
	return b.Bytes()
}
func bridgeTestFrames(t *testing.T, b []byte) []bridgeObject {
	t.Helper()
	var result []bridgeObject
	for _, frame := range bytes.Split(b, []byte("\n\n")) {
		if len(frame) == 0 {
			continue
		}
		_, data, has, err := bridgeFrame(append(frame, '\n', '\n'))
		if err != nil {
			t.Fatal(err)
		}
		if has && data != "[DONE]" {
			result = append(result, bridgeTestObject(t, []byte(data)))
		}
	}
	return result
}

func TestChatBridgeRequestToolRoundTrip(t *testing.T) {
	request := []byte(`{"model":"gpt-test","instructions":"Be brief","input":[{"role":"user","content":[{"type":"input_text","text":"lookup"}]},{"type":"function_call","call_id":"call_1","name":"lookup","arguments":"{\"q\":\"x\"}"},{"type":"function_call","call_id":"call_2","name":"lookup","arguments":"{}"},{"type":"function_call_output","call_id":"call_1","output":"one"},{"type":"function_call_output","call_id":"call_2","output":"two"},{"role":"user","content":"continue"}],"tools":[{"type":"function","name":"lookup","description":"find","parameters":{"type":"object"},"strict":true}],"tool_choice":{"type":"function","name":"lookup"},"max_output_tokens":123,"temperature":0.2,"top_p":0.9,"stream":true}`)
	chat, err := ToChatRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	obj := bridgeTestObject(t, chat)
	messages := obj["messages"].([]any)
	if len(messages) != 6 || len(bridgeMap(messages[2])["tool_calls"].([]any)) != 2 {
		t.Fatalf("parallel call grouping: %s", chat)
	}
	if obj["max_completion_tokens"] != json.Number("123") || bridgeMap(obj["stream_options"])["include_usage"] != true {
		t.Fatalf("options: %s", chat)
	}
	response, err := FromChatRequest(chat)
	if err != nil {
		t.Fatal(err)
	}
	out := bridgeTestObject(t, response)
	items := out["input"].([]any)
	if len(items) != 7 || bridgeMap(items[2])["call_id"] != "call_1" || bridgeMap(items[4])["output"] != "one" {
		t.Fatalf("roundtrip: %s", response)
	}
	if bridgeMap(out["tool_choice"])["name"] != "lookup" || out["max_output_tokens"] != json.Number("123") {
		t.Fatalf("options: %s", response)
	}
}

func TestChatBridgeRejectsUnrepresentableRequests(t *testing.T) {
	for _, extra := range []string{`"previous_response_id":"resp_1"`, `"reasoning":{"effort":"high"}`, `"tools":[{"type":"web_search_preview"}]`, `"store":true`, `"text":{"format":{"type":"json_schema"}}`, `"input":[{"type":"reasoning","encrypted_content":"cipher"}]`, `"input":[{"role":"user","content":[{"type":"input_image","image_url":"x"}]}]`} {
		body := `{"model":"m","input":"hi",` + extra + `}`
		if _, err := ToChatRequest([]byte(body)); err == nil {
			t.Errorf("accepted unsupported request: %s", body)
		}
	}
	for _, body := range []string{`{"model":"m","messages":[{"role":"user","content":"hi"}],"n":2}`, `{"model":"m","messages":[{"role":"assistant","content":"hi","reasoning_content":"secret"}]}`, `{"model":"m","messages":[{"role":"assistant","tool_calls":[{"id":"c","type":"function","function":{"name":"f","arguments":"{"}}]}]}`} {
		if _, err := FromChatRequest([]byte(body)); err == nil {
			t.Errorf("accepted unsupported Chat request: %s", body)
		}
	}
}

func TestChatBridgeJSONResponseRoundTrip(t *testing.T) {
	chat := []byte(`{"id":"chat_1","object":"chat.completion","model":"m","created":123,"choices":[{"index":0,"message":{"role":"assistant","content":"checking","tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":12,"completion_tokens":7,"total_tokens":19,"prompt_tokens_details":{"cached_tokens":3},"completion_tokens_details":{"reasoning_tokens":2}}}`)
	response, err := FromChatResponse(chat)
	if err != nil {
		t.Fatal(err)
	}
	obj := bridgeTestObject(t, response)
	if obj["status"] != "completed" || len(obj["output"].([]any)) != 2 || bridgeMap(obj["usage"])["input_tokens"] != json.Number("12") {
		t.Fatalf("response: %s", response)
	}
	back, err := ToChatResponse(response)
	if err != nil {
		t.Fatal(err)
	}
	got := bridgeTestObject(t, back)
	original := bridgeTestObject(t, chat)
	if !bytes.Equal(bridgeTestJSON(t, got["choices"]), bridgeTestJSON(t, original["choices"])) || !bytes.Equal(bridgeTestJSON(t, got["usage"]), bridgeTestJSON(t, original["usage"])) {
		t.Fatalf("roundtrip: %s", back)
	}
	for _, reason := range []string{"length", "content_filter"} {
		raw := bytes.Replace(chat, []byte(`"finish_reason":"tool_calls"`), []byte(`"finish_reason":"`+reason+`"`), 1)
		response, err := FromChatResponse(raw)
		if err != nil {
			t.Fatal(err)
		}
		if bridgeTestObject(t, response)["status"] != "incomplete" {
			t.Fatal("limit became success")
		}
		back, err := ToChatResponse(response)
		if err != nil {
			t.Fatal(err)
		}
		choice, _ := bridgeChoice(bridgeTestObject(t, back))
		if choice["finish_reason"] != reason {
			t.Fatalf("lost reason: %s", back)
		}
	}
}

func TestChatBridgeJSONRejectsMalformedAndFailed(t *testing.T) {
	for _, body := range []string{`{"error":{"message":"bad"}}`, `{"id":"c","model":"m","object":"chat.completion","choices":[{"message":{"role":"assistant","content":"partial"},"finish_reason":null}]}`, `{"id":"c","model":"m","object":"chat.completion","choices":[{"message":{"role":"assistant","tool_calls":[{"id":"x","type":"function","function":{"name":"f","arguments":"{"}}]},"finish_reason":"tool_calls"}]}`} {
		if _, err := FromChatResponse([]byte(body)); err == nil {
			t.Errorf("accepted bad Chat response: %s", body)
		}
	}
	for _, body := range []string{`{"id":"r","object":"response","model":"m","status":"failed","error":{"message":"bad"},"output":[]}`, `{"id":"r","object":"response","model":"m","status":"completed","output":[{"type":"reasoning","encrypted_content":"cipher"}]}`, `{"id":"r","object":"response","model":"m","status":"completed","output":[{"type":"function_call","call_id":"c","name":"f","arguments":"{"}]}`} {
		if _, err := ToChatResponse([]byte(body)); err == nil {
			t.Errorf("accepted bad Responses result: %s", body)
		}
	}
}

func bridgeChatChunk(delta bridgeObject, reason any) bridgeObject {
	return bridgeObject{"id": "c1", "object": "chat.completion.chunk", "model": "m", "choices": []any{bridgeObject{"index": 0, "delta": delta, "finish_reason": reason}}}
}
func bridgeCall(index int, id, name, args string) bridgeObject {
	return bridgeObject{"index": index, "id": id, "type": "function", "function": bridgeObject{"name": name, "arguments": args}}
}

func TestChatBridgeStreamTextToolsAndUsage(t *testing.T) {
	s := NewChatStream("fallback")
	frames := []bridgeObject{
		bridgeChatChunk(bridgeObject{"role": "assistant", "content": "hi "}, nil),
		bridgeChatChunk(bridgeObject{"content": "there", "tool_calls": []any{bridgeCall(3, "call_1", "lookup", `{"q":`)}}, nil),
		bridgeChatChunk(bridgeObject{"tool_calls": []any{bridgeCall(3, "", "", `"x"}`)}}, "tool_calls"),
		{"id": "c1", "object": "chat.completion.chunk", "model": "m", "choices": []any{}, "usage": bridgeObject{"prompt_tokens": 4, "completion_tokens": 6, "total_tokens": 10}},
	}
	var all bytes.Buffer
	for i, frame := range frames {
		out, err := s.TransformEvent(bridgeTestFrame(t, frame))
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(out, []byte("event: response.completed")) {
			t.Fatal("terminal before [DONE]")
		}
		if i == 0 && !bytes.Contains(out, []byte(`"delta":"hi "`)) {
			t.Fatal("text was buffered until end")
		}
		all.Write(out)
	}
	terminal, err := s.TransformEvent([]byte("data: [DONE]\n\n"))
	if err != nil {
		t.Fatal(err)
	}
	all.Write(terminal)
	if _, err = s.Finish(); err != nil {
		t.Fatal(err)
	}
	events := bridgeTestFrames(t, terminal)
	last := events[len(events)-1]
	response := bridgeMap(last["response"])
	if last["type"] != "response.completed" || bridgeMap(response["usage"])["output_tokens"] != json.Number("6") {
		t.Fatalf("terminal: %s", terminal)
	}
	text := bridgeMap(bridgeMap(response["output"].([]any)[0])["content"].([]any)[0])["text"]
	if text != "hi there" {
		t.Fatalf("SDK final text=%v", text)
	}
	// Every emitted Responses frame must also satisfy the reverse bridge's
	// independent lifecycle validation, preserving content and usage.
	reverse := NewResponsesStream("fallback")
	var chat bytes.Buffer
	for _, frame := range bytes.Split(all.Bytes(), []byte("\n\n")) {
		if len(frame) == 0 {
			continue
		}
		out, err := reverse.TransformEvent(append(frame, '\n', '\n'))
		if err != nil {
			t.Fatalf("reverse: %v; frame=%s", err, frame)
		}
		chat.Write(out)
	}
	if _, err = reverse.Finish(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(chat.Bytes(), []byte(`"finish_reason":"tool_calls"`)) || !bytes.Contains(chat.Bytes(), []byte(`"completion_tokens":6`)) || !bytes.Contains(chat.Bytes(), []byte(`"delta":{"content":"hi "}`)) {
		t.Fatalf("reverse: %s", chat.Bytes())
	}
}

func TestChatBridgeStreamFailureIsSticky(t *testing.T) {
	cases := map[string][]string{
		"missing finish":      {string(bridgeTestFrame(t, bridgeChatChunk(bridgeObject{"content": "hi"}, nil))), "data: [DONE]\n\n"},
		"malformed arguments": {string(bridgeTestFrame(t, bridgeChatChunk(bridgeObject{"tool_calls": []any{bridgeCall(0, "c", "f", "{")}}, "tool_calls"))), "data: [DONE]\n\n"},
		"upstream error":      {string(bridgeTestFrame(t, bridgeChatChunk(bridgeObject{"content": "hi"}, "stop"))), "event: error\ndata: {}\n\n"},
		"multiple frames":     {"data: {}\n\ndata: {}\n\n"},
		"incomplete frame":    {"data: {}"},
		"reasoning":           {string(bridgeTestFrame(t, bridgeChatChunk(bridgeObject{"reasoning_content": "secret"}, nil)))},
		"identity conflict":   {string(bridgeTestFrame(t, bridgeChatChunk(bridgeObject{"tool_calls": []any{bridgeCall(0, "c", "f", "{")}}, nil))), string(bridgeTestFrame(t, bridgeChatChunk(bridgeObject{"tool_calls": []any{bridgeCall(0, "other", "f", "}")}}, nil)))},
	}
	for name, frames := range cases {
		t.Run(name, func(t *testing.T) {
			s := NewChatStream("m")
			var err error
			for _, frame := range frames {
				var out []byte
				out, err = s.TransformEvent([]byte(frame))
				if bytes.Contains(out, []byte("event: response.completed")) {
					t.Fatal("false successful terminal")
				}
				if err != nil {
					break
				}
			}
			if err == nil {
				t.Fatal("expected failure")
			}
			if out, err := s.TransformEvent([]byte("data: [DONE]\n\n")); err == nil || len(out) != 0 {
				t.Fatal("error was not sticky")
			}
			if _, err = s.Finish(); err == nil {
				t.Fatal("Finish repaired failure")
			}
		})
	}
	for _, reason := range []any{nil, "stop"} {
		s := NewChatStream("m")
		if _, err := s.TransformEvent(bridgeTestFrame(t, bridgeChatChunk(bridgeObject{"content": "hi"}, reason))); err != nil {
			t.Fatal(err)
		}
		if out, err := s.Finish(); err == nil || len(out) != 0 {
			t.Fatal("EOF fabricated terminal")
		}
	}
}

func TestChatBridgeStreamAccumulationLimit(t *testing.T) {
	s := NewChatStream("m")
	chunk := strings.Repeat("x", 1<<20)
	for i := 0; i < 8; i++ {
		if _, err := s.TransformEvent(bridgeTestFrame(t, bridgeChatChunk(bridgeObject{"content": chunk}, nil))); err != nil {
			t.Fatal(err)
		}
	}
	if out, err := s.TransformEvent(bridgeTestFrame(t, bridgeChatChunk(bridgeObject{"content": "x"}, nil))); err == nil || len(out) != 0 {
		t.Fatal("unbounded accumulation")
	}
}

func TestChatBridgeStreamIncompleteAndRefusal(t *testing.T) {
	for _, reason := range []string{"length", "content_filter"} {
		t.Run(reason, func(t *testing.T) {
			s := NewChatStream("m")
			if _, err := s.TransformEvent(bridgeTestFrame(t, bridgeChatChunk(bridgeObject{"refusal": "Cannot answer"}, reason))); err != nil {
				t.Fatal(err)
			}
			out, err := s.TransformEvent([]byte("data: [DONE]\n\n"))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(out, []byte("event: response.incomplete")) || bytes.Contains(out, []byte("event: response.completed")) {
				t.Fatalf("lost incomplete state: %s", out)
			}
			events := bridgeTestFrames(t, out)
			response := bridgeMap(events[len(events)-1]["response"])
			chat, err := ToChatResponse(bridgeTestJSON(t, response))
			if err != nil {
				t.Fatal(err)
			}
			choice, err := bridgeChoice(bridgeTestObject(t, chat))
			if err != nil {
				t.Fatal(err)
			}
			if choice["finish_reason"] != reason || bridgeMap(choice["message"])["refusal"] != "Cannot answer" {
				t.Fatalf("refusal: %s", chat)
			}
		})
	}
}

func TestResponsesBridgeRejectsMalformedToolArguments(t *testing.T) {
	s := NewResponsesStream("m")
	frames := []bridgeObject{
		{"type": "response.created", "response": bridgeObject{"id": "r", "model": "m", "status": "in_progress"}},
		{"type": "response.output_item.added", "output_index": 0, "item": bridgeObject{"id": "fc_1", "type": "function_call", "call_id": "call_1", "name": "lookup", "arguments": ""}},
		{"type": "response.function_call_arguments.delta", "output_index": 0, "item_id": "fc_1", "delta": "{"},
	}
	for _, frame := range frames {
		if _, err := s.TransformEvent(bridgeTestFrame(t, frame)); err != nil {
			t.Fatal(err)
		}
	}
	done := bridgeObject{"type": "response.function_call_arguments.done", "output_index": 0, "item_id": "fc_1", "arguments": "{"}
	if out, err := s.TransformEvent(bridgeTestFrame(t, done)); err == nil || len(out) != 0 {
		t.Fatal("malformed arguments accepted")
	}
	if _, err := s.Finish(); err == nil {
		t.Fatal("malformed tool recovered at EOF")
	}
}

func bridgeResponsesFixture(t *testing.T) []bridgeObject {
	t.Helper()
	item := bridgeObject{"id": "msg_1", "type": "message", "role": "assistant", "status": "completed", "content": []any{bridgeObject{"type": "output_text", "text": "Hello", "annotations": []any{}}}}
	return []bridgeObject{
		{"type": "response.created", "response": bridgeObject{"id": "resp_1", "object": "response", "model": "m", "status": "in_progress", "output": []any{}}},
		{"type": "response.output_item.added", "output_index": 0, "item": bridgeObject{"id": "msg_1", "type": "message", "role": "assistant", "status": "in_progress", "content": []any{}}},
		{"type": "response.content_part.added", "output_index": 0, "item_id": "msg_1", "content_index": 0, "part": bridgeObject{"type": "output_text", "text": "", "annotations": []any{}}},
		{"type": "response.output_text.delta", "output_index": 0, "item_id": "msg_1", "content_index": 0, "delta": "Hello"},
		{"type": "response.output_text.done", "output_index": 0, "item_id": "msg_1", "content_index": 0, "text": "Hello"},
		{"type": "response.content_part.done", "output_index": 0, "item_id": "msg_1", "content_index": 0, "part": bridgeObject{"type": "output_text", "text": "Hello", "annotations": []any{}}},
		{"type": "response.output_item.done", "output_index": 0, "item": item},
		{"type": "response.completed", "response": bridgeObject{"id": "resp_1", "object": "response", "model": "m", "status": "completed", "output": []any{item}, "usage": bridgeObject{"input_tokens": 2, "output_tokens": 1, "total_tokens": 3}}},
	}
}

func TestResponsesBridgeIndependentStreamFixture(t *testing.T) {
	s := NewResponsesStream("fallback")
	var out bytes.Buffer
	for _, frame := range bridgeResponsesFixture(t) {
		b, err := s.TransformEvent(bridgeTestFrame(t, frame))
		if err != nil {
			t.Fatal(err)
		}
		out.Write(b)
	}
	if _, err := s.Finish(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out.Bytes(), []byte(`"content":"Hello"`)) || !bytes.Contains(out.Bytes(), []byte(`"finish_reason":"stop"`)) || !bytes.Contains(out.Bytes(), []byte(`"prompt_tokens":2`)) || !bytes.HasSuffix(out.Bytes(), []byte("data: [DONE]\n\n")) {
		t.Fatalf("output: %s", out.Bytes())
	}
}

func TestResponsesBridgeRejectsFalseTerminal(t *testing.T) {
	for _, kind := range []string{"missing item done", "mismatched text", "wrong terminal output", "failed", "EOF", "unsupported reasoning", "missing part done"} {
		t.Run(kind, func(t *testing.T) {
			frames := bridgeResponsesFixture(t)
			switch kind {
			case "missing item done":
				frames = append(frames[:6], frames[7])
			case "missing part done":
				frames = append(frames[:5], frames[6:]...)
			case "mismatched text":
				frames[4]["text"] = "invented"
			case "wrong terminal output":
				response := bridgeMap(frames[7]["response"])
				copy := bridgeTestObject(t, bridgeTestJSON(t, response))
				bridgeMap(bridgeMap(copy["output"].([]any)[0])["content"].([]any)[0])["text"] = "invented"
				frames[7]["response"] = copy
			case "failed":
				frames[7] = bridgeObject{"type": "response.failed", "response": bridgeObject{"status": "failed"}}
			case "EOF":
				frames = frames[:7]
			case "unsupported reasoning":
				frames[2] = bridgeObject{"type": "response.reasoning_summary_text.delta", "delta": "secret"}
			}
			s := NewResponsesStream("m")
			var err error
			for _, frame := range frames {
				var out []byte
				out, err = s.TransformEvent(bridgeTestFrame(t, frame))
				if bytes.Contains(out, []byte("[DONE]")) {
					t.Fatal("false successful terminal")
				}
				if err != nil {
					break
				}
			}
			if kind == "EOF" {
				_, err = s.Finish()
			}
			if err == nil {
				t.Fatal("expected failure")
			}
			if _, err = s.Finish(); err == nil {
				t.Fatal("failure not sticky")
			}
		})
	}
}
