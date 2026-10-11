package relaykitbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func newTestSession(t *testing.T, client, upstream Format) *Session {
	t.Helper()
	s, _, err := ConvertRequest(context.Background(), client, upstream, "fixture-model", []byte(requests[client]))
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func frame(raw string) []byte { return []byte("data: " + raw + "\n\n") }
func TestProtocolMatrix(t *testing.T) {
	for _, client := range formats {
		for _, upstream := range formats {
			t.Run(string(client)+"_to_"+string(upstream), func(t *testing.T) {
				s, body, err := ConvertRequest(context.Background(), client, upstream, "fixture-model", []byte(requests[client]))
				if err != nil {
					t.Fatal(err)
				}
				for _, want := range []string{"hello", "lookup", "integer"} {
					if !bytes.Contains(bytes.ToLower(body), []byte(want)) {
						t.Fatalf("request missing %s: %s", want, body)
					}
				}
				raw, err := s.Response(context.Background(), []byte(replies[upstream]))
				if err != nil {
					t.Fatal(err)
				}
				for _, want := range []string{"hello", "lookup", "9007199254740993"} {
					if !bytes.Contains(raw, []byte(want)) {
						t.Fatalf("response missing %s: %s", want, raw)
					}
				}
				if client != upstream {
					if err := safeResponse(client, raw, false); err != nil {
						t.Fatalf("invalid downstream JSON: %s %v", raw, err)
					}
					if bytes.Contains(raw, []byte("billing_usage")) {
						t.Fatal("internal billing metadata leaked")
					}
				}
				for name, events := range map[string][]string{"text": streams[upstream], "tool": toolStreams[upstream]} {
					t.Run(name, func(t *testing.T) {
						stream, err := newTestSession(t, client, upstream).NewResponseStream()
						if err != nil {
							t.Fatal(err)
						}
						var out []byte
						for _, event := range events {
							chunk, err := stream.TransformEvent(frame(event))
							if err != nil {
								t.Fatalf("%s: %v", event, err)
							}
							out = append(out, chunk...)
						}
						if upstream == Chat {
							chunk, err := stream.TransformEvent(frame("[DONE]"))
							if err != nil {
								t.Fatal(err)
							}
							out = append(out, chunk...)
						}
						tail, err := stream.Finish()
						if err != nil {
							t.Fatal(err)
						}
						out = append(out, tail...)
						want := "hello"
						if name == "tool" {
							want = "lookup"
							if !bytes.Contains(out, []byte("9007199254740993")) {
								t.Fatalf("tool precision lost: %s", out)
							}
						}
						if !bytes.Contains(out, []byte(want)) {
							t.Fatalf("stream content missing %s: %s", want, out)
						}
						if client != upstream && bytes.Contains(out, []byte("billing_usage")) {
							t.Fatal("internal billing metadata leaked")
						}
						if client == Chat && !bytes.HasSuffix(out, frame("[DONE]")) {
							t.Fatalf("missing Chat terminator: %s", out)
						}
						if client == Messages && !bytes.Contains(out, []byte(`"type":"message_stop"`)) {
							t.Fatalf("missing Messages terminator: %s", out)
						}
						if client == Responses && !bytes.Contains(out, []byte(`"type":"response.completed"`)) {
							t.Fatalf("missing Responses terminator: %s", out)
						}
						if client == Gemini && !bytes.Contains(out, []byte(`"finishReason":"STOP"`)) {
							t.Fatalf("missing Gemini terminator: %s", out)
						}
						if chunk, err := stream.Finish(); err != nil || len(chunk) != 0 {
							t.Fatal("Finish must be idempotent")
						}
					})
				}
			})
		}
	}
}

func TestNativeByteIdentity(t *testing.T) {
	for _, format := range formats {
		raw := []byte(" { \"unknown\": 9007199254740993, \"model\":\"x\" } \n")
		s, body, err := ConvertRequest(context.Background(), format, format, "x", raw)
		if err != nil || !bytes.Equal(body, raw) {
			t.Fatalf("native request reencoded: %s %v", body, err)
		}
		body, err = s.Response(context.Background(), raw)
		if err != nil || !bytes.Equal(body, raw) {
			t.Fatalf("native response reencoded: %s %v", body, err)
		}
		stream, err := s.NewResponseStream()
		if err != nil {
			t.Fatal(err)
		}
		event := []byte(": native\r\nevent: vendor_extension\r\ndata: {\"unknown\":true}\r\n\r\n")
		body, err = stream.TransformEvent(event)
		if err != nil || !bytes.Equal(body, event) {
			t.Fatal("native stream reencoded")
		}
		if _, err = stream.Finish(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSpecializedSelectionBeforeConversion(t *testing.T) {
	for _, fixture := range specials {
		if fixture.Phase != "request" || fixture.Name == "trailing-assistant" {
			continue
		}
		t.Run(fixture.Name, func(t *testing.T) {
			to := Chat
			if fixture.Format == Chat {
				to = Responses
			}
			s, out, err := ConvertRequest(context.Background(), fixture.Format, to, "fixture-model", []byte(fixture.Raw))
			if !errors.Is(err, ErrSpecialized) || s != nil || len(out) != 0 {
				t.Fatalf("feature was not selected before conversion: %v %s", err, out)
			}
		})
	}
	_, _, err := ConvertRequest(context.Background(), Chat, Messages, "fixture-model", []byte(`{"messages":null}`))
	if err == nil || errors.Is(err, ErrSpecialized) {
		t.Fatalf("invalid request must not trigger fallback: %v", err)
	}
}

func TestTruncatedStreamsWithholdSuccess(t *testing.T) {
	for _, client := range formats {
		for _, upstream := range formats {
			if client == upstream {
				continue
			}
			t.Run(string(upstream)+"_to_"+string(client), func(t *testing.T) {
				stream, _ := newTestSession(t, client, upstream).NewResponseStream()
				events := streams[upstream]
				if upstream != Chat {
					events = events[:len(events)-1]
				}
				var out []byte
				for _, event := range events {
					chunk, err := stream.TransformEvent(frame(event))
					if err != nil {
						t.Fatal(err)
					}
					out = append(out, chunk...)
				}
				tail, err := stream.Finish()
				if err == nil || len(tail) > 0 {
					t.Fatalf("truncation became success: %s %v", tail, err)
				}
				for _, marker := range []string{"response.completed", "message_stop", "[DONE]"} {
					if bytes.Contains(out, []byte(marker)) {
						t.Fatalf("success escaped before terminal: %s", out)
					}
				}
				if tail, err = stream.Finish(); err == nil || len(tail) > 0 {
					t.Fatal("truncation error was not sticky")
				}
			})
		}
	}
}

func TestOpaqueAndErrorResponsesNeverFallback(t *testing.T) {
	for _, raw := range []string{
		`{"id":"m","type":"message","role":"assistant","content":[{"type":"thinking","thinking":"secret","signature":"opaque"}],"stop_reason":"end_turn"}`,
		`{"type":"error","error":{"message":"private upstream error"}}`,
	} {
		s := newTestSession(t, Chat, Messages)
		out, err := s.Response(context.Background(), []byte(raw))
		if err == nil || errors.Is(err, ErrSpecialized) || len(out) > 0 {
			t.Fatalf("unsafe response returned: %s %v", out, err)
		}
		if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "private upstream") {
			t.Fatal("error leaked upstream content")
		}
	}
}

func TestPlainReasoningResponse(t *testing.T) {
	raw := []byte(`{"id":"c","object":"chat.completion","model":"fixture-model","choices":[{"index":0,"message":{"role":"assistant","reasoning_content":"Reason","content":"Answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`)
	for _, client := range []Format{Responses, Messages, Gemini} {
		s := newTestSession(t, client, Chat)
		out, err := s.Response(context.Background(), raw)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(out, []byte("Reason")) || !bytes.Contains(out, []byte("Answer")) {
			t.Fatalf("plain reasoning lost: %s", out)
		}
	}
}

func TestSessionCancellationAndSingleStream(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s, _, err := ConvertRequest(ctx, Chat, Messages, "fixture-model", []byte(requests[Chat]))
	if err != nil {
		t.Fatal(err)
	}
	stream, err := s.NewResponseStream()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.NewResponseStream(); err == nil {
		t.Fatal("session reused for another stream")
	}
	cancel()
	if _, err = stream.TransformEvent(frame(streams[Messages][0])); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}

func TestCodecPrecisionAndTrailingJSON(t *testing.T) {
	var value any
	if err := (numberCodec{}).Unmarshal([]byte(`{"n":9007199254740993}`), &value); err != nil {
		t.Fatal(err)
	}
	if value.(map[string]any)["n"] != json.Number("9007199254740993") {
		t.Fatal("number rounded")
	}
	if err := (numberCodec{}).Unmarshal([]byte(`{} {}`), &value); err == nil {
		t.Fatal("trailing JSON accepted")
	}
}
