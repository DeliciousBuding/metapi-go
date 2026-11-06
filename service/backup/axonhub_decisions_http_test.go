package backup_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestAxonHubDecisionsImportNativeHTTP(t *testing.T) {
	for _, tc := range []struct{ provider, base, custom, target string }{
		{"openai", "/prefix", "", "/prefix/v1/decisions"},
		{"openai", "/prefix#", "", "/prefix/decisions"},
		{"openai", "/prefix", "/custom", "/prefix/custom"},
		{"openai", "/exact/##", "/ignored", "/exact"},
		{"qiniu", "/prefix", "", "/prefix/v1/decisions"},
		{"qiniu", "/prefix#", "", "/prefix/decisions"},
		{"qiniu", "/prefix", "/custom", "/prefix/custom"},
		{"qiniu", "/exact/##", "/ignored", "/exact"},
	} {
		t.Run(tc.provider+tc.target, func(t *testing.T) {
			var calls atomic.Int32
			response := `{"model":"provider-model","answers":[{"choice":true,"opaque":9007199254740993}],"usage":{"input_tokens":7}}`
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				raw, _ := io.ReadAll(r.Body)
				if r.URL.Path != tc.target || r.Header.Get("Authorization") != "Bearer fixture-media-key" || r.Header.Get("x-api-key") != "" ||
					!strings.Contains(string(raw), `"model":"provider-model"`) || !strings.Contains(string(raw), "9007199254740993") || strings.Contains(string(raw), `"messages"`) ||
					(tc.provider == "qiniu" && (r.Header.Get("X-Decisions-Model") != "provider-model" || !strings.Contains(string(raw), `"copied":"client-alias"`) || strings.Contains(string(raw), `"alias"`))) {
					t.Errorf("imported Decisions changed URL/auth/native body: %s %s", r.URL.Path, raw)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, response)
			}))
			t.Cleanup(upstream.Close)
			router := installAxonHubHTTPFixture(t, tc.provider, upstream.URL+tc.base, "openai/decisions", tc.custom, "chat", func(channel map[string]any) {
				if tc.provider != "qiniu" {
					return
				}
				settings := channel["settings"].(map[string]any)
				settings["bodyOverrideOperations"] = []any{
					map[string]any{"op": "set", "path": "metadata.alias", "value": "{{.RequestModel}}"},
					map[string]any{"op": "copy", "from": "metadata.alias", "to": "metadata.copied"},
					map[string]any{"op": "delete", "path": "metadata.alias"},
				}
				settings["headerOverrideOperations"] = []any{map[string]any{"op": "set", "path": "X-Decisions-Model", "value": "{{.Model}}"}}
			})
			body := `{"model":"client-alias","input":"fixture","questions":[{"extension":9007199254740993}]}`
			out := requestAxonHubNativeHTTP(router, http.MethodPost, "/v1/decisions", body)
			if out.Code != 200 || calls.Load() != 1 || out.Body.String() != response {
				t.Fatalf("imported Decisions failed: %d %s", out.Code, out.Body.String())
			}
			out = requestAxonHubNativeHTTP(router, http.MethodPost, "/v1/chat/completions", `{"model":"client-alias","messages":[{"role":"user","content":"fixture"}]}`)
			if out.Code < 400 || calls.Load() != 1 {
				t.Fatal("model's Decisions format restriction widened to Chat")
			}
		})
	}
}
