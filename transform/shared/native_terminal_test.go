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
