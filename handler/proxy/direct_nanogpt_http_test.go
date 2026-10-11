package proxyhandler

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/deliciousbuding/metapi-go/transform/anthropic/messages"
)

type nanoGPTCaptureStream struct {
	next protocolEventStream
	bytes.Buffer
}

func (s *nanoGPTCaptureStream) TransformEvent(frame []byte) ([]byte, error) {
	out, err := s.next.TransformEvent(frame)
	s.Write(out)
	return out, err
}
func (s *nanoGPTCaptureStream) Finish() ([]byte, error) {
	out, err := s.next.Finish()
	s.Write(out)
	return out, err
}

func nanoGPTHTTPFrames(content string) []byte {
	var result []byte
	result = append(result, nanoGPTFrame(nanoGPTChoice(0, map[string]any{"role": "assistant", "reasoning": "supplied thinking"}, nil), false)...)
	for i := 0; i < len(content); i++ {
		result = append(result, nanoGPTFrame(nanoGPTChoice(0, map[string]any{"content": content[i : i+1]}, nil), false)...)
	}
	result = append(result, nanoGPTFrame(nanoGPTChoice(0, map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "native-id", "type": "function", "function": map[string]string{"name": "ExistingCase", "arguments": "{}"}}}}, "stop"), false)...)
	result = append(result, nanoGPTFrame([]any{}, true)...)
	result = append(result, []byte("data: [DONE]\n\n")...)
	return result
}

