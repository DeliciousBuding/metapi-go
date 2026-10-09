package responses

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

const bridgeReasoningChat = `{"id":"chat_1","object":"chat.completion","model":"moonshot-test","created":123,"choices":[{"index":0,"message":{"role":"assistant","reasoning_content":"Check the supplied fixture.\nThen read the file.","content":"Reading now.","tool_calls":[{"id":"call_1","type":"function","function":{"name":"read","arguments":"{\"offset\":9007199254740993}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":12,"completion_tokens":7,"total_tokens":19,"prompt_tokens_details":{"cached_tokens":3},"completion_tokens_details":{"reasoning_tokens":4}}}`

func TestChatBridgeReasoningPreferences(t *testing.T) {
	for _, effort := range []string{"none", "minimal", "low", "medium", "high", "xhigh"} {
		for _, summary := range []string{"auto", "concise", "detailed"} {
			input := []byte(fmt.Sprintf(`{"model":"alias","input":"hi","reasoning":{"effort":%q,"summary":%q,"generate_summary":"concise","max_tokens":256}}`, effort, summary))
			chat, err := ToChatRequest(input)
			if err != nil {
				t.Fatal(err)
			}
			request := bridgeTestObject(t, chat)
			if request["reasoning_effort"] != effort || request["reasoning_summary"] != summary || request["reasoning_budget"] != json.Number("256") {
				t.Fatalf("reasoning controls lost: %s", chat)
			}
			back, err := FromChatRequest(chat)
			if err != nil {
				t.Fatal(err)
			}
			reasoning := bridgeMap(bridgeTestObject(t, back)["reasoning"])
			if reasoning["effort"] != effort || reasoning["summary"] != summary || reasoning["max_tokens"] != json.Number("256") || reasoning["generate_summary"] != nil {
				t.Fatalf("reasoning controls changed: %s", back)
			}
		}
	}
	for _, raw := range []string{`{"generate_summary":"detailed"}`, `{"summary":"","generate_summary":"detailed"}`} {
		out, err := ToChatRequest([]byte(`{"model":"alias","input":"hi","reasoning":` + raw + `}`))
		if err != nil || bridgeTestObject(t, out)["reasoning_summary"] != "detailed" {
			t.Fatalf("deprecated summary preference was dropped: %s %v", out, err)
		}
	}
	for _, raw := range []string{`null`, `{}`, `{"effort":"","summary":""}`} {
		out, err := ToChatRequest([]byte(`{"model":"alias","input":"hi","reasoning":` + raw + `}`))
		if err != nil {
			t.Fatal(err)
		}
		request := bridgeTestObject(t, out)
		if request["reasoning_effort"] != nil || request["reasoning_summary"] != nil {
			t.Fatal("invented reasoning preference")
		}
	}
	for _, raw := range []string{`[]`, `{"effort":"maximum"}`, `{"effort":true}`, `{"summary":"short"}`, `{"generate_summary":false}`, `{"max_tokens":0}`, `{"max_tokens":1.5}`, `{"context":"all"}`} {
		if out, err := ToChatRequest([]byte(`{"model":"alias","input":"hi","reasoning":` + raw + `}`)); err == nil || len(out) != 0 {
			t.Fatalf("invalid/unsupported reasoning accepted: %s", raw)
		}
	}
}

