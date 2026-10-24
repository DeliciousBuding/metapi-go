package backup_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestAxonHubInactiveRateLimitHTTP(t *testing.T) {
	for _, stream := range []bool{false, true} {
		name := "json"
		if stream {
			name = "stream"
		}
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var body struct {
					Model  string `json:"model"`
					Stream bool   `json:"stream"`
					Tools  []struct {
						Function struct {
							Name string `json:"name"`
						} `json:"function"`
					} `json:"tools"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer fixture-media-key" || body.Model != "provider-model" || body.Stream != stream || len(body.Tools) != 1 || body.Tools[0].Function.Name != "echo" {
					t.Errorf("imported request changed path/auth/model/stream/tools: path=%s body=%+v", r.URL.Path, body)
				}
				if stream {
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = io.WriteString(w, "data: {\"id\":\"fixture\",\"model\":\"provider-model\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"rate-limit-ok\"},\"finish_reason\":null}]}\n\ndata: {\"id\":\"fixture\",\"model\":\"provider-model\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":1,\"total_tokens\":3}}\n\ndata: [DONE]\n\n")
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"id":"fixture","model":"provider-model","choices":[{"index":0,"message":{"role":"assistant","content":"rate-limit-ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`)
			}))
			defer upstream.Close()
			router := installAxonHubHTTPFixture(t, "openai", upstream.URL, "", "", "chat", func(channel map[string]any) {
				channel["settings"] = map[string]any{"rateLimit": map[string]any{"rpm": 0, "tpm": nil, "maxConcurrent": -1, "queueSize": 10, "queueTimeoutMs": 1000}}
			})
			payload := `{"model":"client-alias","messages":[{"role":"user","content":"hello"}],"tools":[{"type":"function","function":{"name":"echo","parameters":{"type":"object"}}}],"stream":false}`
			if stream {
				payload = strings.Replace(payload, `"stream":false`, `"stream":true`, 1)
			}
			response := requestAxonHubMediaHTTP(router, "/v1/chat/completions", "application/json", strings.NewReader(payload))
			if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "rate-limit-ok") || !strings.Contains(response.Body.String(), `"total_tokens":3`) || calls.Load() != 1 {
				t.Fatalf("imported relay status=%d calls=%d body=%s", response.Code, calls.Load(), response.Body.String())
			}
			if stream && !strings.Contains(response.Body.String(), "[DONE]") {
				t.Fatal("imported stream lost completion marker")
			}
		})
	}
}
