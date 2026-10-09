package proxyhandler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/transform/anthropic/messages"
)

const geminiPreciseSchemaRequest = `{"contents":[{"role":"user","parts":[{"text":"use the schema exactly"}]}],"tools":[{"functionDeclarations":[{"name":"echo","parametersJsonSchema":{"type":"object","properties":{"id":{"type":"integer","minimum":9007199254740993,"enum":[9007199254740995]},"ratio":{"type":"number","minimum":0.12345678901234567890123456789,"enum":[0.98765432109876543210987654321]}}}}]}]}`

func assertPreciseSchema(t *testing.T, body []byte) {
	t.Helper()
	for _, number := range []string{"9007199254740993", "9007199254740995", "0.12345678901234567890123456789", "0.98765432109876543210987654321"} {
		if !strings.Contains(string(body), number) {
			t.Errorf("schema number %s changed: %s", number, body)
		}
	}
}

func TestDirectGeminiSchemaPrecisionAcrossProtocols(t *testing.T) {
	for _, path := range []string{"/v1/chat/completions", "/v1/responses", "/v1/messages"} {
		for _, stream := range []bool{false, true} {
			t.Run(path+map[bool]string{false: "_json", true: "_stream"}[stream], func(t *testing.T) {
				body, err := directConvertRequest([]byte(geminiPreciseSchemaRequest), "/v1beta/models/client-alias:generateContent", path, "provider-model", stream, messages.Options{})
				if err != nil {
					t.Fatal(err)
				}
				assertPreciseSchema(t, body)
				var request map[string]json.RawMessage
				if err := json.Unmarshal(body, &request); err != nil {
					t.Fatal(err)
				}
				if string(request["stream"]) != map[bool]string{false: "false", true: "true"}[stream] {
					t.Errorf("stream flag changed: %s", body)
				}
			})
		}
	}
}

func TestDirectGeminiSchemaPrecisionReachesHTTPUpstream(t *testing.T) {
	called := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		assertPreciseSchema(t, body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, directWireFixtures[0].response)
	}))
	defer upstream.Close()
	installAxonHubEndpointFixture(t, "openai", upstream.URL, nil)
	out := httptest.NewRecorder()
	HandleGeminiGenerateContent(out, makeProxyReq(http.MethodPost, "/v1beta/models/client-alias:generateContent", geminiPreciseSchemaRequest))
	if !called || out.Code != http.StatusOK {
		t.Fatalf("HTTP relay failed: called=%v status=%d body=%s", called, out.Code, out.Body.String())
	}
}
