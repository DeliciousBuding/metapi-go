package messages_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/transform/anthropic/messages"
)

func hiddenToolReply(t *testing.T, reasoning string) []byte {
	t.Helper()
	reply := jsonObject(t, decodeJSON(t, []byte(chatToolReply)))
	choice := jsonObject(t, jsonArray(t, reply["choices"])[0])
	message := jsonObject(t, choice["message"])
	message["reasoning_content"] = reasoning
	calls := jsonArray(t, message["tool_calls"])
	message["tool_calls"] = append(calls, map[string]any{"id": "call-2", "type": "function", "function": map[string]any{"name": "Read", "arguments": `{"file_path":"other.txt"}`}})
	return marshalJSON(t, reply)
}

func reasoningContinuation(t *testing.T, reply []byte) map[string]any {
	t.Helper()
	req := readRequest(t)
	message := jsonObject(t, decodeJSON(t, reply))
	var results []any
	for _, item := range jsonArray(t, message["content"]) {
		block := jsonObject(t, item)
		if block["type"] == "tool_use" {
			results = append(results, map[string]any{"type": "tool_result", "tool_use_id": block["id"], "content": "synthetic file contents"})
		}
	}
	req["messages"] = []any{
		map[string]any{"role": "user", "content": "Read both files."},
		map[string]any{"role": "assistant", "content": message["content"]},
		map[string]any{"role": "user", "content": results},
	}
	return req
}

func TestHiddenReasoningSurvivesClaudeToolContinuation(t *testing.T) {
	t.Parallel()
	const hidden = "synthetic hidden reasoning; never a client answer"
	saved := make(map[string]string)
	var saveIDs, loadIDs []string
	options := messages.Options{
		SaveReasoning: func(ids []string, reasoning string) error {
			saveIDs = append([]string(nil), ids...)
			saved[strings.Join(ids, "\x00")] = reasoning
			return nil
		},
		LoadReasoning: func(ids []string) (string, bool, error) {
			loadIDs = append([]string(nil), ids...)
			reasoning, ok := saved[strings.Join(ids, "\x00")]
			return reasoning, ok, nil
		},
	}
	reply, err := messages.FromChatResponse(hiddenToolReply(t, hidden), options)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(reply), hidden) || strings.Contains(string(reply), "reasoning_content") || strings.Contains(string(reply), `"thinking"`) {
		t.Fatalf("omitted thoughts were exposed to the native client: %s", reply)
	}
	if !reflect.DeepEqual(saveIDs, []string{"call-1", "call-2"}) {
		t.Fatalf("save did not capture the full parallel group: %v", saveIDs)
	}
	req := reasoningContinuation(t, reply)
	out, err := messages.ToChatRequest(marshalJSON(t, req), options)
	if err != nil {
		t.Fatal(err)
	}
	chat := jsonObject(t, decodeJSON(t, out))
	if chat["reasoning_effort"] != "high" || !reflect.DeepEqual(saveIDs, loadIDs) {
		t.Fatalf("reasoning controls/group lost: %s; save %v load %v", out, saveIDs, loadIDs)
	}
	transcript := jsonArray(t, chat["messages"])
	found := false
	for _, item := range transcript {
		message := jsonObject(t, item)
		if message["role"] == "assistant" {
			found = true
			if message["reasoning_content"] != hidden || len(jsonArray(t, message["tool_calls"])) != 2 {
				t.Fatalf("tool continuation dropped hidden reasoning: %s", out)
			}
		} else if _, ok := message["reasoning_content"]; ok {
			t.Fatal("reasoning attached to the wrong role")
		}
	}
	if !found {
		t.Fatal("assistant continuation missing")
	}
}

func TestAdaptiveToolHistoryRequiresKnownReplay(t *testing.T) {
	t.Parallel()
	reply, err := messages.FromChatResponse([]byte(chatToolReply))
	if err != nil {
		t.Fatal(err)
	}
	req := reasoningContinuation(t, reply)
	for _, options := range []messages.Options{
		{},
		{LoadReasoning: func([]string) (string, bool, error) { return "", false, nil }},
		{LoadReasoning: func([]string) (string, bool, error) { return "partial cached data", false, nil }},
		{LoadReasoning: func([]string) (string, bool, error) { return "", false, errors.New("SHOULD_NOT_LEAK") }},
	} {
		out, err := messages.ToChatRequest(marshalJSON(t, req), options)
		if !errors.Is(err, messages.ErrReasoningReplay) || len(out) != 0 {
			t.Fatalf("missing/failed replay was ignored: %s, %v", out, err)
		}
		if strings.Contains(err.Error(), "SHOULD_NOT_LEAK") {
			t.Fatal("lookup internals leaked")
		}
	}
	out, err := messages.ToChatRequest(marshalJSON(t, req), messages.Options{LoadReasoning: func([]string) (string, bool, error) { return "", true, nil }})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "reasoning_content") {
		t.Fatalf("known no-reasoning response invented a reasoning field: %s", out)
	}
}

func TestToolReasoningCannotBeDiscardedWithoutCapture(t *testing.T) {
	t.Parallel()
	for _, options := range []messages.Options{
		{},
		{SaveReasoning: func([]string, string) error { return errors.New("SHOULD_NOT_LEAK") }},
	} {
		out, err := messages.FromChatResponse(hiddenToolReply(t, "hidden"), options)
		if !errors.Is(err, messages.ErrReasoningReplay) || len(out) != 0 {
			t.Fatalf("unreplayable tool response succeeded: %s, %v", out, err)
		}
		if strings.Contains(err.Error(), "SHOULD_NOT_LEAK") {
			t.Fatal("save internals leaked")
		}
	}
}

