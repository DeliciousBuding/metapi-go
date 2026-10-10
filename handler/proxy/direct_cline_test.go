package proxyhandler

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func clineTestObject(t *testing.T, raw []byte) map[string]json.RawMessage {
	t.Helper()
	var out map[string]json.RawMessage
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestDirectClineBridgeProjectsTextReasoningOnly(t *testing.T) {
	native, err := normalizeDirectClineJSON([]byte(clineTestJSON), true)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(native, []byte(`"reasoning_details"`)) || !bytes.Contains(native, []byte(`"reasoning":"fallback"`)) {
		t.Fatal("native Chat lost provider fields")
	}
	projected, err := projectDirectClineReasoning(native, false)
	if err != nil || !bytes.Contains(projected, []byte(`"reasoning_content":"think carefully"`)) || bytes.Contains(projected, []byte(`"reasoning_details"`)) || bytes.Contains(projected, []byte(`"reasoning":`)) || !bytes.Contains(projected, []byte(`9007199254740993`)) {
		t.Fatalf("projection lost normalized content/metadata: %s %v", projected, err)
	}
	for _, detail := range []string{`{"type":"reasoning.encrypted","text":"", "data":"opaque"}`, `{"text":"visible","signature":"opaque"}`, `{"summary":"unconverted"}`} {
		raw := []byte(`{"choices":[{"index":0,"message":{"role":"assistant","content":"ok","reasoning_details":[` + detail + `]},"finish_reason":"stop"}]}`)
		native, err := normalizeDirectClineJSON(raw, true)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := projectDirectClineReasoning(native, false); err == nil {
			t.Fatalf("silently flattened non-text reasoning: %s", detail)
		}
	}
}

func TestDirectClineUsageChunkHasCanonicalObject(t *testing.T) {
	body := []byte(`{"choices":[],"usage":{"prompt_tokens":3,"completion_tokens":2}}`)
	out, _, err := directClineChatJSON(body, true)
	if err != nil || !bytes.Contains(out, []byte(`"object":"chat.completion.chunk"`)) || !bytes.Contains(out, []byte(`"prompt_tokens":3`)) {
		t.Fatalf("usage-only chunk not normalized: %s %v", out, err)
	}
	if _, _, err := directClineChatJSON([]byte(`{"object":"response","choices":[],"usage":{}}`), true); err == nil {
		t.Fatal("conflicting object type was silently rewritten")
	}
}

func TestDirectClineRequest(t *testing.T) {
	input := `{"model":"cline-pass/fixture","precision":9007199254740993,"messages":[
		{"role":"user","content":""},{"role":"system"},{"role":"developer","content":null},
		{"role":"user","content":[]},{"role":"user","content":[{"type":"text"}]},
		{"role":"user","content":[{"type":"text","text":null}]},
		{"role":"assistant","content":null,"reasoning_content":"consider","tool_calls":[{"id":"tool_1","function":{"name":"lookup","arguments":"{\"id\":9007199254740993}"}}]},
		{"role":"tool","tool_call_id":"tool_1","content":""},{"role":"user","content":"   "},
		{"role":"user","content":[{"type":"text","text":""},{"type":"image_url","image_url":{"url":"data:image/png;base64,AA=="}}]},
		{"role":"user","content":[{"type":"image_url","image_url":{"url":"https://image.example/fixture.png"}}]}
	],"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object","properties":{"id":{"type":"integer","minimum":9007199254740993}}}}}]} `
	out, err := prepareDirectClineRequest([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	obj := clineTestObject(t, out)
	if string(obj["precision"]) != "9007199254740993" || !bytes.Contains(obj["tools"], []byte("9007199254740993")) {
		t.Fatalf("lost precision or tool schema: %s", out)
	}
	var messages []map[string]json.RawMessage
	if err := json.Unmarshal(obj["messages"], &messages); err != nil {
		t.Fatal(err)
	}
	for i := range 6 {
		if string(messages[i]["content"]) != `[{"type":"text","text":""}]` {
			t.Errorf("empty required content %d was not repaired: %s", i, messages[i]["content"])
		}
	}
	if string(messages[6]["content"]) != "null" || string(messages[6]["reasoning"]) != `"consider"` || messages[6]["reasoning_content"] != nil || !bytes.Contains(messages[6]["tool_calls"], []byte("9007199254740993")) {
		t.Fatalf("assistant reasoning/tools changed: %s", obj["messages"])
	}
	if string(messages[7]["content"]) != `""` || string(messages[8]["content"]) != `"   "` || !bytes.Contains(messages[9]["content"], []byte("image_url")) || !bytes.Contains(messages[10]["content"], []byte("https://image.example/fixture.png")) {
		t.Fatal("nonempty, tool or multimodal content changed")
	}
	for _, input := range []string{`null`, `{}`, `{"messages":[]}`, `{"messages":[null]}`, `{"messages":[{"role":null}]}`, `{"messages":[{"role":"assistant","reasoning_content":false}]}`, `{"messages":[{"role":"assistant","reasoning_content":"a","reasoning":"b"}]}`} {
		if _, err := prepareDirectClineRequest([]byte(input)); err == nil {
			t.Errorf("accepted invalid request: %s", input)
		}
	}
}

const clineTestJSON = `{"id":"chatcmpl-fixture","object":"chat.completion","created":9007199254740993,"model":"cline-pass/fixture","provider_metadata":{"billing":9007199254740993},"choices":[{"index":0,"message":{"role":"assistant","content":null,"reasoning":"fallback","reasoning_details":[{"type":"reasoning.text","text":"think "},{"type":"reasoning.text","text":"carefully","index":9007199254740993}],"tool_calls":[{"id":"tool_1","type":"function","function":{"name":"lookup","arguments":"{\"id\":9007199254740993}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":12,"completion_tokens":7,"total_tokens":19,"prompt_tokens_details":{"cached_tokens":3},"completion_tokens_details":{"reasoning_tokens":4}}}`

func clineTestStream() string {
	return ": heartbeat\r\n\r\n" +
		"data: " + `{"success":true,"data":{"id":"chatcmpl-fixture","object":"chat.completion.chunk","model":"cline-pass/fixture","choices":[{"index":0,"delta":{"role":"assistant","reasoning":"fallback","reasoning_details":[{"text":"think "}]}}]}}` + "\r\n\r\n" +
		"data: " + `{"id":"chatcmpl-fixture","object":"chat.completion.chunk","model":"cline-pass/fixture","created":9007199254740993,"choices":[{"index":0,"delta":{"reasoning":"carefully","tool_calls":[{"index":0,"id":"tool_1","function":{"name":"lookup","arguments":"{\"id\":9007199254740993}"}}]},"finish_reason":"tool_calls"}]}` + "\n\n" +
		"data: " + `{"success":true,"data":{"id":"chatcmpl-fixture","model":"cline-pass/fixture","choices":[],"usage":{"prompt_tokens":12,"completion_tokens":7,"total_tokens":19,"prompt_tokens_details":{"cached_tokens":3},"completion_tokens_details":{"reasoning_tokens":4}}}}` + "\n\n" +
		"data: [DONE]\n\n"
}

func TestDirectClineHTTPFixture(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("X-Client-Type") != "cline-cli" || r.Header.Get("Authorization") != "Bearer fixture-key" {
			t.Error("incorrect Cline request headers or method")
		}
		if r.URL.Path != "/api/v1/chat/completions" && r.URL.Path != "/api/v1/custom/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil || !bytes.Contains(body, []byte(`"content":[{"type":"text","text":""}]`)) {
			t.Errorf("request content not normalized: %s (%v)", body, err)
		}
		payload := `{"success":true,"data":` + clineTestJSON + `}`
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("stream") == "true" {
			payload = clineTestStream()
			w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		}
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("ETag", `"wrapped"`)
		gzipWriter := gzip.NewWriter(w)
		// Both codec bytes and SSE frames cross transport read boundaries.
		for offset := 0; offset < len(payload); offset += 11 {
			if _, err := io.WriteString(gzipWriter, payload[offset:min(offset+11, len(payload))]); err != nil {
				return
			}
			if err := gzipWriter.Flush(); err != nil {
				return
			}
			w.(http.Flusher).Flush()
		}
		_ = gzipWriter.Close()
	}))
	defer server.Close()
	for _, path := range []string{"/api/v1/chat/completions", "/api/v1/custom/chat/completions"} {
		for _, stream := range []bool{false, true} {
			t.Run(path+"/stream="+map[bool]string{true: "true", false: "false"}[stream], func(t *testing.T) {
				body, err := prepareDirectClineRequest([]byte(`{"model":"cline-pass/fixture","messages":[{"role":"user","content":""}]}`))
				if err != nil {
					t.Fatal(err)
				}
				target := server.URL + path
				if stream {
					target += "?stream=true"
				}
				req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, target, bytes.NewReader(body))
				if err != nil {
					t.Fatal(err)
				}
				buildDirectClineHeaders(req.Header)
				req.Header.Set("Authorization", "Bearer fixture-key")
				// Keep gzip on the fixture wire for the product's lazy decoder.
				req.Header.Set("Accept-Encoding", "gzip")
				resp, err := server.Client().Do(req)
				if err != nil {
					t.Fatal(err)
				}
				if err := normalizeDirectClineResponse(t.Context(), resp, stream); err != nil {
					t.Fatal(err)
				}
				defer resp.Body.Close()
				out, err := io.ReadAll(resp.Body)
				if err != nil {
					t.Fatal(err)
				}
				if stream {
					if !bytes.HasSuffix(out, []byte("data: [DONE]\n\n")) || bytes.Contains(out, []byte(`"success"`)) || !bytes.Contains(out, []byte(`"reasoning_content":"think "`)) || !bytes.Contains(out, []byte(`"reasoning_content":"carefully"`)) || !bytes.Contains(out, []byte("9007199254740993")) {
						t.Fatalf("bad stream normalization: %s", out)
					}
					if resp.Header.Get("Content-Encoding") != "" || resp.Header.Get("ETag") != "" || resp.ContentLength != -1 {
						t.Fatal("stale stream headers")
					}
					analyzer := newIncrementalSseAnalyzer()
					analyzer.Push(out)
					usage := analyzer.Result().Usage
					if usage.PromptTokens != 12 || usage.CompletionTokens != 7 || usage.TotalTokens != 19 {
						t.Fatalf("normalized usage is not billable: %+v", usage)
					}
				} else {
					normalized := normalizeBufferedUpstreamBody(resp, out, upstreamBodyIdent{})
					out, err = normalizeDirectClineJSON(normalized.bytes, normalized.readable)
					if err != nil {
						t.Fatal(err)
					}
					if bytes.Contains(out, []byte(`"success"`)) || !bytes.Contains(out, []byte(`"reasoning_content":"think carefully"`)) || !bytes.Contains(out, []byte("9007199254740993")) || !bytes.Contains(out, []byte(`"reasoning_tokens":4`)) {
						t.Fatalf("bad buffered normalization: %s", out)
					}
				}
			})
		}
	}
}

