package proxyhandler

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/deliciousbuding/metapi-go/transform/anthropic/messages"
)

func TestDirectRelayKitSelection(t *testing.T) {
	for _, up := range directWireFixtures {
		for _, down := range directWireFixtures {
			if up.path == down.path {
				continue // Native requests do not enter this conversion boundary.
			}
			t.Run(down.provider+"_to_"+up.provider, func(t *testing.T) {
				body, session, err := prepareDirectProtocolRequest(t.Context(), &store.DirectEndpoint{}, []byte(down.request), down.path, up.path, "actual-model", true, messages.Options{})
				if err != nil {
					t.Fatal(err)
				}
				wantSession := down.provider != "anthropic"
				if (session != nil) != wantSession {
					t.Fatalf("RelayKit session selected=%t want=%t", session != nil, wantSession)
				}
				var payload map[string]json.RawMessage
				if json.Unmarshal(body, &payload) != nil {
					t.Fatal("conversion produced invalid JSON")
				}
				if session != nil && up.provider != "gemini" && (string(payload["model"]) != `"actual-model"` || string(payload["stream"]) != "true") {
					t.Fatal("selected model or HTTP streaming mode was lost")
				}
			})
		}
	}
	t.Run("structured output keeps its specialized owner", func(t *testing.T) {
		body := []byte(`{"model":"m","messages":[{"role":"user","content":"hi"}],"response_format":{"type":"json_schema","json_schema":{"name":"result","strict":true,"schema":{"type":"object","additionalProperties":false,"properties":{}}}}}`)
		_, session, err := prepareDirectProtocolRequest(t.Context(), &store.DirectEndpoint{}, body, "/v1/chat/completions", "/v1/responses", "m", false, messages.Options{})
		if err != nil || session != nil {
			t.Fatalf("specialized schema owner: session=%v err=%v", session != nil, err)
		}
	})
}

func TestDirectFallbackNewAPIMissingResponses(t *testing.T) {
	fixture := installDirectFallbackFixture(t, []string{"chat", "responses"}, map[string]int{"responses": http.StatusNotFound}, `{"error":{"message":"Invalid URL (POST /v1/responses)","type":"invalid_request_error","param":"","code":""}}`, false)
	out := directFallbackHTTPRequest("responses", false)
	calls, logs := fixture.results()
	if out.Code != http.StatusOK || !reflect.DeepEqual(calls, []string{"responses", "chat"}) || len(logs) != 2 || logs[0].Status != "failed" || logs[1].Status != "success" {
		t.Fatalf("New API protocol fallback status=%d calls=%v logs=%d", out.Code, calls, len(logs))
	}
}

func TestDirectRelayKitFramingAndFailure(t *testing.T) {
	const start = `data: {"id":"chat_fixture","created":1700000000,"choices":[{"index":0,"delta":{"content":"合成文本"}}]}` + "\r\n\r\n"
	const finish = `data: {"id":"chat_fixture","created":1700000000,"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}` + "\r\n\r\n"
	const done = "data: [DONE]\r\n\r\n"
	run := func(r io.Reader, truncated bool) []byte {
		t.Helper()
		_, session, err := prepareDirectProtocolRequest(t.Context(), &store.DirectEndpoint{}, []byte(`{"input":"hi","prompt_cache_key":"private-key"}`), "/v1/responses", "/v1/chat/completions", "m", true, messages.Options{})
		if err != nil || session == nil {
			t.Fatal(err)
		}
		s, err := session.NewResponseStream()
		if err != nil {
			t.Fatal(err)
		}
		reader := newProtocolBridgeBody(io.NopCloser(r), s, 1<<20)
		out, err := io.ReadAll(reader)
		if !truncated {
			if err != nil {
				t.Fatal(err)
			}
			// Each attempt owns its generated identity and timestamp. Normalize only
			// the values declared by response.created, including references to its ID.
			first := parseSseBlock(strings.SplitN(string(out), "\n\n", 2)[0])
			if first == nil {
				t.Fatal("missing response.created")
			}
			var created struct {
				Response struct {
					ID   string
					Time int64 `json:"created_at"`
				}
			}
			if first.Event != "response.created" || json.Unmarshal([]byte(first.Data), &created) != nil || created.Response.ID == "" || created.Response.Time == 0 {
				t.Fatal("invalid response.created")
			}
			normalized := strings.ReplaceAll(string(out), created.Response.ID, "fixture-id")
			return []byte(strings.ReplaceAll(normalized, `"created_at":`+strconv.FormatInt(created.Response.Time, 10), `"created_at":1`))
		}
		if err == nil {
			t.Fatal("truncated stream succeeded")
		}
		failure := reader.ResponsesFailureEvents("upstream stream idle timeout")
		if len(failure) == 0 || len(reader.ResponsesFailureEvents("again")) != 0 || bytes.Contains(failure, []byte("合成文本")) || bytes.Contains(failure, []byte("private")) || bytes.Contains(failure, []byte("[DONE]")) {
			t.Fatalf("failure events: %s", failure)
		}
		var seq int
		var id string
		var last string
		for _, block := range strings.Split(strings.TrimSpace(string(append(out, failure...))), "\n\n") {
			e := parseSseBlock(block)
			if e == nil {
				t.Fatalf("bad event %s", block)
			}
			var v struct {
				Type, Code, Message string
				Sequence            *int `json:"sequence_number"`
				Response            struct {
					ID, Status string
					Error      struct{ Code string }
				}
			}
			if err := json.Unmarshal([]byte(e.Data), &v); err != nil || v.Sequence == nil || *v.Sequence != seq || e.Event != v.Type {
				t.Fatalf("event %d: %s", seq, block)
			}
			seq++
			switch v.Type {
			case "response.created":
				id = v.Response.ID
			case "error":
				if v.Code != "upstream_error" || v.Message != "upstream stream idle timeout" {
					t.Fatalf("bad error: %s", block)
				}
			case "response.failed":
				if last != "error" || v.Response.ID != id || id == "" || v.Response.Status != "failed" || v.Response.Error.Code != "upstream_error" {
					t.Fatalf("bad failure: %s", block)
				}
			}
			last = v.Type
		}
		if last != "response.failed" {
			t.Fatal("missing terminal failure")
		}
		return out
	}
	complete := start + finish + done
	frames := run(strings.NewReader(complete), false)
	single := run(iotest.OneByteReader(strings.NewReader(complete)), false)
	if !bytes.Equal(frames, single) || !bytes.Contains(frames, []byte("response.completed")) {
		t.Fatalf("fragmentation changed output: %s vs %s", frames, single)
	}
	run(iotest.OneByteReader(strings.NewReader(start+finish)), true)
}
