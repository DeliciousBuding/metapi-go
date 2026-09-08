package messages_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/deliciousbuding/metapi-go/transform/anthropic/messages"
)

func chatChunk(t *testing.T, delta any, finish any, fields map[string]any) string {
	t.Helper()
	value := map[string]any{"id": "chatcmpl-stream", "model": "m", "object": "chat.completion.chunk", "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}}
	for field, entry := range fields {
		value[field] = entry
	}
	return "data: " + string(marshalJSON(t, value)) + "\n\n"
}

func toolDelta(index any, id, name, arguments string) map[string]any {
	call := map[string]any{"id": id, "type": "function", "function": map[string]any{"name": name, "arguments": arguments}}
	if index != nil {
		call["index"] = index
	}
	return call
}

func toolChunk(t *testing.T, calls ...any) string {
	t.Helper()
	return chatChunk(t, map[string]any{"tool_calls": calls}, nil, nil)
}

func runStream(t *testing.T, stream *messages.ChatStream, frames ...string) *nativeConsumer {
	t.Helper()
	consumer := &nativeConsumer{}
	for _, frame := range frames {
		out, err := stream.TransformEvent([]byte(frame))
		if err != nil {
			t.Fatalf("frame failed: %v\n%s", err, frame)
		}
		consumer.consume(t, out)
	}
	out, err := stream.Finish()
	if err != nil {
		t.Fatal(err)
	}
	consumer.consume(t, out)
	if !consumer.done {
		t.Fatal("no native message_stop")
	}
	if again, err := stream.Finish(); err != nil || len(again) != 0 {
		t.Fatalf("Finish was not idempotent: %s, %v", again, err)
	}
	return consumer
}

func TestChatStreamTextAndUsageOnlyTail(t *testing.T) {
	t.Parallel()
	first := chatChunk(t, map[string]any{"role": "assistant", "content": ""}, nil, nil)
	// A complete multi-line data frame, CRLF framing and metadata SSE fields.
	first = "event: message\r\nid: transport-id\r\nretry: 500\r\n" + strings.ReplaceAll(strings.Replace(first, `,"id"`, ",\ndata: \"id\"", 1), "\n", "\r\n")
	if !strings.Contains(first, "\r\ndata: \"id\"") {
		t.Fatal("test fixture did not create a multi-line SSE data frame")
	}
	consumer := runStream(t, messages.NewChatStream("fallback"),
		": keepalive\n\n", first,
		chatChunk(t, map[string]any{"reasoning_content": "not a final answer"}, nil, nil),
		chatChunk(t, map[string]any{"content": "Hello "}, nil, nil),
		chatChunk(t, map[string]any{"content": "世界"}, "stop", nil),
		chatChunk(t, nil, nil, map[string]any{"choices": []any{}, "usage": map[string]any{"prompt_tokens": 30, "completion_tokens": 6}}),
		"data: [DONE]\n\n",
	)
	requireJSON(t, marshalJSON(t, consumer.message), `{"id":"chatcmpl-stream","type":"message","role":"assistant","model":"m","content":[{"type":"text","text":"Hello 世界"}],"stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":30,"output_tokens":6}}`)
	if len(consumer.startUsage) != 0 {
		t.Fatalf("unknown initial usage was fabricated: %#v", consumer.startUsage)
	}
	if consumer.count("message_start") != 1 || consumer.count("content_block_start") != 1 || consumer.count("message_delta") != 1 || consumer.count("message_stop") != 1 {
		t.Fatalf("unexpected event sequence: %v", consumer.types)
	}
}

func TestChatStreamParallelToolsAndEmptyIdentity(t *testing.T) {
	t.Parallel()
	consumer := runStream(t, messages.NewChatStream("m"),
		chatChunk(t, map[string]any{"role": "assistant", "content": "Reading both."}, nil, nil),
		toolChunk(t,
			toolDelta(0, "tool-a", "Read", `{"file_`),
			toolDelta(1, "tool-b", "Read", `{"file_path":"b`)),
		chatChunk(t, map[string]any{"tool_calls": []any{
			toolDelta(0, "", "", `path":"a.txt"}`),
			toolDelta(nil, "tool-b", "", `.txt"}`),
		}}, nil, map[string]any{"id": "", "model": ""}),
		// Repeated full identity is not concatenated or a new call.
		toolChunk(t, toolDelta(0, "tool-a", "Read", "")),
		chatChunk(t, map[string]any{}, "stop", nil),
		chatChunk(t, nil, nil, map[string]any{"choices": []any{}, "usage": map[string]any{"prompt_tokens": 64, "completion_tokens": 8, "prompt_tokens_details": map[string]any{"cached_tokens": 16}}}),
		"data: [DONE]\n\n",
	)
	requireJSON(t, marshalJSON(t, consumer.message), `{"id":"chatcmpl-stream","type":"message","role":"assistant","model":"m","content":[{"type":"text","text":"Reading both."},{"type":"tool_use","id":"tool-a","name":"Read","input":{"file_path":"a.txt"}},{"type":"tool_use","id":"tool-b","name":"Read","input":{"file_path":"b.txt"}}],"stop_reason":"tool_use","stop_sequence":null,"usage":{"input_tokens":48,"output_tokens":8,"cache_read_input_tokens":16}}`)
	if consumer.count("content_block_start") != 3 || consumer.count("content_block_stop") != 3 {
		t.Fatalf("duplicate or unclosed block: %v", consumer.types)
	}
}

func TestChatStreamPendingIdentityAndSparseIndices(t *testing.T) {
	t.Parallel()
	consumer := runStream(t, messages.NewChatStream("m"),
		toolChunk(t, toolDelta(7, "", "", `{"x":`)),
		toolChunk(t, toolDelta(19, "tool-b", "Read", `{}`)),
		toolChunk(t, toolDelta(7, "tool-a", "Read", `1}`)),
		chatChunk(t, map[string]any{}, "tool_calls", nil), "data: [DONE]\n\n",
	)
	blocks := jsonArray(t, consumer.message["content"])
	if len(blocks) != 2 {
		t.Fatalf("wrong blocks: %#v", blocks)
	}
	byID := make(map[string]any)
	for _, item := range blocks {
		block := jsonObject(t, item)
		byID[block["id"].(string)] = block["input"]
	}
	requireJSON(t, marshalJSON(t, byID), `{"tool-a":{"x":1},"tool-b":{}}`)
}

func TestChatStreamToolArgumentsAtEveryBoundary(t *testing.T) {
	t.Parallel()
	arguments := `{"file_path":"中文.txt","offset":9007199254740993,"quote":"a\"b","escaped":"\u0041"}`
	for split := 0; split <= len(arguments); split++ {
		if !utf8.ValidString(arguments[:split]) || !utf8.ValidString(arguments[split:]) {
			continue
		}
		t.Run(fmt.Sprint(split), func(t *testing.T) {
			consumer := runStream(t, messages.NewChatStream("m"),
				toolChunk(t, toolDelta(0, "tool-a", "Read", arguments[:split])),
				toolChunk(t, toolDelta(0, "", "", arguments[split:])),
				chatChunk(t, map[string]any{}, "tool_calls", nil), "data: [DONE]\n\n",
			)
			block := jsonObject(t, jsonArray(t, consumer.message["content"])[0])
			requireJSON(t, marshalJSON(t, block["input"]), arguments)
		})
	}
}

func TestChatStreamUsageFieldsMergeWithoutInvention(t *testing.T) {
	t.Parallel()
	consumer := runStream(t, messages.NewChatStream("m"),
		chatChunk(t, map[string]any{"role": "assistant", "content": "hello"}, nil, map[string]any{"usage": map[string]any{"prompt_tokens": 20, "prompt_tokens_details": map[string]any{"cached_tokens": 4}}}),
		chatChunk(t, map[string]any{}, "length", nil),
		chatChunk(t, nil, nil, map[string]any{"choices": []any{}, "usage": map[string]any{"completion_tokens": 0}}),
		"data: [DONE]\n\n",
	)
	requireJSON(t, marshalJSON(t, consumer.startUsage), `{"input_tokens":16,"cache_read_input_tokens":4}`)
	requireJSON(t, marshalJSON(t, consumer.message["usage"]), `{"input_tokens":16,"cache_read_input_tokens":4,"output_tokens":0}`)
	if consumer.message["stop_reason"] != "max_tokens" {
		t.Fatal("length was relabeled as ordinary success")
	}
	consumer = runStream(t, messages.NewChatStream("fallback-model"),
		chatChunk(t, map[string]any{"content": ""}, "stop", map[string]any{"model": nil}), "data: [DONE]\n\n",
	)
	if consumer.message["model"] != "fallback-model" {
		t.Fatal("fallback model not used when upstream omits it")
	}
	requireJSON(t, marshalJSON(t, consumer.message["usage"]), `{}`)
}

func TestChatStreamErrorsAndTruncationNeverSucceed(t *testing.T) {
	t.Parallel()
	text := chatChunk(t, map[string]any{"content": "partial"}, nil, nil)
	finish := chatChunk(t, map[string]any{}, "stop", nil)
	for _, tc := range []struct {
		name   string
		frames []string
	}{
		{"empty stream", nil},
		{"no terminal", []string{text}},
		{"no DONE even with finish", []string{text, finish}},
		{"DONE without finish", []string{text, "data: [DONE]\n\n"}},
		{"bare DONE", []string{"data: [DONE]\n\n"}},
		{"usage without finish", []string{text, chatChunk(t, nil, nil, map[string]any{"choices": []any{}, "usage": map[string]any{"completion_tokens": 1}}), "data: [DONE]\n\n"}},
		{"JSON error", []string{text, "data: {\"error\":{\"message\":\"SHOULD_NOT_LEAK\"}}\n\n"}},
		{"error event after finish", []string{text, finish, "event: error\ndata: {\"message\":\"failed\"}\n\n"}},
		{"error event without data", []string{"event: error\n\n"}},
		{"nested delta error", []string{chatChunk(t, map[string]any{"error": map[string]any{"message": "failed"}}, nil, nil)}},
		{"filter finish", []string{text, chatChunk(t, map[string]any{}, "content_filter", nil)}},
		{"refusal", []string{chatChunk(t, map[string]any{"refusal": "no"}, "stop", nil)}},
		{"reasoning only", []string{chatChunk(t, map[string]any{"reasoning_content": "private"}, "stop", nil), "data: [DONE]\n\n"}},
		{"content after finish", []string{text, finish, text}},
		{"changed model", []string{text, chatChunk(t, map[string]any{}, nil, map[string]any{"model": "another"})}},
		{"changed response ID", []string{text, chatChunk(t, map[string]any{}, nil, map[string]any{"id": "another"})}},
		{"wrong response kind", []string{chatChunk(t, map[string]any{"content": "x"}, "stop", map[string]any{"object": "response"})}},
		{"choice index", []string{chatChunk(t, nil, nil, map[string]any{"choices": []any{map[string]any{"index": 1, "delta": map[string]any{"content": "x"}}}})}},
		{"two choices", []string{chatChunk(t, nil, nil, map[string]any{"choices": []any{map[string]any{}, map[string]any{}}})}},
		{"negative usage", []string{text, chatChunk(t, nil, nil, map[string]any{"choices": []any{}, "usage": map[string]any{"completion_tokens": -1}})}},
		{"malformed frame JSON", []string{"data: {\n\n"}},
		{"missing frame delimiter", []string{strings.TrimSuffix(text, "\n\n")}},
		{"two frames at once", []string{text + finish}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stream := messages.NewChatStream("m")
			var all strings.Builder
			var failure error
			for _, frame := range tc.frames {
				out, err := stream.TransformEvent([]byte(frame))
				all.Write(out)
				if err != nil {
					failure = err
					if len(out) != 0 {
						t.Fatal("error also returned successful frame bytes")
					}
					break
				}
			}
			out, err := stream.Finish()
			all.Write(out)
			if failure == nil {
				failure = err
			}
			if failure == nil || err == nil {
				t.Fatalf("bad stream/EOF succeeded: %s", all.String())
			}
			if strings.Contains(all.String(), "event: message_stop") || strings.Contains(all.String(), "event: message_delta") {
				t.Fatalf("error/truncation produced a success terminal: %s", all.String())
			}
			if strings.Contains(failure.Error(), "SHOULD_NOT_LEAK") {
				t.Fatal("raw upstream error leaked")
			}
			if out, err := stream.TransformEvent([]byte("data: [DONE]\n\n")); err == nil || len(out) != 0 {
				t.Fatal("failed stream could be revived")
			}
		})
	}
}

