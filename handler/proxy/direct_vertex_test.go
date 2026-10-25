package proxyhandler

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

// Vertex Anthropic wire (H batch): a full-stack run through the direct proxy
// — GCP service-account exchange, publisher URL derivation, rawPredict action
// selection, and the body rewrite — against a fixture upstream.

func vertexTestSA(t *testing.T, tokenURI string) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	sa := map[string]string{
		"type":         "service_account",
		"client_email": "vertex@fixture.iam.gserviceaccount.com",
		"private_key":  string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
		"token_uri":    tokenURI,
	}
	raw, err := json.Marshal(sa)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestDirectVertexAnthropicWireHTTP(t *testing.T) {
	var exchanges atomic.Int32
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		exchanges.Add(1)
		if err := r.ParseForm(); err != nil || r.Form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" || r.Form.Get("assertion") == "" {
			t.Error("token exchange was not an RFC 7523 bearer grant")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"fixture-vertex-token","expires_in":3600,"token_type":"Bearer"}`)
	}))
	defer tokenServer.Close()

	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{true: "stream", false: "json"}[stream], func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				action := ":rawPredict"
				if stream {
					action = ":streamRawPredict"
				}
				want := "/v1/projects/my-project/locations/us-central1/publishers/anthropic/models/claude-sonnet-4-5@20250929" + action
				if r.URL.Path != want {
					t.Errorf("vertex path = %q, want %q", r.URL.Path, want)
				}
				if r.Header.Get("Authorization") != "Bearer fixture-vertex-token" {
					t.Error("exchanged token not applied as bearer")
				}
				if r.Header.Get("Anthropic-Version") != "" {
					t.Error("client-facing anthropic-version header leaked upstream")
				}
				var body map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body["model"] != nil {
					t.Error("model must move to the URL")
				}
				if string(body["anthropic_version"]) != `"vertex-2023-10-16"` {
					t.Errorf("anthropic_version: %s", body["anthropic_version"])
				}
				if stream && string(body["stream"]) != "true" {
					t.Error("stream selector lost")
				}
				if !stream {
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, `{"id":"msg-v","type":"message","role":"assistant","model":"claude-sonnet-4-5@20250929","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":1}}`)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				for _, event := range []string{
					`{"type":"message_start","message":{"id":"msg-v","type":"message","role":"assistant","model":"claude-sonnet-4-5@20250929","content":[],"usage":{"input_tokens":3,"output_tokens":0}}}`,
					`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
					`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}`,
					`{"type":"content_block_stop","index":0}`,
					`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}`,
					`{"type":"message_stop"}`,
				} {
					fmt.Fprint(w, "data: "+event+"\n\n")
				}
			}))
			defer upstream.Close()

			endpoint := &store.DirectEndpoint{
				URL:       upstream.URL + "/v1/projects/my-project/locations/us-central1/publishers/anthropic/models",
				Auth:      store.DirectAuthBearer,
				Profile:   "vertex-anthropic",
				ModelPath: true,
			}
			db := installDirectMediaRuntime(t, store.DialectSQLite, store.DirectEndpoints{Messages: endpoint}, store.DirectProtocolMessages, "")
			if _, err := db.Exec(db.Rebind(`UPDATE upstream_models SET name='claude-sonnet-4-5@20250929'`)); err != nil {
				t.Fatal(err)
			}
			sa := vertexTestSA(t, tokenServer.URL)
			if _, err := db.Exec(db.Rebind(`UPDATE upstream_credentials SET kind=?, secret='', gcp_service_account=?`), store.DirectCredentialGCPServiceAccount, sa); err != nil {
				t.Fatal(err)
			}

			body := `{"model":"client-alias","max_tokens":8,"messages":[{"role":"user","content":"hi"}]}`
			if stream {
				body = `{"model":"client-alias","stream":true,"max_tokens":8,"messages":[{"role":"user","content":"hi"}]}`
			}
			recorder := httptest.NewRecorder()
			directMediaRouter().ServeHTTP(recorder, makeProxyReq(http.MethodPost, "/v1/messages", body))
			if recorder.Code != 200 {
				t.Fatalf("status %d: %s", recorder.Code, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), "ok") {
				t.Fatalf("response not relayed: %s", recorder.Body.String())
			}
			if calls.Load() != 1 {
				t.Fatalf("upstream calls: %d", calls.Load())
			}
		})
	}
	if exchanges.Load() == 0 {
		t.Fatal("GCP token exchange never happened")
	}
}

