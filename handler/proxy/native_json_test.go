package proxyhandler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

func TestNativeJSONProtocolBoundaries(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			for _, protocol := range []int{store.DirectProtocolSystemOne, store.DirectProtocolAlphaSearch} {
				path, other := "/v1/systemone", "/v1/alpha/search"
				if protocol == store.DirectProtocolAlphaSearch {
					path, other = other, path
				}
				t.Run(path, func(t *testing.T) {
					var calls atomic.Int64
					var response atomic.Value
					response.Store(`{"answers":{"result":9007199254740993},"vendor":"preserved"}`)
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						body, _ := io.ReadAll(r.Body)
						if r.URL.Path != "/custom/native" || r.Header.Get("Authorization") != "Bearer fixture-media-key" || !strings.Contains(string(body), `"model":"provider-model"`) || !strings.Contains(string(body), "9007199254740993") {
							t.Error("native request lost its URL, credential, model mapping or raw numeric precision")
						}
						// A mislabelled JSON body remains valid; SSE/plain invalid bytes fail.
						w.Header().Set("Content-Type", "text/plain")
						_, _ = io.WriteString(w, response.Load().(string))
					}))
					defer server.Close()
					endpoint := &store.DirectEndpoint{URL: server.URL + "/custom/native", Auth: store.DirectAuthBearer}
					endpoints := store.DirectEndpoints{SystemOne: endpoint}
					if protocol == store.DirectProtocolAlphaSearch {
						endpoints = store.DirectEndpoints{AlphaSearch: endpoint}
					}
					installDirectMediaRuntime(t, dialect, endpoints, protocol, "")
					for _, input := range []string{
						`{"model":"client-alias","stream":true}`, `{"model":"client-alias","stream":"false"}`, `{"model":"client-alias","stream":null}`, `{"model":"client-alias","stream":{}}`, `{"state":{}}`,
					} {
						out := httptest.NewRecorder()
						directMediaRouter().ServeHTTP(out, makeProxyReq("POST", path, input))
						if out.Code != 400 || calls.Load() != 0 {
							t.Fatalf("invalid native request made upstream I/O: %d %s", out.Code, out.Body.String())
						}
					}
					body := `{"model":"client-alias","state":{"id":9007199254740993},"questions":{},"vendor":{"unknown":true},"stream":false}`
					for _, target := range []string{other, "/v1/chat/completions"} {
						out := httptest.NewRecorder()
						directMediaRouter().ServeHTTP(out, makeProxyReq("POST", target, body))
						if out.Code < 400 || calls.Load() != 0 {
							t.Fatalf("native grant authorized unrelated endpoint %s", target)
						}
					}
					out := httptest.NewRecorder()
					directMediaRouter().ServeHTTP(out, makeProxyReq("POST", path, body))
					if out.Code != 200 || calls.Load() != 1 || !strings.Contains(out.Body.String(), "9007199254740993") {
						t.Fatalf("native JSON relay failed: %d %s", out.Code, out.Body.String())
					}
					response.Store("data: {\"ok\":true}\n\n")
					out = httptest.NewRecorder()
					directMediaRouter().ServeHTTP(out, makeProxyReq("POST", path, body))
					if out.Code != 502 || strings.Contains(out.Body.String(), "data:") {
						t.Fatalf("native JSON accepted a non-JSON response: %d %s", out.Code, out.Body.String())
					}
				})
			}
		})
	}
}
