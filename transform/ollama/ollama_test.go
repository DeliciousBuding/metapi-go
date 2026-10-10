package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestFromChatRequestNativeValuesAndToolRoundTrip(t *testing.T) {
	body := []byte(`{"model":"qwen3","messages":[{"role":"developer","content":"system"},{"role":"user","content":[{"type":"text","text":"look"},{"type":"image_url","image_url":{"url":"data:image/png;base64,aGVsbG8=","detail":"auto"}},{"type":"text","text":" here"}]},{"role":"assistant","content":null,"reasoning_content":"thinking","tool_calls":[{"id":"call_a","type":"function","function":{"name":"lookup","arguments":"{\"id\":9007199254740993,\"fraction\":0.1234567890123456789}"}}]},{"role":"tool","tool_call_id":"call_a","content":"result"}],"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object","properties":{"id":{"const":9007199254740993}}},"strict":false}}],"temperature":0.1234567890123456789,"max_tokens":42,"max_completion_tokens":42,"stop":"END","seed":9007199254740993,"reasoning_effort":"high","response_format":{"type":"json_schema","json_schema":{"name":"out","schema":{"type":"object","properties":{"n":{"const":9007199254740993}}}}},"stream_options":{"include_usage":true}}`)
	got, err := FromChatRequest(body)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"stream":false`, `"role":"system"`, `"content":"look here"`, `"images":["aGVsbG8="]`, `"thinking":"thinking"`, `"tool_name":"lookup"`, `"num_predict":42`, `"stop":["END"]`, `"seed":9007199254740993`, `"fraction":0.1234567890123456789`, `"think":"high"`, `"format":{"type":"object"`} {
		if !bytes.Contains(got, []byte(want)) {
			t.Fatalf("missing %s in %s", want, got)
		}
	}
	if bytes.Contains(got, []byte(`"tool_call_id"`)) || bytes.Contains(got, []byte(`"strict"`)) {
		t.Fatalf("Chat fields leaked: %s", got)
	}
}

func TestFromChatRequestRejectsUnrepresentableFields(t *testing.T) {
	for name, extra := range map[string]string{
		"continuity": `,"previous_response_id":"resp_1"`, "unknown": `,"unknown":1`, "multiple": `,"n":2`,
		"stored":      `,"store":true`,
		"forced tool": `,"tool_choice":"required"`, "serial tools": `,"parallel_tool_calls":false`,
		"bad stream": `,"stream":null`, "stream field": `,"stream_options":{"include_obfuscation":true}`,
		"strict tool":  `,"tools":[{"type":"function","function":{"name":"x","parameters":{},"strict":true}}]`,
		"unknown tool": `,"tools":[{"type":"web_search"}]`, "conflicting max": `,"max_tokens":2,"max_completion_tokens":3`,
		"conflicting options": `,"top_p":0.5,"options":{"top_p":0.1}`, "conflicting think": `,"think":true,"reasoning_effort":"high"`,
		"string temperature": `,"temperature":"1.0"`, "unknown schema": `,"response_format":{"type":"yaml"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if got, err := FromChatRequest([]byte(`{"model":"qwen3","messages":[{"role":"user","content":"hi"}]` + extra + `}`)); err == nil {
				t.Fatalf("accepted unsupported request: %s", got)
			}
		})
	}
	for _, message := range []string{
		`{"role":"user","content":[{"type":"input_audio","input_audio":{"data":"aA=="}}]}`,
		`{"role":"user","content":[{"type":"image_url","image_url":{"url":"https://example.com/image.png"}}]}`,
		`{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,%%%"}}]}`,
		`{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,aA==","detail":"high"}}]}`,
		`{"role":"tool","tool_call_id":"missing","content":"x"}`,
		`{"role":"assistant","content":"x","refusal":"refused"}`,
		`{"role":"assistant","thinking":"x","reasoning_content":"y"}`,
		`{"role":"user","name":"bob","content":"x"}`,
		`{"role":"assistant","tool_calls":[{"id":"a","type":"function","function":{"name":"x","arguments":"[]"}}]}`,
	} {
		if got, err := FromChatRequest([]byte(`{"model":"qwen3","messages":[` + message + `]}`)); err == nil {
			t.Fatalf("accepted %s: %s", message, got)
		}
	}
}

func TestFromChatRequestNoneAndNativeOptions(t *testing.T) {
	got, err := FromChatRequest([]byte(`{"model":"qwen3","messages":[{"role":"user","content":"hi"}],"tools":[{"type":"function","function":{"name":"x"}}],"tool_choice":"none","options":{"num_ctx":8192,"future_native_option":9007199254740993},"keep_alive":"5m","think":false,"response_format":{"type":"json_object"},"store":false}`))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(got, []byte(`"tools"`)) || !bytes.Contains(got, []byte(`"future_native_option":9007199254740993`)) || !bytes.Contains(got, []byte(`"format":"json"`)) {
		t.Fatalf("wrong conversion: %s", got)
	}
}