// Leaf integration: a real HTTP transport feeds the NanoGPT adapter and the
// existing four client codecs. Store/dispatcher profile wiring is owned by the
// integrating caller, not installed or simulated by this helper test.
func TestDirectNanoGPTHTTPFourClientFormats(t *testing.T) {
	content := "first output\n<Read>{\"path\":\"receipt\",\"offset\":9007199254740993}</Read>\nafter"
	for _, down := range directWireFixtures {
		for _, stream := range []bool{false, true} {
			for _, compressed := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/stream_%t/gzip_%t", down.provider, stream, compressed), func(t *testing.T) {
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						body, _ := io.ReadAll(r.Body)
						if r.URL.Path != "/exact/nanogpt/chat" || r.Header.Get("Authorization") != "Bearer fixture-only" || !bytes.Contains(body, []byte(`"model":"provider-model"`)) || !bytes.Contains(body, []byte(`"name":"Read"`)) {
							t.Errorf("request wire changed: %s %s", r.URL, body)
						}
						payload := nanoGPTTestResponse(content)
						w.Header().Set("Content-Type", "application/json")
						if stream {
							payload = nanoGPTHTTPFrames(content)
							w.Header().Set("Content-Type", "text/event-stream")
						}
						var writer io.Writer = w
						if compressed {
							w.Header().Set("Content-Encoding", "gzip")
							codec := gzip.NewWriter(w)
							defer codec.Close()
							writer = codec
						}
						for offset := 0; offset < len(payload); offset += 11 {
							if _, err := writer.Write(payload[offset:min(offset+11, len(payload))]); err != nil {
								return
							}
						}
					}))
					defer server.Close()
					request := strings.ReplaceAll(down.request, `"echo"`, `"Read"`)
					if down.provider == "anthropic" {
						request = strings.TrimSuffix(request, "}") + `,"metadata":{"user_id":"nano-http-conversation"}}`
					}
					var obj map[string]json.RawMessage
					if json.Unmarshal([]byte(request), &obj) != nil {
						t.Fatal("invalid client fixture")
					}
					if down.provider != "gemini" {
						obj["model"] = json.RawMessage(`"provider-model"`)
						obj["stream"] = json.RawMessage(fmt.Sprint(stream))
					}
					encoded, _ := json.Marshal(obj)
					options := messages.Options{}
					if down.provider == "anthropic" {
						r := makeProxyReq(http.MethodPost, down.path, request)
						ctx, failure := PrepareCtx(r, SurfConfig{DownstreamPath: down.path, RequireModel: true, SurfaceFormat: "anthropic"})
						if failure != nil {
							t.Fatal(failure)
						}
						selected := &routing.SelectedChannel{Channel: store.RouteChannel{ID: 1}, Site: store.Site{ID: 1, URL: server.URL}, Account: store.Account{ID: 1}, ActualModel: "provider-model", TokenValue: "fixture-only"}
						replay := newMessagesBridgeRequest(r, ctx, selected)
						if err := replay.ready(); err != nil {
							t.Fatal(err)
						}
						options = replay.Options()
					}
					encoded, err := directConvertRequest(encoded, down.path, "/v1/chat/completions", "provider-model", stream, options)
					if err != nil {
						t.Fatal(err)
					}
					encoded, err = prepareDirectNanoGPTRequest(encoded)
					if err != nil {
						t.Fatal(err)
					}
					tools, err := newDirectNanoGPTTools(encoded)
					if err != nil {
						t.Fatal(err)
					}
					req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+"/exact/nanogpt/chat", bytes.NewReader(encoded))
					req.Header.Set("Content-Type", "application/json")
					req.Header.Set("Authorization", "Bearer fixture-only")
					req.Header.Set("Accept-Encoding", "gzip")
					resp, err := server.Client().Do(req)
					if err != nil {
						t.Fatal(err)
					}
					defer resp.Body.Close()
					var out []byte
					if stream {
						decoded := proxy.WrapUpstreamStreamBody(resp.Header, resp.Body)
						if !decoded.Readable {
							t.Fatal("unreadable fixture encoding")
						}
						source := resp.Body
						if decoded.Reader != nil {
							source = decoded.Reader
						}
						capture := &nanoGPTCaptureStream{next: newDirectNanoGPTStream(tools)}
						var codec protocolEventStream = capture
						if next := directResponseStream(down.path, "/v1/chat/completions", "provider-model", options); next != nil {
							codec = &chainedProtocolStream{first: capture, second: next}
						}
						reader := newProtocolBridgeBody(source, codec, 1<<20)
						defer reader.Close()
						out, err = io.ReadAll(reader)
						if err != nil {
							t.Fatalf("client codec failed: %v body=%s", err, out)
						}
						if !strings.Contains(capture.String(), "nanogpt_xml_") || strings.Contains(capture.String(), `\u003cRead`) {
							t.Fatalf("XML was not converted: %s", capture.String())
						}
						if usage := reader.original.Result().Usage; usage.PromptTokens != 5 || usage.CompletionTokens != 2 || usage.TotalTokens != 7 {
							t.Fatalf("original HTTP usage lost: %+v", usage)
						}
					} else {
						raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
						if err != nil {
							t.Fatal(err)
						}
						decoded := normalizeBufferedUpstreamBody(resp, raw, upstreamBodyIdent{})
						if !decoded.readable {
							t.Fatal("unreadable JSON")
						}
						normalized, err := normalizeDirectNanoGPTJSON(decoded.bytes, tools)
						if err != nil {
							t.Fatal(err)
						}
						if !bytes.Contains(normalized, []byte("nanogpt_xml_")) {
							t.Fatalf("XML was not converted: %s", normalized)
						}
						out, err = directConvertResponse(normalized, down.path, "/v1/chat/completions", "provider-model", options)
						if err != nil {
							t.Fatal(err)
						}
					}
					for _, marker := range []string{"Read", "ExistingCase", "9007199254740993", down.toolMarker} {
						if !bytes.Contains(out, []byte(marker)) {
							t.Errorf("lost %s in client output: %s", marker, out)
						}
					}
					text := string(out)
					if stream {
						text = nanoGPTClientStreamText(t, out, down.provider)
					}
					if !strings.Contains(text, "first output") || !strings.Contains(text, "after") {
						t.Fatalf("client text was lost: %q", text)
					}
				})
			}
		}
	}
}

