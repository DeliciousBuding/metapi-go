package relaykitbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/types"
)

func TestFunctionHistoryIdentity(t *testing.T) {
	history := map[Format]string{
		Chat:      `{"messages":[{"role":"user","content":"run"},{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{\"id\":9007199254740993}"}}]},{"role":"tool","tool_call_id":"call_1","content":"done"}]}`,
		Responses: `{"input":[{"role":"user","content":"run"},{"type":"function_call","call_id":"call_1","name":"lookup","arguments":"{\"id\":9007199254740993}"},{"type":"function_call_output","call_id":"call_1","output":"done"}]}`,
		Messages:  `{"max_tokens":512,"messages":[{"role":"user","content":"run"},{"role":"assistant","content":[{"type":"tool_use","id":"call_1","name":"lookup","input":{"id":9007199254740993}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_1","content":"done"}]}]}`,
		Gemini:    `{"contents":[{"role":"user","parts":[{"text":"run"}]},{"role":"model","parts":[{"functionCall":{"id":"call_1","name":"lookup","args":{"id":9007199254740993}}}]},{"role":"user","parts":[{"functionResponse":{"id":"call_1","name":"lookup","response":{"result":"done"}}}]}]}`,
	}
	for _, from := range formats {
		for _, to := range formats {
			t.Run(string(from)+"_"+string(to), func(t *testing.T) {
				_, out, err := ConvertRequest(context.Background(), from, to, "fixture-model", []byte(history[from]))
				if err != nil {
					t.Fatal(err)
				}
				for _, marker := range []string{"call_1", "lookup", "9007199254740993", "done"} {
					if !bytes.Contains(out, []byte(marker)) {
						t.Fatalf("history lost %s: %s", marker, out)
					}
				}
			})
		}
	}
}

func TestLossyRequestFeaturesChooseSpecialized(t *testing.T) {
	cases := []struct {
		from, to Format
		raw      string
	}{
		{Gemini, Chat, `{"contents":[{"role":"user","parts":[{"text":"run"}]}],"toolConfig":{"functionCallingConfig":{"mode":"NONE"}}}`},
		{Messages, Chat, `{"messages":[{"role":"user","content":"run"}],"tool_choice":{"type":"none"}}`},
		{Chat, Gemini, `{"messages":[{"role":"user","content":"run"}],"parallel_tool_calls":false}`},
		{Chat, Gemini, `{"messages":[{"role":"user","content":"run"}],"tools":[{"type":"function","function":{"name":"f","parameters":{"type":"object","additionalProperties":false}}}]}`},
		{Chat, Gemini, `{"messages":[{"role":"user","content":"run"}],"tools":[{"type":"function","function":{"name":"f","parameters":{"type":"object","properties":{"x":{"type":["string","integer"]}}}}}]}`},
		{Gemini, Chat, `{"contents":[{"role":"user","parts":[{"text":"run"}]}],"tools":[{"functionDeclarations":[{"name":"f","parametersJsonSchema":{"type":"object","additionalProperties":false}}]}]}`},
		{Gemini, Chat, `{"contents":[{"role":"user","parts":[{"text":"run"}]}],"tools":[{"functionDeclarations":[{"name":"f","parameters":{"type":"OBJECT"}}]}]}`},
	}
	for i, tc := range cases {
		_, out, err := ConvertRequest(context.Background(), tc.from, tc.to, "fixture-model", []byte(tc.raw))
		if !errors.Is(err, ErrSpecialized) || len(out) > 0 {
			t.Fatalf("case %d silently converted %s: %v", i, out, err)
		}
	}
}

func TestToolSchemaPrecision(t *testing.T) {
	raw := []byte(`{"messages":[{"role":"user","content":"run"}],"tools":[{"type":"function","function":{"name":"f","parameters":{"type":"object","properties":{"x":{"type":"integer","minimum":9007199254740993}}}}}]}`)
	for _, to := range []Format{Responses, Messages, Gemini} {
		_, out, err := ConvertRequest(context.Background(), Chat, to, "fixture-model", raw)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(out, []byte(`9007199254740993`)) {
			t.Fatalf("schema constraint rounded: %s", out)
		}
	}
}