func TestChatBridgePlainReasoningJSONAndToolReplay(t *testing.T) {
	response, err := FromChatResponse([]byte(bridgeReasoningChat))
	if err != nil {
		t.Fatal(err)
	}
	value := bridgeTestObject(t, response)
	output := value["output"].([]any)
	if len(output) != 3 {
		t.Fatalf("reasoning, message and tool expected: %s", response)
	}
	item := bridgeMap(output[0])
	if item["id"] != "rs_chat_1" || item["type"] != "reasoning" || item["status"] != "completed" || item["encrypted_content"] != nil {
		t.Fatalf("wrong reasoning item: %s", response)
	}
	parts := item["summary"].([]any)
	expectedText := "Check the supplied fixture.\nThen read the file."
	if len(parts) != 1 || bridgeMap(parts[0])["type"] != "summary_text" || bridgeMap(parts[0])["text"] != expectedText {
		t.Fatal("upstream reasoning was changed or locally summarized")
	}
	back, err := ToChatResponse(response)
	if err != nil {
		t.Fatal(err)
	}
	original, got := bridgeTestObject(t, []byte(bridgeReasoningChat)), bridgeTestObject(t, back)
	for _, field := range []string{"choices", "usage"} {
		if !bytes.Equal(bridgeTestJSON(t, original[field]), bridgeTestJSON(t, got[field])) {
			t.Fatalf("reasoning response round trip lost %s: %s", field, back)
		}
	}
	for _, sanitize := range []bool{false, true} {
		history := append([]any{bridgeObject{"role": "user", "content": "Read the fixture"}}, output...)
		history = append(history, bridgeObject{"type": "function_call_output", "call_id": "call_1", "output": "fixture contents"})
		var input any = history
		if sanitize {
			input, err = SanitizeResponsesInputItems(history)
			if err != nil {
				t.Fatal(err)
			}
		}
		chat, err := ToChatRequest(bridgeTestJSON(t, bridgeObject{"model": "alias", "reasoning": bridgeObject{"effort": "high"}, "input": input}))
		if err != nil {
			t.Fatal(err)
		}
		messages := bridgeTestObject(t, chat)["messages"].([]any)
		if len(messages) != 3 || bridgeMap(messages[1])["reasoning_content"] != expectedText || bridgeMap(messages[1])["content"] != "Reading now." || len(bridgeMap(messages[1])["tool_calls"].([]any)) != 1 || bridgeMap(messages[2])["tool_call_id"] != "call_1" {
			t.Fatalf("reasoning not attached to tool turn: %s", chat)
		}
		again, err := FromChatRequest(chat)
		if err != nil {
			t.Fatal(err)
		}
		replayed, err := ToChatRequest(again)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(bridgeTestJSON(t, bridgeTestObject(t, chat)["messages"]), bridgeTestJSON(t, bridgeTestObject(t, replayed)["messages"])) {
			t.Fatalf("tool continuation did not roundtrip: %s", replayed)
		}
	}
}

func TestChatBridgeReasoningHistoryBoundaries(t *testing.T) {
	for _, input := range []string{
		`[{"type":"reasoning","summary":[{"type":"summary_text","text":"first "}]},{"type":"reasoning","summary":[{"type":"summary_text","text":"second"}]},{"role":"assistant","content":"answer"}]`,
		`[{"type":"reasoning","content":[{"type":"reasoning_text","text":"first second"}]},{"role":"assistant","content":"answer"}]`,
		`[{"type":"reasoning","summary":"first second","content":"first second"},{"role":"assistant","content":"answer"}]`,
	} {
		out, err := ToChatRequest([]byte(`{"model":"m","input":` + input + `}`))
		if err != nil {
			t.Fatal(err)
		}
		messages := bridgeTestObject(t, out)["messages"].([]any)
		if len(messages) != 1 || bridgeMap(messages[0])["reasoning_content"] != "first second" || bridgeMap(messages[0])["content"] != "answer" {
			t.Fatalf("plain history lost content or doubled sanitizer copy: %s", out)
		}
	}
	for _, item := range []string{
		`{"type":"reasoning","summary":[{"type":"summary_text","text":"plain"}],"encrypted_content":"opaque"}`,
		`{"type":"reasoning","summary":[{"type":"summary_text","text":"plain"}],"signature":"opaque"}`,
		`{"type":"reasoning","summary":[{"type":"summary_text","text":"summary"}],"content":"different full text"}`,
		`{"type":"reasoning","summary":[{"type":"summary_text","text":1}]}`,
		`{"type":"reasoning","summary":[{"type":"summary_text","text":"plain","signature":"opaque"}]}`,
		`{"type":"reasoning","summary":[]}`,
	} {
		if out, err := ToChatRequest([]byte(`{"model":"m","input":[` + item + `]}`)); err == nil || len(out) != 0 {
			t.Fatalf("unrepresentable history accepted: %s", item)
		}
		if out, err := ToChatResponse([]byte(`{"id":"r","object":"response","model":"m","status":"completed","output":[` + item + `]}`)); err == nil || len(out) != 0 {
			t.Fatalf("unrepresentable response accepted: %s", item)
		}
	}
	for _, role := range []string{"user", "system", "tool"} {
		if _, err := FromChatRequest([]byte(fmt.Sprintf(`{"model":"m","messages":[{"role":%q,"content":"hi","reasoning_content":"wrong role"}]}`, role))); err == nil {
			t.Fatalf("non-assistant reasoning accepted: %s", role)
		}
	}
}