func nanoGPTClientStreamText(t *testing.T, frames []byte, provider string) string {
	t.Helper()
	var text strings.Builder
	for _, line := range strings.Split(string(frames), "\n") {
		if !strings.HasPrefix(line, "data: ") || strings.TrimPrefix(line, "data: ") == "[DONE]" {
			continue
		}
		var event struct {
			Type    string          `json:"type"`
			Delta   json.RawMessage `json:"delta"`
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
			Candidates []struct {
				Content struct {
					Parts []struct {
						Text    string `json:"text"`
						Thought bool   `json:"thought"`
					} `json:"parts"`
				} `json:"content"`
			} `json:"candidates"`
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
			t.Fatal(err)
		}
		switch provider {
		case "openai":
			for _, choice := range event.Choices {
				text.WriteString(choice.Delta.Content)
			}
		case "openai_responses":
			if event.Type == "response.output_text.delta" {
				var part string
				_ = json.Unmarshal(event.Delta, &part)
				text.WriteString(part)
			}
		case "anthropic":
			if event.Type == "content_block_delta" {
				var delta struct{ Type, Text string }
				_ = json.Unmarshal(event.Delta, &delta)
				if delta.Type == "text_delta" {
					text.WriteString(delta.Text)
				}
			}
		case "gemini":
			for _, candidate := range event.Candidates {
				for _, part := range candidate.Content.Parts {
					if !part.Thought {
						text.WriteString(part.Text)
					}
				}
			}
		}
	}
	return text.String()
}

func TestDirectNanoGPTHTTPFirstOutputCancelIdleAndLimit(t *testing.T) {
	for _, ending := range []string{"cancel", "idle", "limit", "truncated", "error"} {
		t.Run(ending, func(t *testing.T) {
			prefix := nanoGPTFrame(nanoGPTChoice(0, map[string]any{"role": "assistant", "content": "first output without newline"}, nil), true)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = w.Write(prefix)
				w.(http.Flusher).Flush()
				switch ending {
				case "cancel", "idle":
					<-r.Context().Done()
				case "limit":
					_, _ = io.WriteString(w, "data: "+strings.Repeat("x", 1000)+"\n\n")
				case "error":
					_, _ = io.WriteString(w, "event: error\ndata: {\"error\":{\"message\":\"fixture failure\"}}\n\n")
				}
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
			resp, err := server.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			var source io.ReadCloser = resp.Body
			var idle *streamIdleBody
			if ending == "idle" {
				idle = &streamIdleBody{ReadCloser: resp.Body}
				idle.guard = newStreamIdleGuard(30*time.Millisecond, idle.closeUnderlying)
				source = idle
			}
			limit := int64(1 << 20)
			if ending == "limit" {
				limit = int64(len(prefix) + 20)
			}
			reader := newProtocolBridgeBody(source, newDirectNanoGPTStream(nanoGPTTestTools(t)), limit)
			defer reader.Close()
			first := make([]byte, 4096)
			n, err := reader.Read(first)
			if err != nil || !bytes.Contains(first[:n], []byte("first output without newline")) {
				t.Fatalf("first output waited for completion: n=%d error=%v output=%s", n, err, first[:n])
			}
			if ending == "cancel" {
				cancel()
			}
			rest, err := io.ReadAll(reader)
			all := append(first[:n], rest...)
			if err == nil || bytes.Contains(all, []byte("[DONE]")) {
				t.Fatalf("failed HTTP stream got a success terminal: %v %s", err, all)
			}
			switch ending {
			case "cancel":
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancellation cause changed: %v", err)
				}
			case "idle":
				if !idle.guard.fired.Load() {
					t.Fatal("idle guard did not own termination")
				}
			case "limit":
				if !errors.Is(err, errMessagesChatStreamLimit) {
					t.Fatalf("limit cause changed: %v", err)
				}
			case "truncated":
				if !errors.Is(err, io.ErrUnexpectedEOF) {
					t.Fatalf("EOF became successful: %v", err)
				}
			}
			if usage := reader.original.Result().Usage; usage.PromptTokens != 5 || usage.CompletionTokens != 2 || usage.TotalTokens != 7 {
				t.Fatalf("failure lost observed usage: %+v", usage)
			}
		})
	}
}