func TestDirectClineJSONFailures(t *testing.T) {
	for _, body := range []string{
		`{"success":false,"error":"model not found"}`,
		`{"success":false,"errors":[{"message":"quota exceeded"}]}`,
		`{"success":false}`, `{"success":null,"choices":[]}`, `{"success":"true","data":{}}`,
		`{"success":true}`, `{"success":true,"data":null}`, `{"success":true,"data":{"error":"nested"}}`,
		`{"success":true,"data":{"success":false,"choices":[{"index":0,"message":{"content":"not success"}}]}}`,
		`{"error":{"message":"denied"}}`, `{"success":true,"error":"contradictory","data":` + clineTestJSON + `}`,
		`{"data":` + clineTestJSON + `}`, `{}`, `null`, `[]`, `{"choices":[]}`, `{"choices":null}`, `<html>error</html>`,
		`{"choices":[{"index":0,"message":null}]}`, `{"choices":[{"index":null,"message":{}}]}`,
		`{"choices":[{"index":0,"message":{"reasoning_details":{}}}]}`,
	} {
		t.Run(body, func(t *testing.T) {
			if _, err := normalizeDirectClineJSON([]byte(body), true); err == nil {
				t.Fatalf("accepted invalid/error response: %s", body)
			}
		})
	}
	if _, err := normalizeDirectClineJSON([]byte(clineTestJSON), false); err == nil {
		t.Fatal("accepted opaque encoded response")
	}
	out, err := normalizeDirectClineJSON([]byte(clineTestJSON), true)
	if err != nil || !bytes.Contains(out, []byte(`"reasoning_content":"think carefully"`)) {
		t.Fatalf("valid plain Chat response rejected: %s (%v)", out, err)
	}
	plain := []byte(`{"id":"native","model":"fixture","choices":[{"index":0,"message":{"role":"assistant","content":"ok","reasoning_content":"native reason"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`)
	out, err = normalizeDirectClineJSON(plain, true)
	if err != nil || !bytes.Contains(out, []byte(`"reasoning_content":"native reason"`)) || !bytes.Contains(out, []byte(`"total_tokens":3`)) {
		t.Fatalf("native Chat compatibility lost: %s (%v)", out, err)
	}
}

