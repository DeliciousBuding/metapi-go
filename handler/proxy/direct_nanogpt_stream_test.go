package proxyhandler

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
)

func nanoGPTFrame(choices any, usage bool) []byte {
	object := map[string]any{"id": "nano-response", "object": "chat.completion.chunk", "model": "provider-model", "choices": choices}
	if usage {
		object["usage"] = map[string]int{"prompt_tokens": 5, "completion_tokens": 2, "total_tokens": 7}
	}
	encoded, _ := json.Marshal(object)
	return append(append([]byte("data: "), encoded...), '\n', '\n')
}
func nanoGPTChoice(index int, delta map[string]any, finish any) []any {
	return []any{map[string]any{"index": index, "delta": delta, "finish_reason": finish}}
}

func nanoGPTParseOutputTools(t *testing.T, frames []byte) (string, []map[string]json.RawMessage) {
	t.Helper()
	var text strings.Builder
	var tools []map[string]json.RawMessage
	for _, line := range strings.Split(string(frames), "\n") {
		if !strings.HasPrefix(line, "data: ") || strings.TrimPrefix(line, "data: ") == "[DONE]" {
			continue
		}
		var object struct {
			Choices []struct {
				Delta struct {
					Content string                       `json:"content"`
					Tools   []map[string]json.RawMessage `json:"tool_calls"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &object); err != nil {
			t.Fatal(err)
		}
		for _, choice := range object.Choices {
			text.WriteString(choice.Delta.Content)
			tools = append(tools, choice.Delta.Tools...)
		}
	}
	return text.String(), tools
}

func TestDirectNanoGPTStreamXMLNativeCoexistenceAndStableIDs(t *testing.T) {
	content := "before\n<Read path=\"same\"/><Read path=\"same\"/>\n```xml\n<Read/>\n```\nafter"
	tools := nanoGPTTestTools(t)
	jsonBody, err := normalizeDirectNanoGPTJSON(nanoGPTTestResponse(content), tools)
	if err != nil {
		t.Fatal(err)
	}
	var want struct {
		Choices []struct {
			Message struct {
				Calls []struct {
					ID string `json:"id"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(jsonBody, &want) != nil {
		t.Fatal("invalid normalized JSON fixture")
	}
	for split := 0; split <= len(content); split++ {
		stream := newDirectNanoGPTStream(tools)
		var result []byte
		feed := func(frame []byte) {
			t.Helper()
			out, err := stream.TransformEvent(frame)
			if err != nil {
				t.Fatalf("split %d: %v", split, err)
			}
			result = append(result, out...)
		}
		feed(nanoGPTFrame(nanoGPTChoice(0, map[string]any{"role": "assistant", "reasoning": "supplied thinking", "content": content[:split]}, nil), false))
		// A native call can arrive after complete XML in earlier chunks. Its
		// actual index and identity remain intact; synthesized calls follow it.
		feed(nanoGPTFrame(nanoGPTChoice(0, map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "native-id", "type": "function", "function": map[string]string{"name": "ExistingCase", "arguments": "{}"}}}, "content": content[split:]}, nil), false))
		feed(nanoGPTFrame(nanoGPTChoice(0, map[string]any{}, "stop"), false))
		feed(nanoGPTFrame([]any{}, true))
		feed([]byte("data: [DONE]\n\n"))
		if bytes.Contains(result, []byte("[DONE]")) {
			t.Fatal("terminal was emitted before clean EOF")
		}
		end, err := stream.Finish()
		if err != nil {
			t.Fatal(err)
		}
		result = append(result, end...)
		text, calls := nanoGPTParseOutputTools(t, result)
		if text != "before\n\n```xml\n<Read/>\n```\nafter" || len(calls) != 3 {
			t.Fatalf("split=%d text=%q calls=%v", split, text, calls)
		}
		for i, call := range calls {
			if nanoGPTString(call["id"]) != want.Choices[0].Message.Calls[i].ID {
				t.Fatalf("JSON/SSE tool identity differs at split=%d call=%d: %s", split, i, result)
			}
			if index, _ := nanoGPTIndex(call["index"]); index != i {
				t.Fatalf("tool index collision: %s", result)
			}
		}
		if bytes.Contains(result, []byte(`"reasoning":`)) || !bytes.Contains(result, []byte(`"reasoning_content":"supplied thinking"`)) {
			t.Fatal("reasoning was not normalized")
		}
		analyzer := newIncrementalSseAnalyzer()
		analyzer.Push(result)
		if usage := analyzer.Result().Usage; usage.PromptTokens != 5 || usage.CompletionTokens != 2 || usage.TotalTokens != 7 {
			t.Fatalf("usage changed: %+v", usage)
		}
	}
}

func TestDirectNanoGPTStreamIndependentChoicesAndFailures(t *testing.T) {
	tools := nanoGPTTestTools(t)
	stream := newDirectNanoGPTStream(tools)
	first, err := stream.TransformEvent(nanoGPTFrame([]any{
		map[string]any{"index": 2, "delta": map[string]string{"content": "<Read path=\"same\"/>"}},
		map[string]any{"index": 7, "delta": map[string]string{"content": "<Read path=\"same\"/>"}},
	}, false))
	if err != nil {
		t.Fatal(err)
	}
	last, err := stream.TransformEvent(nanoGPTFrame([]any{
		map[string]any{"index": 7, "delta": map[string]string{}, "finish_reason": "stop"},
		map[string]any{"index": 2, "delta": map[string]string{}, "finish_reason": "stop"},
	}, false))
	if err != nil {
		t.Fatal(err)
	}
	_, calls := nanoGPTParseOutputTools(t, append(first, last...))
	if len(calls) != 2 || nanoGPTString(calls[0]["id"]) == nanoGPTString(calls[1]["id"]) {
		t.Fatal("separate choices shared tool identity")
	}
	if _, err := stream.Finish(); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("missing DONE became successful: %v", err)
	}
	for _, bad := range [][]byte{
		[]byte("data: [DONE]\n\n"),
		nanoGPTFrame(nanoGPTChoice(0, map[string]any{"content": "<Read/>"}, "length"), true),
		nanoGPTFrame(nanoGPTChoice(0, map[string]any{"content": "<Read x=\"1\" x=\"2\"/>"}, "stop"), true),
		nanoGPTFrame(nanoGPTChoice(0, map[string]any{"content": "<Read>{\"x\":1,\"x\":2}</Read>"}, "stop"), true),
	} {
		if _, err := newDirectNanoGPTStream(tools).TransformEvent(bad); err == nil {
			t.Fatalf("invalid action/terminal accepted: %s", bad)
		}
	}
	if _, _, err := newNanoGPTText(tools).feed("<Read>"+strings.Repeat("x", directNanoGPTXMLLimit), false); err == nil {
		t.Fatal("XML retention limit was not enforced")
	}
	plain := strings.Repeat("x", directNanoGPTXMLLimit+1)
	if text, calls, err := newNanoGPTText(tools).feed(plain, false); err != nil || text != plain || len(calls) != 0 {
		t.Fatal("large ordinary text was treated as buffered XML")
	}
	text, _, err := newNanoGPTText(tools).feed("first output without a newline", false)
	if err != nil || text != "first output without a newline" {
		t.Fatal("ordinary first output was held for a future newline")
	}
}