func TestChatStreamRejectsBrokenToolCalls(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		frames []string
	}{
		{"truncated JSON", []string{toolChunk(t, toolDelta(0, "a", "Read", `{"file_path":`))}},
		{"null input", []string{toolChunk(t, toolDelta(0, "a", "Read", "null"))}},
		{"array input", []string{toolChunk(t, toolDelta(0, "a", "Read", "[]"))}},
		{"empty arguments", []string{toolChunk(t, toolDelta(0, "a", "Read", ""))}},
		{"missing ID", []string{toolChunk(t, toolDelta(0, "", "Read", "{}"))}},
		{"missing name", []string{toolChunk(t, toolDelta(0, "a", "", "{}"))}},
		{"changed ID", []string{toolChunk(t, toolDelta(0, "a", "Read", "{")), toolChunk(t, toolDelta(0, "b", "", "}"))}},
		{"changed name", []string{toolChunk(t, toolDelta(0, "a", "Read", "{")), toolChunk(t, toolDelta(0, "", "Write", "}"))}},
		{"duplicate ID across indices", []string{toolChunk(t, toolDelta(0, "a", "Read", "{}"), toolDelta(1, "a", "Read", "{}"))}},
		{"conflicting index and ID", []string{toolChunk(t, toolDelta(0, "a", "Read", "{"), toolDelta(1, "b", "Read", "{")), toolChunk(t, toolDelta(0, "b", "", "}"))}},
		{"no identity cannot use array position", []string{toolChunk(t, toolDelta(0, "a", "Read", "{")), toolChunk(t, toolDelta(nil, "", "", "}"))}},
		{"negative index", []string{toolChunk(t, toolDelta(-1, "a", "Read", "{}"))}},
		{"noninteger index", []string{toolChunk(t, toolDelta(0.5, "a", "Read", "{}"))}},
		{"nonfunction tool", []string{toolChunk(t, map[string]any{"index": 0, "id": "a", "type": "custom"})}},
		{"nonstring arguments", []string{toolChunk(t, map[string]any{"index": 0, "id": "a", "function": map[string]any{"name": "Read", "arguments": map[string]any{}}})}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stream := messages.NewChatStream("m")
			frames := append(append([]string{}, tc.frames...), chatChunk(t, map[string]any{}, "length", nil), "data: [DONE]\n\n")
			var all strings.Builder
			var failure error
			for _, frame := range frames {
				out, err := stream.TransformEvent([]byte(frame))
				all.Write(out)
				if err != nil {
					failure = err
					break
				}
			}
			if failure == nil {
				t.Fatal("malformed tool call became success")
			}
			if strings.Contains(all.String(), "event: message_stop") {
				t.Fatal("malformed tool call emitted native success")
			}
			if _, err := stream.Finish(); err == nil {
				t.Fatal("Finish repaired a broken tool call")
			}
		})
	}
}

