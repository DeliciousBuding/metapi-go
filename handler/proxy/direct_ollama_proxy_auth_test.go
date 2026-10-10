package proxyhandler

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestDirectOllamaProxyCannotRestoreCustomCredentials(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, anonymous := range []bool{false, true} {
			t.Run(fmt.Sprintf("stream=%t/anonymous=%t", stream, anonymous), func(t *testing.T) {
				var calls atomic.Int64
				proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					want := "Bearer fixture-media-key"
					if anonymous {
						want = ""
					}
					if r.Header.Get("Authorization") != want || r.Header.Get("X-API-Key") != "" || r.Header.Get("X-Goog-API-Key") != "" {
						t.Error("proxy transport restored an unselected credential")
					}
					if r.Header.Get("X-Trace") != "fixture-trace" {
						t.Error("non-authentication custom header lost")
					}
					if stream {
						w.Header().Set("Content-Type", "application/x-ndjson")
					} else {
						w.Header().Set("Content-Type", "application/json")
					}
					_, _ = io.WriteString(w, `{"model":"provider-model","done":true,"message":{"role":"assistant","content":"hello"},"prompt_eval_count":2,"eval_count":1}`+"\n")
				}))
				defer proxy.Close()
				db := installOllamaHTTPRuntime(t, store.DialectSQLite, "http://upstream.example", anonymous)
				if _, err := db.Exec(`UPDATE upstream_channels SET channel_proxy=?,custom_header=?`, proxy.URL, `[{"header_key":"x-api-key","header_value":"fixture-unselected"},{"header_key":"X-Goog-Api-Key","header_value":"fixture-unselected"},{"header_key":"X-Trace","header_value":"fixture-trace"}]`); err != nil {
					t.Fatal(err)
				}
				routing.InvalidateCache()
				out := httptest.NewRecorder()
				directMediaRouter().ServeHTTP(out, makeProxyReq("POST", "/v1/chat/completions", fmt.Sprintf(`{"model":"client-alias","messages":[{"role":"user","content":"hello"}],"stream":%t}`, stream)))
				if out.Code != 200 || calls.Load() != 1 {
					t.Fatalf("proxy request failed: %d calls=%d %s", out.Code, calls.Load(), out.Body.String())
				}
			})
		}
	}
}
