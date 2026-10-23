package shared

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

const nativeChatTool = `{"id":"chat-1","object":"chat.completion","model":"example","choices":[{"index":0,"message":{"role":"assistant","content":null,"reasoning_content":"keep reasoning","tool_calls":[{"id":"call-1","type":"function","function":{"name":"echo","arguments":"{\"value\":\"ok\"}"}}]},"finish_reason":"stop"}],"usage":{"total_tokens":9007199254740993},"extension":{"signature":"keep"}}`
const nativeMessageTool = `{"type":"message","role":"assistant","id":"msg-1","content":[{"type":"thinking","thinking":"keep","signature":"signed"},{"type":"tool_use","id":"tool-1","name":"echo","input":{"value":"ok"}}],"stop_reason":"end_turn","stop_sequence":null,"usage":{"output_tokens":12}}`

func TestNativeTerminalJSONRepairsOnlyCompleteToolStops(t *testing.T) {
	for _, tc := range []struct {
		name      string
		protocol  NativeTerminalProtocol
		raw, want string
	}{
		{"chat", NativeChatCompletions, nativeChatTool, `"finish_reason":"tool_calls"`},
		{"messages", NativeMessages, nativeMessageTool, `"stop_reason":"tool_use"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := NormalizeNativeTerminalJSON([]byte(tc.raw), tc.protocol)
			if !bytes.Contains(out, []byte(tc.want)) {
				t.Fatalf("missing corrected terminal: %s", out)
			}
			var before, after map[string]json.RawMessage
			if err := json.Unmarshal([]byte(tc.raw), &before); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(out, &after); err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{"usage", "extension", "content", "id", "model"} {
				if !jsonEqual(before[key], after[key]) {
					t.Errorf("changed %s: %s != %s", key, before[key], after[key])
				}
			}
			if tc.protocol == NativeChatCompletions && !bytes.Contains(out, []byte(`9007199254740993`)) {
				t.Fatal("large integer rounded")
			}
		})
	}
}
func jsonEqual(a, b []byte) bool {
	var aa, bb bytes.Buffer
	_ = json.Compact(&aa, a)
	_ = json.Compact(&bb, b)
	return bytes.Equal(aa.Bytes(), bb.Bytes())
}

func TestNativeTerminalJSONDoesNotHideFailureOrAmbiguity(t *testing.T) {
	cases := []struct {
		name, raw string
		protocol  NativeTerminalProtocol
	}{
		{"length", strings.Replace(nativeChatTool, `"finish_reason":"stop"`, `"finish_reason":"length"`, 1), NativeChatCompletions},
		{"filter", strings.Replace(nativeChatTool, `"finish_reason":"stop"`, `"finish_reason":"content_filter"`, 1), NativeChatCompletions},
		{"missing_finish", strings.Replace(nativeChatTool, `"finish_reason":"stop"`, `"finish_reason":null`, 1), NativeChatCompletions},
		{"truncated_args", strings.Replace(nativeChatTool, `{\"value\":\"ok\"}`, `{\"value\":`, 1), NativeChatCompletions},
		{"non_object_args", strings.Replace(nativeChatTool, `{\"value\":\"ok\"}`, `[]`, 1), NativeChatCompletions},
		{"empty_id", strings.Replace(nativeChatTool, `"id":"call-1"`, `"id":""`, 1), NativeChatCompletions},
		{"refusal", strings.Replace(nativeChatTool, `"content":null`, `"content":null,"refusal":"not allowed"`, 1), NativeChatCompletions},
		{"error", strings.Replace(nativeChatTool, `"object":`, `"error":{"type":"failure"},"object":`, 1), NativeChatCompletions},
		{"duplicate_finish", strings.Replace(nativeChatTool, `"finish_reason":"stop"`, `"finish_reason":"length","finish_reason":"stop"`, 1), NativeChatCompletions},
		{"duplicate_root", strings.Replace(nativeChatTool, `"object":`, `"object":"other","object":`, 1), NativeChatCompletions},
		{"messages_max_tokens", strings.Replace(nativeMessageTool, `"end_turn"`, `"max_tokens"`, 1), NativeMessages},
		{"messages_sequence", strings.Replace(nativeMessageTool, `"stop_sequence":null`, `"stop_sequence":"end"`, 1), NativeMessages},
		{"messages_bad_input", strings.Replace(nativeMessageTool, `"input":{"value":"ok"}`, `"input":"bad"`, 1), NativeMessages},
		{"messages_empty_tool_name", strings.Replace(nativeMessageTool, `"name":"echo"`, `"name":""`, 1), NativeMessages},
		{"wrong_protocol", nativeMessageTool, NativeChatCompletions},
		{"not_json", "{bad", NativeChatCompletions},
		{"already_correct", strings.Replace(nativeChatTool, `"finish_reason":"stop"`, `"finish_reason":"tool_calls"`, 1), NativeChatCompletions},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := NormalizeNativeTerminalJSON([]byte(tc.raw), tc.protocol)
			if string(out) != tc.raw {
				t.Fatalf("rewrote protected payload: %s", out)
			}
		})
	}
}
func chatChunk(delta, reason string) []byte {
	return []byte(`{"id":"chat-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":` + delta + `,"finish_reason":` + reason + `}],"usage":{"total_tokens":9007199254740993}}`)
}

