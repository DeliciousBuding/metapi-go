package messages_test

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/transform/anthropic/messages"
)

func generatedToolIDs(t *testing.T, response []byte) []string {
	t.Helper()
	var ids []string
	for _, value := range jsonArray(t, jsonObject(t, decodeJSON(t, response))["content"]) {
		block := jsonObject(t, value)
		if block["type"] == "tool_use" {
			ids = append(ids, block["id"].(string))
		}
	}
	return ids
}

func TestNewToolUseIDsAreSavedAndReplayedWithoutRestoringUpstreamIDs(t *testing.T) {
	t.Parallel()
	issued := 0
	var saved []string
	options := messages.Options{
		NewToolUseID:  func() (string, error) { issued++; return fmt.Sprintf("caller-tool-%d", issued), nil },
		SaveReasoning: func(ids []string, reasoning string) error { saved = append([]string(nil), ids...); return nil },
		LoadReasoning: func(ids []string) (string, bool, error) { return "", reflect.DeepEqual(ids, saved), nil },
	}
	reply, err := messages.FromChatResponse(hiddenToolReply(t, ""), options)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"caller-tool-1", "caller-tool-2"}
	if issued != 2 || !reflect.DeepEqual(generatedToolIDs(t, reply), want) || !reflect.DeepEqual(saved, want) {
		t.Fatalf("client/save identities disagree: issued=%d saved=%v", issued, saved)
	}
	req := reasoningContinuation(t, reply)
	converted, err := messages.ToChatRequest(marshalJSON(t, req), options)
	if err != nil {
		t.Fatal(err)
	}
	var calls, results []string
	for _, value := range jsonArray(t, jsonObject(t, decodeJSON(t, converted))["messages"]) {
		message := jsonObject(t, value)
		switch message["role"] {
		case "assistant":
			for _, call := range jsonArray(t, message["tool_calls"]) {
				calls = append(calls, jsonObject(t, call)["id"].(string))
			}
		case "tool":
			results = append(results, message["tool_call_id"].(string))
		}
	}
	if !reflect.DeepEqual(calls, want) || !reflect.DeepEqual(results, want) || issued != 2 {
		t.Fatalf("Chat continuation lost/reallocated caller IDs: calls=%v results=%v", calls, results)
	}
	unchanged, err := messages.FromChatResponse([]byte(chatToolReply))
	if err != nil || !reflect.DeepEqual(generatedToolIDs(t, unchanged), []string{"call-1"}) {
		t.Fatal("default allocator no longer preserves upstream IDs")
	}
}

func TestChatStreamAllocatesEachOutputToolIDOnlyOnce(t *testing.T) {
	t.Parallel()
	issued := 0
	var saved []string
	options := messages.Options{
		NewToolUseID:  func() (string, error) { issued++; return fmt.Sprintf("caller-tool-%d", issued), nil },
		SaveReasoning: func(ids []string, reasoning string) error { saved = append([]string(nil), ids...); return nil },
	}
	stream := messages.NewChatStream("m", options)
	consumer := &nativeConsumer{}
	for i, frame := range []string{
		toolChunk(t, toolDelta(7, "", "", `{"n":`)),
		toolChunk(t, toolDelta(19, "upstream-b", "Read", `{}`)),
		toolChunk(t, toolDelta(7, "upstream-a", "Read", `1}`)),
		toolChunk(t, toolDelta(19, "", "", ""), toolDelta(7, "upstream-a", "", "")),
		chatChunk(t, map[string]any{}, "tool_calls", nil),
		"data: [DONE]\n\n",
	} {
		out, err := stream.TransformEvent([]byte(frame))
		if err != nil {
			t.Fatal(err)
		}
		consumer.consume(t, out)
		if i == 0 && issued != 0 {
			t.Fatal("allocated an ID before the upstream tool identity was available")
		}
	}
	if _, err := stream.Finish(); err != nil {
		t.Fatal(err)
	}
	want := []string{"caller-tool-1", "caller-tool-2"}
	if issued != 2 || !reflect.DeepEqual(saved, want) || !reflect.DeepEqual(generatedToolIDs(t, marshalJSON(t, consumer.message)), want) {
		t.Fatalf("SSE allocated more than once or saved upstream rather than output IDs: %d, %v", issued, saved)
	}
	content := jsonArray(t, consumer.message["content"])
	requireJSON(t, marshalJSON(t, jsonObject(t, content[0])["input"]), `{}`)
	requireJSON(t, marshalJSON(t, jsonObject(t, content[1])["input"]), `{"n":1}`)
}

func TestToolIDAllocatorFailureCannotReleaseSuccessfulOutput(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		allocate func() (string, error)
	}{
		{"error", func() (string, error) { return "", errors.New("SECRET_CALLBACK_DETAIL") }},
		{"empty", func() (string, error) { return "", nil }},
		{"blank", func() (string, error) { return " ", nil }},
		{"invalid UTF8", func() (string, error) { return string([]byte{0xff}), nil }},
		{"duplicate", func() (string, error) { return "same-output-id", nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			saved := 0
			options := messages.Options{NewToolUseID: tc.allocate, SaveReasoning: func([]string, string) error { saved++; return nil }}
			out, err := messages.FromChatResponse(hiddenToolReply(t, ""), options)
			if err == nil || len(out) != 0 || saved != 0 || strings.Contains(err.Error(), "SECRET_CALLBACK_DETAIL") {
				t.Fatal("invalid allocator output was saved/released or leaked its error")
			}
			stream := messages.NewChatStream("m", options)
			out, err = stream.TransformEvent([]byte(toolChunk(t, toolDelta(0, "a", "Read", "{}"), toolDelta(1, "b", "Read", "{}"))))
			if err == nil || len(out) != 0 || saved != 0 {
				t.Fatal("stream accepted invalid/duplicate output IDs")
			}
			if _, err := stream.Finish(); err == nil {
				t.Fatal("Finish revived an allocator failure")
			}
		})
	}
}