func TestChatStreamStateIsPerRequest(t *testing.T) {
	t.Parallel()
	for i := range 8 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			arguments := fmt.Sprintf(`{"request":%d}`, i)
			consumer := runStream(t, messages.NewChatStream("m"), toolChunk(t, toolDelta(0, "same-tool-id", "Read", arguments)), chatChunk(t, map[string]any{}, "tool_calls", nil), "data: [DONE]\n\n")
			block := jsonObject(t, jsonArray(t, consumer.message["content"])[0])
			requireJSON(t, marshalJSON(t, block["input"]), arguments)
		})
	}
}

func TestChatStreamRejectsDataAfterDONE(t *testing.T) {
	t.Parallel()
	stream := messages.NewChatStream("m")
	runStream(t, stream, chatChunk(t, map[string]any{"content": "ok"}, "stop", nil), "data: [DONE]\n\n")
	if out, err := stream.TransformEvent([]byte(": heartbeat\n\n")); err != nil || len(out) != 0 {
		t.Fatalf("post-terminal comment: %s, %v", out, err)
	}
	if out, err := stream.TransformEvent([]byte("data: {\"error\":{}}\n\n")); err == nil || len(out) != 0 {
		t.Fatal("post-terminal data was ignored")
	}
	if _, err := stream.Finish(); err == nil {
		t.Fatal("post-terminal error did not poison Finish")
	}
}