func TestNativeTerminalChatStreamNullAndCompletedTool(t *testing.T) {
	s := NativeTerminalStream{Protocol: NativeChatCompletions}
	start := chatChunk(`{"role":"assistant","tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"echo","arguments":"{\"value\":"}}]}`, `""`)
	out := s.AddData(start)
	if !bytes.Contains(out, []byte(`"finish_reason":null`)) {
		t.Fatalf("empty string not normalized: %s", out)
	}
	delta := chatChunk(`{"tool_calls":[{"index":0,"function":{"arguments":"\"ok\"}"}}]}`, `null`)
	if out := s.AddData(delta); !bytes.Equal(out, delta) {
		t.Fatalf("changed nonterminal data: %s", out)
	}
	stop := chatChunk(`{}`, `"stop"`)
	out = s.AddData(stop)
	if !bytes.Contains(out, []byte(`"finish_reason":"tool_calls"`)) {
		t.Fatalf("missing tool stop: %s", out)
	}
	if !bytes.Contains(out, []byte(`9007199254740993`)) {
		t.Fatal("lost exact usage")
	}
	if out := s.AddData(stop); !bytes.Equal(out, stop) {
		t.Fatal("changed repeated terminal")
	}
}

func TestNativeTerminalChatStreamDoesNotRepairIncompleteTools(t *testing.T) {
	for _, reason := range []string{`"stop"`, `"length"`, `"content_filter"`} {
		s := NativeTerminalStream{Protocol: NativeChatCompletions}
		s.AddData(chatChunk(`{"tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"echo","arguments":"{"}}]}`, `null`))
		stop := chatChunk(`{}`, reason)
		if out := s.AddData(stop); !bytes.Equal(out, stop) {
			t.Fatalf("rewrote incomplete/failed tool: %s", out)
		}
	}
	s := NativeTerminalStream{Protocol: NativeChatCompletions}
	stop := chatChunk(`{}`, `"stop"`)
	if out := s.AddData(stop); !bytes.Equal(out, stop) {
		t.Fatal("text stream became tool stream")
	}
}

func feedMessageTool(s *NativeTerminalStream, partial string, closeBlock bool) {
	s.AddData([]byte(`{"type":"message_start","message":{"id":"m","role":"assistant","content":[]}}`))
	s.AddData([]byte(`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"t","name":"echo","input":{}}}`))
	if partial != "" {
		encoded, _ := json.Marshal(partial)
		s.AddData([]byte(`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":` + string(encoded) + `}}`))
	}
	if closeBlock {
		s.AddData([]byte(`{"type":"content_block_stop","index":0}`))
	}
}
func TestNativeTerminalMessagesStreamRequiresCompleteClosedTool(t *testing.T) {
	for _, tc := range []struct {
		name, args, reason string
		closed, wantChange bool
	}{
		{"complete", `{"value":"ok"}`, "end_turn", true, true},
		{"empty_object", "", "end_turn", true, true},
		{"truncated", `{"value":`, "end_turn", true, false},
		{"open", `{}`, "end_turn", false, false},
		{"limit", `{}`, "max_tokens", true, false},
		{"refusal", `{}`, "refusal", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NativeTerminalStream{Protocol: NativeMessages}
			feedMessageTool(&s, tc.args, tc.closed)
			raw := []byte(`{"type":"message_delta","delta":{"stop_reason":"` + tc.reason + `","stop_sequence":null},"usage":{"output_tokens":37}}`)
			out := s.AddData(raw)
			if tc.wantChange {
				if !bytes.Contains(out, []byte(`"stop_reason":"tool_use"`)) {
					t.Fatalf("not repaired: %s", out)
				}
			} else if !bytes.Equal(raw, out) {
				t.Fatalf("invalid terminal repaired: %s", out)
			}
		})
	}
}