const terminal = `{"model":"qwen3","created_at":"2026-01-01T00:00:00Z","message":{"role":"assistant","content":"answer","thinking":"reason"},"done":true,"done_reason":"stop","prompt_eval_count":8,"eval_count":3,"total_duration":9007199254740993,"load_duration":17,"prompt_eval_duration":18,"eval_duration":19}`

func TestToChatResponseAndTools(t *testing.T) {
	got, err := ToChatResponse([]byte(terminal))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"object":"chat.completion"`, `"content":"answer"`, `"reasoning_content":"reason"`, `"finish_reason":"stop"`, `"total_tokens":11`, `"total_duration":9007199254740993`, `"created":1767225600`} {
		if !bytes.Contains(got, []byte(want)) {
			t.Fatalf("missing %s in %s", want, got)
		}
	}
	native := `{"model":"qwen3","done":true,"message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"lookup","description":"lookup record","arguments":{"id":9007199254740993,"fraction":0.1234567890123456789}}}]}}`
	got, err = ToChatResponse([]byte(native))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got, []byte(`"finish_reason":"tool_calls"`)) || !bytes.Contains(got, []byte(`9007199254740993`)) || !bytes.Contains(got, []byte(`0.1234567890123456789`)) {
		t.Fatalf("invalid tool response: %s", got)
	}
	var response struct {
		Choices []struct {
			Message json.RawMessage `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(got, &response); err != nil {
		t.Fatal(err)
	}
	var msg object
	if err := json.Unmarshal(response.Choices[0].Message, &msg); err != nil {
		t.Fatal(err)
	}
	var calls []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(msg["tool_calls"], &calls); err != nil {
		t.Fatal(err)
	}
	continued := append([]byte(`{"model":"qwen3","messages":[`), response.Choices[0].Message...)
	continued = append(continued, []byte(`,{"role":"tool","tool_call_id":"`+calls[0].ID+`","content":"found"}]}`)...)
	got, err = FromChatRequest(continued)
	if err != nil || !bytes.Contains(got, []byte(`"tool_name":"lookup"`)) || !bytes.Contains(got, []byte(`"description":"lookup record"`)) {
		t.Fatalf("tool continuation failed: %s %v", got, err)
	}
}

func TestResponseNeverFabricatesSuccess(t *testing.T) {
	for _, body := range []string{
		`{"model":"qwen3","message":{"content":"partial"},"done":false}`,
		`{"model":"qwen3","message":{"content":"partial"}}`,
		`{"error":"model missing"}`, `{"model":"qwen3","done":null}`,
		`{"model":"qwen3","done":true,"done_reason":"unload"}`,
		`{"model":"qwen3","done":true,"eval_count":-1}`,
		`{"model":"qwen3","done":true,"prompt_eval_count":9223372036854775807,"eval_count":1}`,
		`{"model":"qwen3","done":true,"unknown_semantics":true}`,
		`{"model":"qwen3","done":true,"message":{"images":["aA=="]}}`,
		`{"model":"qwen3","done":true,"message":{"tool_calls":[{"function":{"name":"x","arguments":"partial"}}]}}`,
	} {
		if got, err := ToChatResponse([]byte(body)); err == nil {
			t.Fatalf("accepted invalid response %s: %s", body, got)
		}
	}
}

func TestStreamDefersFinishUntilDoneAndPreservesDistinctTools(t *testing.T) {
	s := NewStream("qwen3")
	first, err := s.TransformEvent([]byte(`{"model":"qwen3","done":false,"message":{"thinking":"reason","content":"","tool_calls":[{"function":{"name":"a","arguments":{"id":9007199254740993}}}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(first, []byte(`"finish_reason":null`)) || bytes.Contains(first, []byte("[DONE]")) {
		t.Fatalf("premature finish: %s", first)
	}
	second, err := s.TransformEvent([]byte(`{"model":"qwen3","done":false,"message":{"tool_calls":[{"function":{"name":"a","arguments":{"id":2}}}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(second, []byte(`"index":1`)) {
		t.Fatalf("tool calls merged: %s", second)
	}
	last, err := s.TransformEvent([]byte(`{"model":"qwen3","done":true,"done_reason":"stop","prompt_eval_count":8,"eval_count":3}`))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(last, []byte(`"finish_reason":"tool_calls"`)) || !bytes.HasSuffix(last, []byte("data: [DONE]\n\n")) {
		t.Fatalf("missing terminal: %s", last)
	}
	if _, err := s.Finish(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.TransformEvent([]byte(terminal)); err == nil {
		t.Fatal("accepted data after terminal")
	}
	if _, err := NewStream("qwen3").Finish(); err == nil {
		t.Fatal("fabricated terminal")
	}
}

func TestStreamModelUsageAndLength(t *testing.T) {
	s := NewStream("alias")
	_, err := s.TransformEvent([]byte(`{"model":"resolved:latest","done":false,"message":{"tool_calls":[{"function":{"name":"lookup","arguments":{}}}]},"prompt_eval_count":7}`))
	if err != nil {
		t.Fatal(err)
	}
	last, err := s.TransformEvent([]byte(`{"model":"resolved:latest","done":true,"done_reason":"length","eval_count":3}`))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(last, []byte(`"finish_reason":"length"`)) || !bytes.Contains(last, []byte(`"total_tokens":10`)) {
		t.Fatalf("lost usage or truncation: %s", last)
	}
	s = NewStream("alias")
	_, err = s.TransformEvent([]byte(`{"model":"resolved:latest","done":false,"message":{"content":"a"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.TransformEvent([]byte(`{"model":"another","done":true}`)); err == nil {
		t.Fatal("accepted changed model without timestamps")
	}
	got, err := ToChatResponse([]byte(`{"model":"qwen3","done":true,"message":{"content":"answer"}}`))
	if err != nil || bytes.Contains(got, []byte(`"usage"`)) {
		t.Fatalf("fabricated usage: %s %v", got, err)
	}
}

func TestNDJSONReaderIsIncrementalAndRequiresTerminal(t *testing.T) {
	reader, writer := io.Pipe()
	r := NewNDJSONReader(context.Background(), reader, "qwen3", StreamOptions{IdleTimeout: time.Second})
	defer r.Close()
	wrote := make(chan error, 1)
	go func() {
		_, err := io.WriteString(writer, "\r\n"+`{"model":"qwen3","done":false,"message":{"content":"first"}}`+"\r\n")
		wrote <- err
	}()
	buf := make([]byte, 4096)
	n, err := r.Read(buf)
	if err != nil || !bytes.Contains(buf[:n], []byte("first")) || bytes.Contains(buf[:n], []byte("[DONE]")) {
		t.Fatalf("not incremental: %s %v", buf[:n], err)
	}
	if err := <-wrote; err != nil {
		t.Fatal(err)
	}
	go func() { _, err := io.WriteString(writer, terminal); _ = writer.Close(); wrote <- err }()
	last, err := io.ReadAll(r)
	if err != nil || !bytes.Contains(last, []byte("[DONE]")) {
		t.Fatalf("missing final line without newline: %s %v", last, err)
	}
	if err := <-wrote; err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{"", "\n", `{"model":"qwen3","done":false,"message":{"content":"partial"}}` + "\n"} {
		r := NewNDJSONReader(context.Background(), io.NopCloser(strings.NewReader(body)), "qwen3", StreamOptions{})
		got, err := io.ReadAll(r)
		if err == nil || bytes.Contains(got, []byte("[DONE]")) {
			t.Fatalf("truncated stream succeeded: %s %v", got, err)
		}
	}
}

func TestNDJSONReaderCancellationIdleAndLimits(t *testing.T) {
	t.Run("cancel blocked read", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		reader, writer := io.Pipe()
		defer writer.Close()
		r := NewNDJSONReader(ctx, reader, "qwen3", StreamOptions{})
		result := make(chan error, 1)
		go func() { _, err := io.ReadAll(r); result <- err }()
		cancel()
		select {
		case err := <-result:
			if !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
		case <-time.After(time.Second):
			t.Fatal("cancel did not unblock Read")
		}
	})
	t.Run("idle blocked read", func(t *testing.T) {
		reader, writer := io.Pipe()
		defer writer.Close()
		r := NewNDJSONReader(context.Background(), reader, "qwen3", StreamOptions{IdleTimeout: 20 * time.Millisecond})
		_, err := io.ReadAll(r)
		if !errors.Is(err, ErrIdleTimeout) {
			t.Fatal(err)
		}
	})
	for name, fixture := range map[string]struct {
		body string
		opts StreamOptions
	}{
		"line":   {strings.Repeat("x", 10000), StreamOptions{MaxLineBytes: 32}},
		"total":  {strings.Repeat("\n", 100), StreamOptions{MaxBytes: 32}},
		"output": {`{"done":true}`, StreamOptions{MaxBytes: 32}},
	} {
		t.Run(name, func(t *testing.T) {
			r := NewNDJSONReader(context.Background(), io.NopCloser(strings.NewReader(fixture.body)), "qwen3", fixture.opts)
			got, err := io.ReadAll(r)
			if err == nil || bytes.Contains(got, []byte("[DONE]")) {
				t.Fatalf("limit not enforced: %s %v", got, err)
			}
		})
	}
}