// A small independent Messages consumer, not an inspection of bridge internals.
// In particular, start indices must match append order, deltas cannot reference
// unstarted/stopped blocks, tool JSON must be complete at stop, and the terminal
// may only appear after all content blocks have closed.
type nativeConsumer struct {
	message, startUsage map[string]any
	blocks              []consumerBlock
	types               []string
	terminal, done      bool
}

type consumerBlock struct {
	value   map[string]any
	args    string
	stopped bool
}

func (c *nativeConsumer) count(kind string) int {
	n := 0
	for _, value := range c.types {
		if value == kind {
			n++
		}
	}
	return n
}

func (c *nativeConsumer) consume(t *testing.T, output []byte) {
	t.Helper()
	if len(output) == 0 {
		return
	}
	if !strings.HasSuffix(string(output), "\n\n") || strings.Contains(string(output), "[DONE]") {
		t.Fatalf("not complete native frames: %q", output)
	}
	for _, block := range strings.Split(strings.TrimSuffix(string(output), "\n\n"), "\n\n") {
		lines := strings.Split(block, "\n")
		if len(lines) != 2 || !strings.HasPrefix(lines[0], "event: ") || !strings.HasPrefix(lines[1], "data: ") {
			t.Fatalf("invalid native SSE block: %q", block)
		}
		event := strings.TrimPrefix(lines[0], "event: ")
		value := jsonObject(t, decodeJSON(t, []byte(strings.TrimPrefix(lines[1], "data: "))))
		if value["type"] != event || c.done {
			t.Fatalf("event/type mismatch or event after stop: %q", block)
		}
		c.types = append(c.types, event)
		switch event {
		case "message_start":
			if c.message != nil {
				t.Fatal("duplicate message_start")
			}
			c.message = jsonObject(t, value["message"])
			if c.message["type"] != "message" || c.message["role"] != "assistant" || len(jsonArray(t, c.message["content"])) != 0 {
				t.Fatalf("bad message_start: %s", block)
			}
			c.startUsage = jsonObject(t, decodeJSON(t, marshalJSON(t, c.message["usage"])))
		case "content_block_start":
			if c.message == nil || c.terminal {
				t.Fatal("content start outside message")
			}
			index := consumerIndex(t, value)
			if index != len(c.blocks) {
				t.Fatalf("start index %d not contiguous with %d blocks", index, len(c.blocks))
			}
			item := jsonObject(t, value["content_block"])
			if item["type"] == "tool_use" && !reflect.DeepEqual(item["input"], map[string]any{}) {
				t.Fatal("tool start must carry an empty input object")
			}
			c.blocks = append(c.blocks, consumerBlock{value: item})
		case "content_block_delta", "content_block_stop":
			index := consumerIndex(t, value)
			if index < 0 || index >= len(c.blocks) || c.blocks[index].stopped || c.terminal {
				t.Fatalf("delta/stop for inactive block: %s", block)
			}
			item := &c.blocks[index]
			if event == "content_block_stop" {
				item.stopped = true
				if item.value["type"] == "tool_use" {
					item.value["input"] = jsonObject(t, decodeJSON(t, []byte(item.args)))
				}
				continue
			}
			delta := jsonObject(t, value["delta"])
			switch delta["type"] {
			case "text_delta":
				if item.value["type"] != "text" {
					t.Fatal("text delta for nontext block")
				}
				item.value["text"] = item.value["text"].(string) + delta["text"].(string)
			case "input_json_delta":
				if item.value["type"] != "tool_use" {
					t.Fatal("JSON delta for nontool block")
				}
				item.args += delta["partial_json"].(string)
			default:
				t.Fatalf("unexpected native delta: %s", block)
			}
		case "message_delta":
			if c.message == nil || c.terminal {
				t.Fatal("duplicate/out-of-order message_delta")
			}
			content := make([]any, 0, len(c.blocks))
			for _, item := range c.blocks {
				if !item.stopped {
					t.Fatal("message_delta before content block stop")
				}
				content = append(content, item.value)
			}
			c.message["content"] = content
			for key, entry := range jsonObject(t, value["delta"]) {
				c.message[key] = entry
			}
			for key, entry := range jsonObject(t, value["usage"]) {
				jsonObject(t, c.message["usage"])[key] = entry
			}
			c.terminal = true
		case "message_stop":
			if !c.terminal {
				t.Fatal("message_stop without message_delta")
			}
			c.done = true
		default:
			t.Fatalf("unexpected native event: %s", block)
		}
	}
}