func TestNativeTerminalStreamStateIsBoundedAndDoesNotRepairAfterError(t *testing.T) {
	s := NativeTerminalStream{Protocol: NativeChatCompletions}
	fragment, _ := json.Marshal(strings.Repeat("a", terminalStateLimit+1))
	s.AddData(chatChunk(`{"tool_calls":[{"index":0,"id":"c","function":{"name":"echo","arguments":`+string(fragment)+`}}]}`, `null`))
	raw := chatChunk(`{}`, `""`)
	if out := s.AddData(raw); !bytes.Equal(out, raw) || !s.invalid || s.choices != nil {
		t.Fatal("overflow did not disable repair/release retained tool state")
	}
	for _, protocol := range []NativeTerminalProtocol{NativeChatCompletions, NativeMessages} {
		s := NativeTerminalStream{Protocol: protocol}
		s.AddData([]byte(`{"type":"error","error":{"message":"fail"}}`))
		raw := chatChunk(`{"content":"text"}`, `""`)
		if out := s.AddData(raw); !bytes.Equal(out, raw) {
			t.Fatal("repair after error")
		}
	}
}

func TestNativeTerminalChatStreamDoesNotRepairAfterDone(t *testing.T) {
	s := NativeTerminalStream{Protocol: NativeChatCompletions}
	s.AddData(chatChunk(`{"tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"echo","arguments":"{}"}}]}`, `null`))
	done := []byte(" [DONE]\n")
	if out := s.AddData(done); !bytes.Equal(out, done) {
		t.Fatal("changed stream delimiter")
	}
	late := chatChunk(`{}`, `"stop"`)
	if out := s.AddData(late); !bytes.Equal(out, late) {
		t.Fatalf("repaired data after the stream ended: %s", out)
	}
}

const nativeResponsesNullReasoningItem = `{"type":"response.output_item.done","output_index":0,"item":{"id":"rs-fixture","type":"reasoning","summary":[],"content":[{"type":"reasoning_text","text":"keep reasoning"}],"encrypted_content":null,"status":null,"extension":{"signature":"keep"}}}`

func TestNativeTerminalResponsesRepairsOnlyNullReasoningStatus(t *testing.T) {
	raw := []byte(`{"type":"response.output_item.done","output_index":0,"item":{"id":"rs-fixture","type":"reasoning","summary":[],"content":[{"type":"reasoning_text","text":"keep reasoning"}],"encrypted_content":null,"status":null,"extension":{"signature":"keep"}},"usage":{"input_tokens":9007199254740993}}`)
	stream := NativeTerminalStream{Protocol: NativeResponses}
	out := stream.AddData(raw)

	var before, after map[string]json.RawMessage
	if err := json.Unmarshal(raw, &before); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(out, &after); err != nil {
		t.Fatalf("normalized response is invalid JSON: %v\n%s", err, out)
	}
	for _, key := range []string{"type", "output_index", "usage"} {
		if !jsonEqual(before[key], after[key]) {
			t.Fatalf("root field %s changed: %s != %s", key, before[key], after[key])
		}
	}
	beforeItem := terminalObject(before["item"])
	afterItem := terminalObject(after["item"])
	if beforeItem == nil || afterItem == nil {
		t.Fatalf("item was not preserved: %s", out)
	}
	for _, key := range []string{"id", "type", "summary", "content", "encrypted_content", "extension"} {
		if !jsonEqual(beforeItem[key], afterItem[key]) {
			t.Fatalf("item field %s changed: %s != %s", key, beforeItem[key], afterItem[key])
		}
	}
	if got := string(afterItem["status"]); got != `"completed"` {
		t.Fatalf("reasoning status = %s, want completed\n%s", got, out)
	}
}

func TestNativeTerminalResponsesLeavesAmbiguousOrFailedItemsUntouched(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"missing status", strings.Replace(nativeResponsesNullReasoningItem, `,"status":null`, "", 1)},
		{"failed status", strings.Replace(nativeResponsesNullReasoningItem, `"status":null`, `"status":"failed"`, 1)},
		{"completed status", strings.Replace(nativeResponsesNullReasoningItem, `"status":null`, `"status":"completed"`, 1)},
		{"non reasoning", strings.Replace(nativeResponsesNullReasoningItem, `"type":"reasoning"`, `"type":"message"`, 1)},
		{"missing id", strings.Replace(nativeResponsesNullReasoningItem, `"id":"rs-fixture",`, "", 1)},
		{"missing summary", strings.Replace(nativeResponsesNullReasoningItem, `"summary":[],`, "", 1)},
		{"null summary", strings.Replace(nativeResponsesNullReasoningItem, `"summary":[]`, `"summary":null`, 1)},
		{"scalar content", strings.Replace(nativeResponsesNullReasoningItem, `"content":[{"type":"reasoning_text","text":"keep reasoning"}]`, `"content":"bad"`, 1)},
		{"missing output index", strings.Replace(nativeResponsesNullReasoningItem, `"output_index":0,`, "", 1)},
		{"wrong event", strings.Replace(nativeResponsesNullReasoningItem, `"response.output_item.done"`, `"response.output_item.added"`, 1)},
		{"item error", strings.Replace(nativeResponsesNullReasoningItem, `"status":null`, `"status":null,"error":{"type":"failure"}`, 1)},
		{"duplicate status", strings.Replace(nativeResponsesNullReasoningItem, `"status":null`, `"status":"incomplete","status":null`, 1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stream := NativeTerminalStream{Protocol: NativeResponses}
			if out := stream.AddData([]byte(tc.raw)); !bytes.Equal(out, []byte(tc.raw)) {
				t.Fatalf("ambiguous or failed item was rewritten: %s", out)
			}
		})
	}
}