func TestDiagnosticsFailClosed(t *testing.T) {
	jsonBody := []byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"result"}]},"groundingMetadata":{"webSearchQueries":["private code"]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1}}`)
	s := newTestSession(t, Chat, Gemini)
	out, err := s.Response(context.Background(), jsonBody)
	if err == nil || len(out) > 0 || errors.Is(err, ErrSpecialized) {
		t.Fatalf("response diagnostic escaped: %s %v", out, err)
	}
	if strings.Contains(err.Error(), "private code") {
		t.Fatal("diagnostic leaked content")
	}
	stream, _ := newTestSession(t, Chat, Gemini).NewResponseStream()
	out, err = stream.TransformEvent(frame(string(jsonBody)))
	if err == nil || len(out) > 0 || errors.Is(err, ErrSpecialized) {
		t.Fatalf("stream diagnostic escaped: %s %v", out, err)
	}
	if out, err = stream.Finish(); err == nil || len(out) > 0 {
		t.Fatal("diagnostic failure finalized successfully")
	}
}

func TestMalformedToolStreams(t *testing.T) {
	for _, upstream := range []Format{Chat, Messages, Responses} {
		t.Run(string(upstream), func(t *testing.T) {
			client := Chat
			if upstream == Chat {
				client = Responses
			}
			stream, _ := newTestSession(t, client, upstream).NewResponseStream()
			var failed bool
			for _, event := range toolStreams[upstream] {
				event = strings.ReplaceAll(event, `9007199254740993}`, `9007199254740993`)
				out, err := stream.TransformEvent(frame(event))
				if err != nil {
					if len(out) > 0 {
						t.Fatal("failure emitted bytes")
					}
					failed = true
					break
				}
			}
			if !failed {
				t.Fatal("incomplete function arguments accepted")
			}
			if out, err := stream.Finish(); err == nil || len(out) > 0 {
				t.Fatal("incomplete tools finalized successfully")
			}
		})
	}
}

func TestWireFramingAndTerminalFailures(t *testing.T) {
	for name, events := range map[string][][]byte{
		"done_without_reason": {frame(`[DONE]`)},
		"invalid_json":        {frame(`{"choices":`)},
		"unknown_fields":      {[]byte("not-sse\n\n")},
		"multiple_events":     {append(frame(streams[Chat][0]), frame(streams[Chat][1])...)},
		"error_event":         {[]byte("event: error\ndata: {\"message\":\"private\"}\n\n")},
		"changed_choice":      {frame(strings.Replace(streams[Chat][0], `"index":0`, `"index":1`, 1))},
	} {
		t.Run(name, func(t *testing.T) {
			stream, _ := newTestSession(t, Responses, Chat).NewResponseStream()
			for _, event := range events {
				out, err := stream.TransformEvent(event)
				if err == nil || len(out) > 0 {
					t.Fatalf("bad frame accepted: %s %v", out, err)
				}
			}
		})
	}
	stream, _ := newTestSession(t, Chat, Messages).NewResponseStream()
	if _, err := stream.TransformEvent([]byte("event: content_block_stop\ndata: {\"type\":\"message_start\"}\n\n")); err == nil {
		t.Fatal("event/data mismatch accepted")
	}
}

func TestStreamLimits(t *testing.T) {
	stream, _ := newTestSession(t, Responses, Chat).NewResponseStream()
	if out, err := stream.TransformEvent(bytes.Repeat([]byte("x"), maxFrameBytes+1)); err == nil || len(out) > 0 {
		t.Fatal("frame limit not enforced")
	}
	stream, _ = newTestSession(t, Responses, Chat).NewResponseStream()
	stream.inputBytes = maxStreamBytes
	if out, err := stream.TransformEvent(frame(streams[Chat][0])); err == nil || len(out) > 0 {
		t.Fatal("stream limit not enforced")
	}
}

func TestConcurrentSessionsDoNotShareState(t *testing.T) {
	var wg sync.WaitGroup
	for n := 0; n < 16; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, _, err := ConvertRequest(context.Background(), Responses, Messages, "fixture-model", []byte(requests[Responses]))
			if err != nil {
				t.Error(err)
				return
			}
			stream, err := s.NewResponseStream()
			if err != nil {
				t.Error(err)
				return
			}
			for _, raw := range toolStreams[Messages] {
				if _, err := stream.TransformEvent(frame(raw)); err != nil {
					t.Error(err)
					return
				}
			}
			if _, err := stream.Finish(); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
}

func TestUserArgumentsAreNotProtocolMarkers(t *testing.T) {
	raw := []byte(`{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"c1","type":"function","function":{"name":"f","arguments":"{\"signature\":\"customer field\",\"image_url\":\"customer value\"}"}}]},"finish_reason":"tool_calls"}]}`)
	for _, client := range []Format{Messages, Responses, Gemini} {
		out, err := newTestSession(t, client, Chat).Response(context.Background(), raw)
		if err != nil {
			t.Fatal(err)
		}
		if !json.Valid(out) || !bytes.Contains(out, []byte("customer field")) {
			t.Fatalf("tool data lost: %s", out)
		}
	}
}

func TestGeminiTrailingUsageAndReorderedToolSnapshot(t *testing.T) {
	stream, _ := newTestSession(t, Chat, Gemini).NewResponseStream()
	for _, raw := range streams[Gemini] {
		if _, err := stream.TransformEvent(frame(raw)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := stream.TransformEvent(frame(`{"candidates":[],"usageMetadata":{"promptTokenCount":12,"candidatesTokenCount":7,"totalTokenCount":19}}`)); err != nil {
		t.Fatalf("valid trailing usage rejected: %v", err)
	}
	if _, err := stream.Finish(); err != nil {
		t.Fatal(err)
	}
	if !sameJSON([]byte(`{"a":9007199254740993,"b":1}`), []byte(`{"b":1, "a":9007199254740993}`)) {
		t.Fatal("equivalent tool snapshot rejected")
	}
}

func TestInvalidRolesAndMixedGeminiParts(t *testing.T) {
	for _, raw := range []string{
		`{"contents":[{"role":"assistant","parts":[{"text":"x"}]}]}`,
		`{"contents":[{"role":"user","parts":[{"text":"x","functionCall":{"name":"f","args":{}}}]}]}`,
	} {
		_, out, err := ConvertRequest(context.Background(), Gemini, Chat, "fixture-model", []byte(raw))
		if err == nil || len(out) > 0 {
			t.Fatalf("ambiguous request accepted: %s", out)
		}
	}
}

func TestPromptCacheKeyConversation(t *testing.T) {
	const key = "fixture-private-cache-key"
	for _, p := range []struct{ from, to Format }{{Chat, Responses}, {Responses, Chat}} {
		raw := strings.TrimSuffix(requests[p.from], "}") + `,"prompt_cache_key":"` + key + `"}`
		s, out, err := ConvertRequest(t.Context(), p.from, p.to, "fixture-model", []byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		obj, err := readObject(out)
		if err != nil || text(obj["prompt_cache_key"]) != key {
			t.Fatal("cache key lost")
		}
		body, err := s.Response(t.Context(), []byte(replies[p.to]))
		if err != nil || !bytes.Contains(body, []byte("hello")) || bytes.Contains(body, []byte(key)) {
			t.Fatalf("response = %s %v", body, err)
		}
		stream, err := s.NewResponseStream()
		if err != nil {
			t.Fatal(err)
		}
		var events []byte
		for _, chunk := range streams[p.to] {
			out, err := stream.TransformEvent(frame(chunk))
			if err != nil {
				t.Fatal(err)
			}
			events = append(events, out...)
		}
		if p.to == Chat {
			if _, err = stream.TransformEvent(frame("[DONE]")); err != nil {
				t.Fatal(err)
			}
		}
		tail, err := stream.Finish()
		events = append(events, tail...)
		if err != nil || !bytes.Contains(events, []byte("hello")) || bytes.Contains(events, []byte(key)) {
			t.Fatal("invalid stream", err)
		}
	}
}

func TestRelayKitStrictDiagnosticPrivacy(t *testing.T) {
	const key = "private-cache-key"
	_, body, err := ConvertRequest(t.Context(), Chat, Responses, "m", []byte(`{"messages":[{"role":"user","content":"private-text"}],"tool_choice":{"type":"function","function":{"name":"private-name"}},"prompt_cache_key":"`+key+`"}`))
	var diagnostic interface{ DiagnosticsSummary() []Diagnostic }
	if len(body) != 0 || !errors.Is(err, ErrConversion) || !errors.As(err, &diagnostic) {
		t.Fatalf("strict conversion: err=%v", err)
	}
	ds := diagnostic.DiagnosticsSummary()
	if len(ds) != 1 || ds[0] != (Diagnostic{"request", "tool_choice_without_tools", "warning", "tool_choice"}) {
		t.Fatalf("unexpected diagnostic: %+v", ds)
	}
	if strings.Contains(fmt.Sprint(err, ds), "private-") {
		t.Fatal("private data in error")
	}
	s := newTestSession(t, Chat, Gemini)
	steps := s.Steps()
	if s.Quality() == "" || len(steps) == 0 {
		t.Fatal("missing request trace")
	}
	steps[0].Converter = "overwritten"
	if s.Steps()[0].Converter == steps[0].Converter {
		t.Fatal("mutable request trace")
	}
	// Unknown library fields must remain safe even when the loss error is wrapped.
	e := fmt.Errorf("private-error: %w", &types.ConversionLossError{Diagnostics: []types.ConversionDiagnostic{{Path: "tools[private-name].private-param", Code: key, Message: "private-text", Severity: "private-severity"}}})
	err = s.conversionError("stream", e, nil)
	ds = s.DiagnosticsSummary()
	if len(ds) != 1 || ds[0] != (Diagnostic{"stream", "unknown", "unknown", "tools"}) || strings.Contains(fmt.Sprintf("%+v %#v", err, ds), "private-") {
		t.Fatalf("summary=%v error=%v", ds, err)
	}
	ds[0].Code = "overwritten"
	if s.DiagnosticsSummary()[0].Code == ds[0].Code {
		t.Fatal("mutable diagnostics")
	}
}

func TestPromptCacheKeyEligibility(t *testing.T) {
	for _, p := range []struct{ from, to Format }{{Chat, Responses}, {Responses, Chat}, {Chat, Messages}, {Responses, Gemini}} {
		for _, v := range []string{`null`, `123`, `["private"]`, `{"private":1}`, `""`, `"private"`} {
			raw := strings.TrimSuffix(requests[p.from], "}") + `,"prompt_cache_key":` + v + `}`
			_, out, err := ConvertRequest(t.Context(), p.from, p.to, "m", []byte(raw))
			if (p.to == Chat || p.to == Responses) && v[0] == '"' {
				obj, e := readObject(out)
				if err != nil || e != nil || string(obj["prompt_cache_key"]) != v {
					t.Fatalf("%s -> %s: key %s lost, %v", p.from, p.to, v, err)
				}
			} else if !errors.Is(err, ErrSpecialized) || len(out) > 0 || strings.Contains(err.Error(), "private") {
				t.Fatalf("unexpected admission: %v", err)
			}
		}
	}
}

func TestParallelFunctionHistoryRoundTrip(t *testing.T) {
	const raw = `{"model":"fixture-model","messages":[{"role":"user","content":"run"},{"role":"assistant","tool_calls":[{"id":"a","type":"function","function":{"name":"lookup","arguments":"{\"id\":9223372036854775807}"}},{"id":"b","type":"function","function":{"name":"lookup","arguments":"{\"id\":-9223372036854775808}"}}]},{"role":"tool","tool_call_id":"b","content":"second"},{"role":"tool","tool_call_id":"a","content":"first"}]}`
	_, res, err := ConvertRequest(t.Context(), Chat, Responses, "m", []byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		Input []struct {
			Type              string
			CallID            string `json:"call_id"`
			Arguments, Output string
		}
	}
	if err = json.Unmarshal(res, &response); err != nil {
		t.Fatal(err)
	}
	args := map[string]string{}
	outputs := map[string]string{}
	for _, item := range response.Input {
		switch item.Type {
		case "function_call":
			args[item.CallID] = item.Arguments
		case "function_call_output":
			outputs[item.CallID] = item.Output
		}
	}
	wantArgs := map[string]string{"a": `{"id":9223372036854775807}`, "b": `{"id":-9223372036854775808}`}
	wantOutputs := map[string]string{"a": "first", "b": "second"}
	if !maps.Equal(args, wantArgs) || !maps.Equal(outputs, wantOutputs) {
		t.Fatalf("Responses tool pairing: %s", res)
	}
	_, chat, err := ConvertRequest(t.Context(), Responses, Chat, "m", res)
	if err != nil {
		t.Fatal(err)
	}
	var request struct {
		Messages []struct {
			Role, Content string
			ToolCallID    string `json:"tool_call_id"`
			Calls         []struct {
				ID       string
				Function struct{ Arguments string }
			} `json:"tool_calls"`
		}
	}
	if err = json.Unmarshal(chat, &request); err != nil {
		t.Fatal(err)
	}
	clear(args)
	clear(outputs)
	for _, msg := range request.Messages {
		for _, call := range msg.Calls {
			args[call.ID] = call.Function.Arguments
		}
		if msg.Role == "tool" {
			outputs[msg.ToolCallID] = msg.Content
		}
	}
	if !maps.Equal(args, wantArgs) || !maps.Equal(outputs, wantOutputs) {
		t.Fatalf("Chat tool pairing: %s", chat)
	}
}

func TestResponsesFailureEventsForEachUpstream(t *testing.T) {
	for _, up := range []Format{Chat, Messages, Gemini} {
		t.Run(string(up), func(t *testing.T) {
			s, err := newTestSession(t, Responses, up).NewResponseStream()
			if err != nil {
				t.Fatal(err)
			}
			for _, data := range streams[up][:1] {
				if _, err := s.TransformEvent(frame(data)); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.TransformEvent(frame(`{"error":{"message":"private-data"}}`)); !errors.Is(err, ErrConversion) {
				t.Fatal("upstream error accepted")
			}
			out := s.ResponsesFailureEvents("private-message")
			if !bytes.Contains(out, []byte("event: response.failed\n")) || bytes.Contains(out, []byte("private-")) || bytes.Contains(out, []byte("[DONE]")) {
				t.Fatalf("invalid failure output: %s", out)
			}
		})
	}
	s, err := newTestSession(t, Chat, Responses).NewResponseStream()
	if err != nil {
		t.Fatal(err)
	}
	if got := s.ResponsesFailureEvents("ignored"); got != nil {
		t.Fatal("Responses failure emitted to Chat")
	}
}