func TestChatStreamHiddenReasoningSavedOnlyAtValidTerminal(t *testing.T) {
	t.Parallel()
	var captured string
	var capturedIDs []string
	calls := 0
	stream := messages.NewChatStream("m", messages.Options{SaveReasoning: func(ids []string, reasoning string) error {
		calls++
		capturedIDs = append([]string(nil), ids...)
		captured = reasoning
		return nil
	}})
	consumer := &nativeConsumer{}
	for _, frame := range []string{
		chatChunk(t, map[string]any{"reasoning_content": "hidden "}, nil, nil),
		toolChunk(t, toolDelta(7, "", "", "{")),
		chatChunk(t, map[string]any{"reasoning_content": "continuation", "tool_calls": []any{toolDelta(19, "b", "Read", "{}")}}, nil, nil),
		toolChunk(t, toolDelta(7, "a", "Read", "}")),
		chatChunk(t, map[string]any{}, "tool_calls", nil),
	} {
		out, err := stream.TransformEvent([]byte(frame))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(out), "hidden") || strings.Contains(string(out), "continuation") || calls != 0 {
			t.Fatalf("hidden/partial reasoning leaked or was saved too early: %s, calls=%d", out, calls)
		}
		consumer.consume(t, out)
	}
	out, err := stream.TransformEvent([]byte("data: [DONE]\n\n"))
	if err != nil {
		t.Fatal(err)
	}
	consumer.consume(t, out)
	if captured != "hidden continuation" || calls != 1 || !reflect.DeepEqual(capturedIDs, []string{"b", "a"}) {
		t.Fatalf("incorrect complete reasoning/group: %q, %v, saves=%d", captured, capturedIDs, calls)
	}
	if _, err := stream.Finish(); err != nil || calls != 1 {
		t.Fatalf("Finish recaptured reasoning: %v; saves=%d", err, calls)
	}
	// Native output order, not sparse upstream-index encounter order, keys the
	// next assistant message's replay record.
	req := reasoningContinuation(t, marshalJSON(t, consumer.message))
	_, err = messages.ToChatRequest(marshalJSON(t, req), messages.Options{LoadReasoning: func(ids []string) (string, bool, error) {
		return captured, reflect.DeepEqual(ids, capturedIDs), nil
	}})
	if err != nil {
		t.Fatal(err)
	}
}

func TestChatStreamNeverCapturesFailedReasoning(t *testing.T) {
	t.Parallel()
	for _, ending := range []string{"EOF", "error", "bad arguments"} {
		t.Run(ending, func(t *testing.T) {
			saves := 0
			stream := messages.NewChatStream("m", messages.Options{SaveReasoning: func([]string, string) error { saves++; return nil }})
			arguments := "{}"
			if ending == "bad arguments" {
				arguments = "{"
			}
			for _, frame := range []string{
				chatChunk(t, map[string]any{"reasoning_content": "hidden"}, nil, nil),
				toolChunk(t, toolDelta(0, "a", "Read", arguments)),
				chatChunk(t, map[string]any{}, "tool_calls", nil),
			} {
				if _, err := stream.TransformEvent([]byte(frame)); err != nil {
					t.Fatal(err)
				}
			}
			if ending == "error" {
				if _, err := stream.TransformEvent([]byte("event: error\ndata: {}\n\n")); err == nil {
					t.Fatal("upstream error ignored")
				}
			}
			if ending == "bad arguments" {
				if _, err := stream.TransformEvent([]byte("data: [DONE]\n\n")); err == nil {
					t.Fatal("bad tool JSON ignored")
				}
			}
			if _, err := stream.Finish(); err == nil || saves != 0 {
				t.Fatalf("failed stream saved reasoning: %v, saves=%d", err, saves)
			}
		})
	}
}

func TestChatStreamMissingCaptureFailsWithoutNativeSuccess(t *testing.T) {
	t.Parallel()
	for _, options := range []messages.Options{{}, {SaveReasoning: func([]string, string) error { return errors.New("storage unavailable") }}} {
		stream := messages.NewChatStream("m", options)
		for _, frame := range []string{
			chatChunk(t, map[string]any{"reasoning_content": "hidden"}, nil, nil),
			toolChunk(t, toolDelta(0, "a", "Read", "{}")),
			chatChunk(t, map[string]any{}, "tool_calls", nil),
		} {
			if _, err := stream.TransformEvent([]byte(frame)); err != nil {
				t.Fatal(err)
			}
		}
		out, err := stream.TransformEvent([]byte("data: [DONE]\n\n"))
		if !errors.Is(err, messages.ErrReasoningReplay) || len(out) != 0 {
			t.Fatalf("unreplayable stream emitted native success: %s, %v", out, err)
		}
		if _, err := stream.Finish(); !errors.Is(err, messages.ErrReasoningReplay) {
			t.Fatalf("capture error was not sticky: %v", err)
		}
	}
}

func TestCaptureRecordsKnownEmptyReasoning(t *testing.T) {
	t.Parallel()
	calls := 0
	options := messages.Options{SaveReasoning: func(ids []string, reasoning string) error {
		calls++
		if reasoning != "" || len(ids) != 1 {
			t.Fatalf("incorrect no-reasoning record: %v, %q", ids, reasoning)
		}
		return nil
	}}
	if _, err := messages.FromChatResponse([]byte(chatToolReply), options); err != nil {
		t.Fatal(err)
	}
	runStream(t, messages.NewChatStream("m", options), toolChunk(t, toolDelta(0, "a", "Read", "{}")), chatChunk(t, map[string]any{}, "tool_calls", nil), "data: [DONE]\n\n")
	if calls != 2 {
		t.Fatalf("no-reasoning tool responses were not recorded: %d", calls)
	}
}