func reasoningStreamFixture(t *testing.T) ([]bridgeObject, []byte) {
	t.Helper()
	frames := []bridgeObject{
		bridgeChatChunk(bridgeObject{"role": "assistant", "content": "", "reasoning_content": ""}, nil),
		bridgeChatChunk(bridgeObject{"reasoning_content": "Check "}, nil),
		bridgeChatChunk(bridgeObject{"reasoning_content": "fixture.", "content": "Reading", "tool_calls": []any{bridgeCall(0, "c", "read", `{"offset":`)}}, nil),
		bridgeChatChunk(bridgeObject{"tool_calls": []any{bridgeCall(0, "", "", `9007199254740993}`)}}, "tool_calls"),
		{"id": "c1", "object": "chat.completion.chunk", "model": "m", "choices": []any{}, "usage": bridgeObject{"prompt_tokens": 12, "completion_tokens": 7, "total_tokens": 19, "completion_tokens_details": bridgeObject{"reasoning_tokens": 4}}},
	}
	stream := NewChatStream("fallback")
	var all bytes.Buffer
	for i, frame := range frames {
		out, err := stream.TransformEvent(bridgeTestFrame(t, frame))
		if err != nil {
			t.Fatal(err)
		}
		if i == 1 && !bytes.Contains(out, []byte(`"delta":"Check "`)) {
			t.Fatal("reasoning buffered until stream end")
		}
		if bytes.Contains(out, []byte("event: response.completed")) {
			t.Fatal("completed emitted before actual Chat terminal")
		}
		all.Write(out)
	}
	out, err := stream.TransformEvent([]byte("data: [DONE]\n\n"))
	if err != nil {
		t.Fatal(err)
	}
	all.Write(out)
	if _, err := stream.Finish(); err != nil {
		t.Fatal(err)
	}
	return bridgeTestFrames(t, all.Bytes()), all.Bytes()
}

