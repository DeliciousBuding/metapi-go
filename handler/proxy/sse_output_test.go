package proxyhandler

import "testing"

func TestGeneratedSseOutput(t *testing.T) {
	for _, data := range []string{
		`{"choices":[{"delta":{"content":"hello"}}]}`,
		`{"choices":[{"delta":{"reasoning_content":"think"}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"function":{"name":"search"}}]}}]}`,
		`{"type":"response.output_text.delta","delta":"hi"}`,
		`{"type":"response.function_call_arguments.delta","delta":"{"}`,
		`{"type":"response.reasoning_summary_text.delta","delta":"thought"}`,
		`{"type":"content_block_delta","delta":{"type":"text_delta","text":"hi"}}`,
		`{"type":"content_block_delta","delta":{"thinking":"thought"}}`,
		`{"type":"content_block_start","content_block":{"type":"tool_use","name":"search"}}`,
		`{"candidates":[{"content":{"parts":[{"text":"hello"}]}}]}`,
		`{"candidates":[{"content":{"parts":[{"functionCall":{"name":"search"}}]}}]}`,
	} {
		if !hasGeneratedSseOutput(SseEvent{Data: data}) {
			t.Errorf("output ignored: %s", data)
		}
	}
	for _, data := range []string{
		``, `[DONE]`, `not-json`, `{"type":"response.created"}`,
		`{"choices":[{"delta":{"role":"assistant","content":""}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"completion_tokens":2}}`,
		`{"type":"message_start","message":{"role":"assistant"}}`,
		`{"type":"content_block_start","content_block":{"type":"text","text":""}}`,
		`{"type":"error","error":{"message":"oops"}}`,
		`{"candidates":[{"content":{"parts":[{"text":""}]}}]}`,
	} {
		if hasGeneratedSseOutput(SseEvent{Data: data}) {
			t.Errorf("metadata counted as output: %s", data)
		}
	}
}

func TestFirstOutputObserverHandlesSplitEventsOnce(t *testing.T) {
	a := newIncrementalSseAnalyzer()
	calls := 0
	a.onFirstOutput = func() { calls++ }
	a.Push([]byte(": ping\n\ndata: {\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\n"))
	a.Push([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\r\n"))
	if calls != 0 {
		t.Fatal("observed metadata or incomplete event")
	}
	a.Push([]byte("\r\n"))
	a.Push([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"again\"}}]}\n\n"))
	if calls != 1 {
		t.Fatalf("callbacks=%d, want 1", calls)
	}
}