func TestDirectVertexGeminiWireHTTP(t *testing.T) {
	for _, downstream := range []string{"gemini", "chat"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", downstream, stream), func(t *testing.T) {
				var exchanges, calls atomic.Int32
				tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					exchanges.Add(1)
					if err := r.ParseForm(); err != nil || r.Form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" || r.Form.Get("assertion") == "" {
						t.Error("token exchange was not an RFC 7523 bearer grant")
					}
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, `{"access_token":"fixture-vertex-token","expires_in":3600,"token_type":"Bearer"}`)
				}))
				defer tokenServer.Close()

				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					action := ":generateContent"
					if stream {
						action = ":streamGenerateContent"
					}
					want := "/v1/projects/my-project/locations/us-central1/publishers/google/models/gemini-2.5-pro" + action
					if r.URL.Path != want || (r.URL.Query().Get("alt") == "sse") != stream {
						t.Errorf("Vertex Gemini target = %s, want %s (stream %t)", r.URL.String(), want, stream)
					}
					if r.Header.Get("Authorization") != "Bearer fixture-vertex-token" || r.Header.Get("X-Goog-Api-Key") != "" {
						t.Error("Vertex Gemini must use the exchanged bearer token only")
					}
					var body map[string]json.RawMessage
					if json.NewDecoder(r.Body).Decode(&body) != nil || !strings.Contains(string(body["contents"]), "receipt-history") || !strings.Contains(string(body["tools"]), "echo") {
						t.Error("Vertex Gemini lost native contents or tools")
					}
					if body["model"] != nil || body["stream"] != nil {
						t.Error("Vertex Gemini model and stream must remain URL selectors")
					}
					if downstream == "gemini" && !strings.Contains(string(body["generationConfig"]), "9007199254740993") {
						t.Error("native Vertex request lost integer precision")
					}
					fixture := directWireFixtures[3]
					if stream {
						w.Header().Set("Content-Type", "text/event-stream")
						for _, frame := range directNativeSSE(fixture) {
							fmt.Fprint(w, frame)
						}
						return
					}
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, fixture.response)
				}))
				defer upstream.Close()
				endpoint := &store.DirectEndpoint{URL: upstream.URL + "/v1/projects/my-project/locations/us-central1/publishers/google/models", Auth: store.DirectAuthBearer, Profile: "vertex", ModelPath: true}
				db := installDirectMediaRuntime(t, store.DialectSQLite, store.DirectEndpoints{Gemini: endpoint}, store.DirectProtocolGemini, "")
				if _, err := db.Exec(db.Rebind(`UPDATE upstream_models SET name='gemini-2.5-pro'`)); err != nil {
					t.Fatal(err)
				}
				if _, err := db.Exec(db.Rebind(`UPDATE upstream_credentials SET kind=?, secret='', gcp_service_account=?`), store.DirectCredentialGCPServiceAccount, vertexTestSA(t, tokenServer.URL)); err != nil {
					t.Fatal(err)
				}
				body, path := directWireFixtures[3].request, directWireFixtures[3].path
				body = strings.TrimSuffix(body, "}") + `,"generationConfig":{"maxOutputTokens":8,"seed":9007199254740993}}`
				if downstream == "chat" {
					body, path = directWireFixtures[0].request, directWireFixtures[0].path
					if stream {
						body = strings.TrimSuffix(body, "}") + `,"stream":true}`
					}
				} else if stream {
					path = strings.Replace(path, ":generateContent", ":streamGenerateContent", 1)
				}
				recorder := httptest.NewRecorder()
				directMediaRouter().ServeHTTP(recorder, makeProxyReq(http.MethodPost, path, body))
				if recorder.Code != http.StatusOK || calls.Load() != 1 || exchanges.Load() != 1 {
					t.Fatalf("status=%d calls=%d exchanges=%d body=%s", recorder.Code, calls.Load(), exchanges.Load(), recorder.Body.String())
				}
				for _, marker := range []string{"receipt-text", "receipt-tool"} {
					if !strings.Contains(recorder.Body.String(), marker) {
						t.Errorf("Vertex response lost %s: %s", marker, recorder.Body.String())
					}
				}
			})
		}
	}
}