func TestNativeTerminalResponsesStopsAfterTerminalEvent(t *testing.T) {
	stream := NativeTerminalStream{Protocol: NativeResponses}
	stream.AddData([]byte(`{"type":"response.completed","response":{"status":"completed"}}`))
	raw := []byte(nativeResponsesNullReasoningItem)
	if out := stream.AddData(raw); !bytes.Equal(out, raw) {
		t.Fatalf("repaired an item after the response completed: %s", out)
	}
}

const nativeResponsesCompletedDocument = `{"id":"resp-1","object":"response","model":"example","status":"completed","output":[{"id":"rs-1","type":"reasoning","summary":[],"content":[{"type":"reasoning_text","text":"keep"}],"encrypted_content":null,"status":null,"extension":{"signature":"keep"}},{"id":"msg-1","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],"usage":{"total_tokens":9007199254740993},"extension":{"signature":"root"}}`

func responsesReasoningStatus(t *testing.T, raw []byte) string {
	t.Helper()
	var doc struct {
		Output []struct {
			Type   string `json:"type"`
			Status string `json:"status"`
		} `json:"output"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("normalized response is invalid JSON: %v\n%s", err, raw)
	}
	if len(doc.Output) == 0 || doc.Output[0].Type != "reasoning" {
		t.Fatalf("reasoning item lost: %s", raw)
	}
	return doc.Output[0].Status
}

func TestNativeResponsesReasoningStatusInBufferedDocument(t *testing.T) {
	out := NormalizeNativeTerminalJSON([]byte(nativeResponsesCompletedDocument), NativeResponses)
	if got := responsesReasoningStatus(t, out); got != "completed" {
		t.Fatalf("reasoning status = %q, want completed\n%s", got, out)
	}
	for _, marker := range []string{`"text":"keep"`, `"encrypted_content":null`, `9007199254740993`, `"signature":"root"`, `"text":"ok"`} {
		if !bytes.Contains(out, []byte(marker)) {
			t.Fatalf("buffered response lost %q: %s", marker, out)
		}
	}
}

func TestNativeResponsesReasoningStatusLeavesNonCompletedDocumentsUntouched(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"failed response", strings.Replace(nativeResponsesCompletedDocument, `"status":"completed","output"`, `"status":"failed","output"`, 1)},
		{"incomplete response", strings.Replace(nativeResponsesCompletedDocument, `"status":"completed","output"`, `"status":"incomplete","output"`, 1)},
		{"in_progress response", strings.Replace(nativeResponsesCompletedDocument, `"status":"completed","output"`, `"status":"in_progress","output"`, 1)},
		{"missing response status", strings.Replace(nativeResponsesCompletedDocument, `"status":"completed","output"`, `"output"`, 1)},
		{"malformed item", strings.Replace(nativeResponsesCompletedDocument, `"content":[{"type":"reasoning_text","text":"keep"}],`, "", 1)},
		{"missing item status", strings.Replace(nativeResponsesCompletedDocument, `,"status":null`, "", 1)},
		{"item error", strings.Replace(nativeResponsesCompletedDocument, `"status":null`, `"status":null,"error":{"type":"failure"}`, 1)},
		{"wrong object", strings.Replace(nativeResponsesCompletedDocument, `"object":"response"`, `"object":"message"`, 1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if out := NormalizeNativeTerminalJSON([]byte(tc.raw), NativeResponses); !bytes.Equal(out, []byte(tc.raw)) {
				t.Fatalf("non-completed or malformed document was rewritten: %s", out)
			}
		})
	}
}

func TestNativeTerminalResponsesRepairsCompletedEventDocument(t *testing.T) {
	stream := NativeTerminalStream{Protocol: NativeResponses}
	raw := []byte(`{"type":"response.completed","response":` + nativeResponsesCompletedDocument + `}`)
	out := stream.AddData(raw)
	if bytes.Equal(out, raw) {
		t.Fatal("completed event document was not repaired")
	}
	var event struct {
		Response json.RawMessage `json:"response"`
	}
	if err := json.Unmarshal(out, &event); err != nil {
		t.Fatalf("normalized event is invalid JSON: %v\n%s", err, out)
	}
	if got := responsesReasoningStatus(t, event.Response); got != "completed" {
		t.Fatalf("reasoning status = %q, want completed\n%s", got, out)
	}
}
