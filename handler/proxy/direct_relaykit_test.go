package proxyhandler

import (
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

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
	if out.Code != http.StatusOK || !reflect.DeepEqual(calls, []string{"responses", "chat"}) || len(logs) != 1 || logs[0].Status != "success" {
		t.Fatalf("New API protocol fallback status=%d calls=%v logs=%d", out.Code, calls, len(logs))
	}
}