func consumerIndex(t *testing.T, value map[string]any) int {
	t.Helper()
	index, ok := value["index"].(json.Number)
	if !ok {
		t.Fatal("content event missing integer index")
	}
	n, err := index.Int64()
	if err != nil {
		t.Fatal(err)
	}
	return int(n)
}

func TestChatStreamTextAndToolsInSameChunk(t *testing.T) {
	t.Parallel()
	consumer := runStream(t, messages.NewChatStream("m"),
		"event: ping\ndata: {\"type\":\"ping\"}\n\n",
		chatChunk(t, map[string]any{"content": "before", "tool_calls": []any{toolDelta(0, "a", "Read", "{")}}, nil, nil),
		chatChunk(t, map[string]any{"content": "after", "tool_calls": []any{toolDelta(0, "", "", "}")}}, "tool_calls", nil),
		"data: [DONE]\n\n",
	)
	requireJSON(t, marshalJSON(t, consumer.message["content"]), `[{"type":"text","text":"before"},{"type":"tool_use","id":"a","name":"Read","input":{}},{"type":"text","text":"after"}]`)
}

func TestChatStreamPingCannotHideUpstreamError(t *testing.T) {
	t.Parallel()
	stream := messages.NewChatStream("m")
	if out, err := stream.TransformEvent([]byte("event: ping\ndata: {\"error\":{\"message\":\"failed\"}}\n\n")); err == nil || len(out) != 0 {
		t.Fatal("upstream error was discarded as a ping")
	}
	if _, err := stream.Finish(); err == nil {
		t.Fatal("ping error was not sticky")
	}
}