func TestChatBridgeReasoningSSELifecycleAndFinalReplay(t *testing.T) {
	events, raw := reasoningStreamFixture(t)
	counts := map[string]int{}
	for i, event := range events {
		counts[bridgeString(event["type"])]++
		if event["sequence_number"] != json.Number(fmt.Sprint(i)) {
			t.Fatal("non-contiguous sequence numbers")
		}
		if strings.HasPrefix(bridgeString(event["type"]), "response.reasoning_summary_") {
			if event["item_id"] != "rs_c1" || event["output_index"] != json.Number("0") || event["summary_index"] != json.Number("0") || event["content_index"] != nil {
				t.Fatalf("reasoning event has wrong identity/index: %v", event)
			}
		}
	}
	for event, want := range map[string]int{"response.reasoning_summary_part.added": 1, "response.reasoning_summary_text.delta": 2, "response.reasoning_summary_text.done": 1, "response.reasoning_summary_part.done": 1, "response.output_item.done": 3, "response.completed": 1} {
		if counts[event] != want {
			t.Fatalf("%s count=%d want=%d", event, counts[event], want)
		}
	}
	final := bridgeMap(events[len(events)-1]["response"])
	output := final["output"].([]any)
	if len(output) != 3 || bridgeMap(output[0])["type"] != "reasoning" || bridgeMap(bridgeMap(final["usage"])["output_tokens_details"])["reasoning_tokens"] != json.Number("4") {
		t.Fatal("final output or reasoning usage lost")
	}
	text, err := bridgeReasoningText(bridgeMap(output[0]))
	if err != nil || text != "Check fixture." {
		t.Fatal("final reasoning disagrees with source fragments")
	}
	request, err := ToChatRequest(bridgeTestJSON(t, bridgeObject{"model": "m", "input": append(output, bridgeObject{"type": "function_call_output", "call_id": "c", "output": "done"})}))
	if err != nil || bridgeMap(bridgeTestObject(t, request)["messages"].([]any)[0])["reasoning_content"] != text {
		t.Fatalf("completed SSE output cannot continue: %v", err)
	}
	reverse := NewResponsesStream("fallback")
	var chat bytes.Buffer
	for _, frame := range bytes.Split(raw, []byte("\n\n")) {
		if len(frame) == 0 {
			continue
		}
		out, err := reverse.TransformEvent(append(frame, '\n', '\n'))
		if err != nil {
			t.Fatalf("reverse: %v, frame=%s", err, frame)
		}
		chat.Write(out)
	}
	if _, err := reverse.Finish(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(chat.Bytes(), []byte(`"reasoning_content":"Check "`)) || !bytes.Contains(chat.Bytes(), []byte(`"reasoning_content":"fixture."`)) || !bytes.Contains(chat.Bytes(), []byte(`"reasoning_tokens":4`)) || !bytes.Contains(chat.Bytes(), []byte(`"finish_reason":"tool_calls"`)) || bytes.Count(chat.Bytes(), []byte("[DONE]")) != 1 {
		t.Fatalf("reverse lost reasoning/tool/usage: %s", chat.Bytes())
	}
}

// Independent native fixture: two summary parts, no visible answer, and an
// explicit token-limit result. No output of ChatStream constructs this fixture.
func nativeReasoningEvents(t *testing.T) []bridgeObject {
	t.Helper()
	raw := []string{
		`{"type":"response.created","response":{"id":"r","model":"m","status":"in_progress","output":[]}}`,
		`{"type":"response.output_item.added","output_index":0,"item":{"id":"rs","type":"reasoning","summary":[],"status":"in_progress"}}`,
		`{"type":"response.reasoning_summary_part.added","item_id":"rs","output_index":0,"summary_index":0,"part":{"type":"summary_text"}}`,
		`{"type":"response.reasoning_summary_text.delta","item_id":"rs","output_index":0,"summary_index":0,"delta":"First"}`,
		`{"type":"response.reasoning_summary_text.done","item_id":"rs","output_index":0,"summary_index":0,"text":"First"}`,
		`{"type":"response.reasoning_summary_part.done","item_id":"rs","output_index":0,"summary_index":0,"part":{"type":"summary_text","text":"First"}}`,
		`{"type":"response.reasoning_summary_part.added","item_id":"rs","output_index":0,"summary_index":1,"part":{"type":"summary_text","text":""}}`,
		`{"type":"response.reasoning_summary_text.delta","item_id":"rs","output_index":0,"summary_index":1,"delta":" second"}`,
		`{"type":"response.reasoning_summary_text.done","item_id":"rs","output_index":0,"summary_index":1,"text":" second"}`,
		`{"type":"response.reasoning_summary_part.done","item_id":"rs","output_index":0,"summary_index":1,"part":{"type":"summary_text","text":" second"}}`,
		`{"type":"response.output_item.done","output_index":0,"item":{"id":"rs","type":"reasoning","status":"incomplete","summary":[{"type":"summary_text","text":"First"},{"type":"summary_text","text":" second"}]}}`,
		`{"type":"response.incomplete","response":{"id":"r","model":"m","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[{"id":"rs","type":"reasoning","status":"incomplete","summary":[{"type":"summary_text","text":"First"},{"type":"summary_text","text":" second"}]}],"usage":{"output_tokens":8,"output_tokens_details":{"reasoning_tokens":8}}}}`,
	}
	result := make([]bridgeObject, 0, len(raw))
	for _, value := range raw {
		result = append(result, bridgeTestObject(t, []byte(value)))
	}
	return result
}

func TestResponsesReasoningNativeSSEAndNegativeLifecycle(t *testing.T) {
	stream := NewResponsesStream("m")
	var result bytes.Buffer
	for _, event := range nativeReasoningEvents(t) {
		out, err := stream.TransformEvent(bridgeTestFrame(t, event))
		if err != nil {
			t.Fatal(err)
		}
		result.Write(out)
	}
	if _, err := stream.Finish(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(result.Bytes(), []byte(`"reasoning_content":"First"`)) || !bytes.Contains(result.Bytes(), []byte(`"reasoning_content":" second"`)) || !bytes.Contains(result.Bytes(), []byte(`"finish_reason":"length"`)) || !bytes.Contains(result.Bytes(), []byte(`"reasoning_tokens":8`)) || bytes.Contains(result.Bytes(), []byte(`"prompt_tokens":0`)) {
		t.Fatalf("native reasoning/limit/unknown usage changed: %s", result.Bytes())
	}
	for name, mutate := range map[string]func([]bridgeObject) []bridgeObject{
		"wrong done text":     func(events []bridgeObject) []bridgeObject { events[4]["text"] = "changed"; return events },
		"wrong summary index": func(events []bridgeObject) []bridgeObject { events[3]["summary_index"] = 1; return events },
		"wrong item id":       func(events []bridgeObject) []bridgeObject { events[3]["item_id"] = "different"; return events },
		"missing part close":  func(events []bridgeObject) []bridgeObject { return append(events[:5], events[6:]...) },
		"missing text done":   func(events []bridgeObject) []bridgeObject { return append(events[:4], events[5:]...) },
		"delta after done": func(events []bridgeObject) []bridgeObject {
			return append(append(events[:5:5], events[3]), events[5:]...)
		},
		"encrypted start": func(events []bridgeObject) []bridgeObject {
			bridgeMap(events[1]["item"])["encrypted_content"] = "opaque"
			return events
		},
		"encrypted done": func(events []bridgeObject) []bridgeObject {
			bridgeMap(events[10]["item"])["encrypted_content"] = "opaque"
			return events
		},
		"encrypted delta": func(events []bridgeObject) []bridgeObject {
			events[3]["encrypted_content"] = "opaque"
			return events
		},
		"contradictory final": func(events []bridgeObject) []bridgeObject {
			item := bridgeMap(bridgeMap(events[11]["response"])["output"].([]any)[0])
			bridgeMap(item["summary"].([]any)[0])["text"] = "changed"
			return events
		},
		"encrypted final": func(events []bridgeObject) []bridgeObject {
			bridgeMap(bridgeMap(events[11]["response"])["output"].([]any)[0])["encrypted_content"] = "opaque"
			return events
		},
		"overlapping parts": func(events []bridgeObject) []bridgeObject {
			return append(append(events[:4:4], events[6]), events[4:]...)
		},
		"text disguised as summary": func(events []bridgeObject) []bridgeObject {
			bridgeMap(events[2]["part"])["type"] = "output_text"
			return events
		},
	} {
		t.Run(name, func(t *testing.T) {
			stream := NewResponsesStream("m")
			failed := false
			for _, event := range mutate(nativeReasoningEvents(t)) {
				out, err := stream.TransformEvent(bridgeTestFrame(t, event))
				if bytes.Contains(out, []byte("[DONE]")) {
					t.Fatal("invalid reasoning stream emitted successful terminal")
				}
				if err != nil {
					failed = true
					break
				}
			}
			if !failed {
				t.Fatal("broken reasoning lifecycle accepted")
			}
			if out, err := stream.TransformEvent([]byte("data: [DONE]\n\n")); err == nil || len(out) != 0 {
				t.Fatal("terminal revived failed stream")
			}
			if _, err := stream.Finish(); err == nil {
				t.Fatal("Finish revived failed stream")
			}
		})
	}
	events := nativeReasoningEvents(t)
	for cutoff := 0; cutoff < len(events); cutoff++ {
		stream := NewResponsesStream("m")
		for _, event := range events[:cutoff] {
			if _, err := stream.TransformEvent(bridgeTestFrame(t, event)); err != nil {
				t.Fatal(err)
			}
		}
		if out, err := stream.Finish(); err == nil || len(out) != 0 {
			t.Fatal("truncated native reasoning became successful")
		}
	}
}

func TestChatReasoningOnlyLimitAndBoundedAccumulation(t *testing.T) {
	empty := NewChatStream("m")
	if _, err := empty.TransformEvent(bridgeTestFrame(t, bridgeChatChunk(bridgeObject{"role": "assistant", "content": ""}, "stop"))); err != nil {
		t.Fatal(err)
	}
	if out, err := empty.TransformEvent([]byte("data: [DONE]\n\n")); err != nil || !bytes.Contains(out, []byte("event: response.completed")) {
		t.Fatalf("empty completion regressed: %s %v", out, err)
	}
	for _, finish := range []string{"stop", "length"} {
		stream := NewChatStream("m")
		out, err := stream.TransformEvent(bridgeTestFrame(t, bridgeChatChunk(bridgeObject{"reasoning_content": "Supplied fixture text."}, finish)))
		if err != nil || bytes.Contains(out, []byte("event: response.completed")) {
			t.Fatalf("reasoning-only stream rejected/finished early: %v", err)
		}
		out, err = stream.TransformEvent([]byte("data: [DONE]\n\n"))
		if err != nil {
			t.Fatal(err)
		}
		events := bridgeTestFrames(t, out)
		final := bridgeMap(events[len(events)-1]["response"])
		status := "completed"
		if finish == "length" {
			status = "incomplete"
		}
		if final["status"] != status || final["usage"] != nil || len(final["output"].([]any)) != 1 {
			t.Fatal("reasoning-only final changed status or invented usage")
		}
	}
	stream := NewChatStream("m")
	part := strings.Repeat("x", 1<<20)
	for i := 0; i < 8; i++ {
		if _, err := stream.TransformEvent(bridgeTestFrame(t, bridgeChatChunk(bridgeObject{"reasoning_content": part}, nil))); err != nil {
			t.Fatal(err)
		}
	}
	if out, err := stream.TransformEvent(bridgeTestFrame(t, bridgeChatChunk(bridgeObject{"content": "x"}, nil))); err == nil || len(out) != 0 {
		t.Fatal("reasoning evaded the shared accumulation limit")
	}
	if _, err := stream.Finish(); err == nil {
		t.Fatal("Finish repaired overflow")
	}
}