func TestDirectClineHTTP200Failure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"success":false,"errors":[{"message":"fixture quota exceeded"}]}`)
	}))
	defer server.Close()
	resp, err := server.Client().Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("fixture must exercise an in-band failure: %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if out, err := normalizeDirectClineJSON(body, true); err == nil || len(out) != 0 || !strings.Contains(err.Error(), "fixture quota exceeded") {
		t.Fatalf("HTTP 200 failure was accepted or lost: %s (%v)", out, err)
	}
}

func TestDirectClineRejectsFalseSuccessWithData(t *testing.T) {
	// A completion-looking data field must never override an explicit failure.
	body := []byte(`{"success":false,"data":` + clineTestJSON + `}`)
	if out, err := normalizeDirectClineJSON(body, true); err == nil || len(out) != 0 {
		t.Fatalf("success=false was accepted as a completion: %s (%v)", out, err)
	}
}

func TestDirectClineStreamFailures(t *testing.T) {
	valid := clineTestStream()
	unfinished := "data: " + `{"choices":[{"index":0,"delta":{"content":"partial"}}]}` + "\n\n"
	for name, input := range map[string]string{
		"wrapped failure":       `data: {"success":false,"error":"quota exceeded"}` + "\n\n",
		"wrapped errors":        `data: {"success":false,"errors":[{"message":"quota exceeded"}]}` + "\n\n",
		"plain error":           `data: {"error":{"message":"upstream failed"}}` + "\n\n",
		"event error":           "event: error\n\n",
		"nested event error":    `data: {"event":"error","data":{"error":"bad"}}` + "\n\n",
		"named error with data": "event: error\ndata: {}\n\n",
		"missing DONE":          strings.TrimSuffix(valid, "data: [DONE]\n\n"),
		"partial choice":        unfinished,
		"unfinished at DONE":    unfinished + "data: [DONE]\n\n",
		"bare DONE":             "data: [DONE]\n\n",
		"DONE prefix":           unfinished + "data: [DONE]not-a-marker\n\n",
		"duplicate DONE":        valid + "data: [DONE]\n\n",
		"late error":            valid + "event: error\n\n",
		"late choice":           valid + unfinished,
		"choice after finish":   strings.TrimSuffix(valid, "data: [DONE]\n\n") + unfinished + "data: [DONE]\n\n",
		"malformed JSON":        "data: {\n\n",
		"not SSE":               clineTestJSON + "\n\n",
		"unrelated JSON":        "data: {\"ok\":true}\n\n",
		"oversized frame":       "data: " + strings.Repeat("x", maxIncrementalSsePendingBytes+1) + "\n\n",
	} {
		t.Run(name, func(t *testing.T) {
			reader := newDirectClineBody(t.Context(), io.NopCloser(strings.NewReader(input)), 4<<20, time.Minute)
			defer reader.Close()
			out, err := io.ReadAll(reader)
			if err == nil {
				t.Fatal("invalid stream succeeded")
			}
			if bytes.Contains(out, []byte("data: [DONE]")) {
				t.Fatalf("failure emitted success terminal: %s", out)
			}
		})
	}
	t.Run("input byte limit", func(t *testing.T) {
		reader := newDirectClineBody(t.Context(), io.NopCloser(strings.NewReader(valid)), int64(len(valid)-1), time.Minute)
		defer reader.Close()
		out, err := io.ReadAll(reader)
		if !errors.Is(err, errMessagesChatStreamLimit) || bytes.Contains(out, []byte("data: [DONE]")) {
			t.Fatalf("byte limit reported success: %s (%v)", out, err)
		}
	})
}

func TestDirectClineMultiChoiceTerminal(t *testing.T) {
	first := "data: " + `{"choices":[{"index":0,"delta":{"content":"first"},"finish_reason":"stop"},{"index":1,"delta":{"content":"second"}}]}` + "\n\n"
	last := "data: " + `{"choices":[{"index":1,"delta":{},"finish_reason":"stop"}]}` + "\n\n"
	for _, complete := range []bool{false, true} {
		input := first
		if complete {
			input += last
		}
		body := newDirectClineBody(t.Context(), io.NopCloser(strings.NewReader(input+"data: [DONE]\n\n")), 1<<20, time.Minute)
		out, err := io.ReadAll(body)
		_ = body.Close()
		if complete && (err != nil || !bytes.Contains(out, []byte("data: [DONE]"))) {
			t.Fatalf("valid multiple choices failed: %s (%v)", out, err)
		}
		if !complete && (err == nil || bytes.Contains(out, []byte("data: [DONE]"))) {
			t.Fatalf("unfinished secondary choice succeeded: %s (%v)", out, err)
		}
	}
}

type clineTestReadCloser struct {
	io.ReadCloser
	reads, closes atomic.Int32
	readStarted   chan struct{}
}

func (b *clineTestReadCloser) Read(p []byte) (int, error) {
	b.reads.Add(1)
	if b.readStarted != nil {
		select {
		case b.readStarted <- struct{}{}:
		default:
		}
	}
	return b.ReadCloser.Read(p)
}

func (b *clineTestReadCloser) Close() error {
	b.closes.Add(1)
	return b.ReadCloser.Close()
}

func TestDirectClineLazyCancellationAndLimits(t *testing.T) {
	for _, compressed := range []bool{false, true} {
		t.Run(map[bool]string{true: "gzip", false: "identity"}[compressed], func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			reader, writer := io.Pipe()
			defer writer.Close()
			source := &clineTestReadCloser{ReadCloser: reader, readStarted: make(chan struct{}, 1)}
			resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: source}
			if compressed {
				resp.Header.Set("Content-Encoding", "gzip")
			}
			if err := normalizeDirectClineResponse(ctx, resp, true); err != nil {
				t.Fatal(err)
			}
			if source.reads.Load() != 0 {
				t.Fatal("normalizer read eagerly")
			}
			result := make(chan error, 1)
			go func() {
				_, err := io.ReadAll(resp.Body)
				result <- err
			}()
			select {
			case <-source.readStarted:
			case <-time.After(2 * time.Second):
				t.Fatal("transport read did not start")
			}
			cancel()
			select {
			case err := <-result:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancellation lost: %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("cancellation did not unblock read")
			}
			_ = resp.Body.Close()
			if source.closes.Load() != 1 {
				t.Fatalf("transport closed %d times", source.closes.Load())
			}
		})
	}
	t.Run("idle timeout", func(t *testing.T) {
		reader, writer := io.Pipe()
		defer writer.Close()
		body := newDirectClineBody(t.Context(), reader, 1<<20, 15*time.Millisecond)
		defer body.Close()
		out, err := io.ReadAll(body)
		if !errors.Is(err, errDirectClineIdleTimeout) || len(out) != 0 {
			t.Fatalf("idle timeout succeeded: %s (%v)", out, err)
		}
	})
	t.Run("no unknown encoding fallback", func(t *testing.T) {
		resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}, "Content-Encoding": []string{"br"}}, Body: io.NopCloser(strings.NewReader("opaque"))}
		if err := normalizeDirectClineResponse(t.Context(), resp, true); err == nil {
			t.Fatal("unsupported encoding accepted")
		}
	})
	t.Run("JSON error is not an SSE stream", func(t *testing.T) {
		resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"success":false}`))}
		if err := normalizeDirectClineResponse(t.Context(), resp, true); err == nil {
			t.Fatal("JSON response treated as successful SSE")
		}
	})
}
