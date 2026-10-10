package backup_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/deliciousbuding/metapi-go/auth"
)

func requestAxonHubNativeHTTP(router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for _, header := range []string{"Authorization", "x-api-key", "x-goog-api-key"} {
		req.Header.Set(header, "fixture-downstream-secret")
	}
	req = req.WithContext(auth.WithProxyAuth(req.Context(), &auth.ProxyAuthContext{Source: "global", Token: "fixture-downstream-key"}))
	out := httptest.NewRecorder()
	router.ServeHTTP(out, req)
	return out
}

func TestAxonHubNativeImportHTTPJSON(t *testing.T) {
	for _, tc := range []struct {
		name, provider, format, custom, requestPath, upstreamPath string
		oauth                                                     bool
	}{
		{"System One", "typesafe", "", "", "/v1/systemone", "/prefix/v1/systemone", false},
		{"System One custom", "typesafe", "typesafe/systemone", "/ask", "/v1/systemone", "/prefix/ask", false},
		{"Alpha Search", "openai", "openai/alpha_search", "/search", "/v1/alpha/search", "/prefix/search", false},
		{"Codex Alpha Search OAuth", "codex", "openai/alpha_search", "/search", "/v1/alpha/search", "/prefix/search", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var body map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if r.URL.Path != tc.upstreamPath || r.Header.Get("Authorization") != "Bearer fixture-media-key" || r.Header.Get("x-api-key") != "" || r.Header.Get("x-goog-api-key") != "" || string(body["model"]) != `"provider-model"` || string(body["opaque"]) != "9007199254740993" {
					t.Errorf("native JSON wire mismatch path=%s body=%s", r.URL.Path, body)
				}
				if _, found := body["stream"]; found {
					t.Error("native JSON acquired stream field")
				}
				if tc.oauth && r.Header.Get("Session_id") == "" && r.Header.Get("Session-Id") == "" {
					t.Error("Codex Alpha Search lost session metadata")
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"model":"provider-model","answers":{"check":{"type":"noul","noul":0.945}},"receipt":9007199254740993}`)
			}))
			defer upstream.Close()
			configure := func(ch map[string]any) {
				if tc.oauth {
					ch["credentials"] = map[string]any{"oauth": map[string]any{"access_token": "fixture-media-key", "expires_at": "2099-01-01T00:00:00Z"}}
				}
			}
			router := installAxonHubHTTPFixture(t, tc.provider, upstream.URL+"/prefix", tc.format, tc.custom, "chat", configure)
			body := `{"model":"client-alias","state":{"large_int":9007199254740993},"questions":{"check":{"type":"noul","instructions":"True?"}},"id":"fixture-session","opaque":9007199254740993}`
			out := requestAxonHubNativeHTTP(router, http.MethodPost, tc.requestPath, body)
			if out.Code != 200 || calls.Load() != 1 || !strings.Contains(out.Body.String(), `"receipt":9007199254740993`) {
				t.Fatalf("native JSON status=%d calls=%d body=%s", out.Code, calls.Load(), out.Body.String())
			}
			out = requestAxonHubNativeHTTP(router, http.MethodPost, tc.requestPath, strings.TrimSuffix(body, "}")+`,"stream":true}`)
			if out.Code != 400 || calls.Load() != 1 {
				t.Fatalf("streaming native JSON was not rejected before I/O: status=%d calls=%d", out.Code, calls.Load())
			}
			out = requestAxonHubNativeHTTP(router, http.MethodPost, "/v1/chat/completions", `{"model":"client-alias","messages":[{"role":"user","content":"hello"}]}`)
			if out.Code < 400 || calls.Load() != 1 {
				t.Fatalf("native JSON grant authorized conversation traffic: status=%d calls=%d", out.Code, calls.Load())
			}
		})
	}
}

func TestAxonHubNativeImportHTTPBedrock(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if r.URL.Path != "/region/model/provider-model/invoke" || r.Header.Get("Authorization") != "Bearer fixture-media-key" || r.Header.Get("Anthropic-Version") != "bedrock-2023-05-31" || r.Header.Get("x-api-key") != "" || string(body["anthropic_version"]) != `"bedrock-2023-05-31"` {
			t.Errorf("Bedrock wire mismatch path=%s body=%s", r.URL.Path, body)
		}
		if len(body["model"]) != 0 || len(body["stream"]) != 0 {
			t.Error("Bedrock body retained model/stream")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"message-fixture","type":"message","role":"assistant","model":"provider-model","content":[{"type":"text","text":"native-receipt"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":2}}`)
	}))
	defer upstream.Close()
	router := installAxonHubMediaHTTP(t, "anthropic_aws", upstream.URL+"/region", "", "", "chat")
	out := requestAxonHubNativeHTTP(router, http.MethodPost, "/v1/messages", `{"model":"client-alias","max_tokens":20,"messages":[{"role":"user","content":"hello"}]}`)
	if out.Code != 200 || calls.Load() != 1 || !strings.Contains(out.Body.String(), "native-receipt") {
		t.Fatalf("Bedrock status=%d calls=%d body=%s", out.Code, calls.Load(), out.Body.String())
	}
}

func TestAxonHubNativeImportHTTPMessagesAuthentication(t *testing.T) {
	for _, tc := range []struct {
		name, provider, format, path, wantAuth string
		noAuth                                 bool
	}{
		{"Ollama Bearer", "ollama_anthropic", "", "/v1/messages", "Authorization", false},
		{"Ollama anonymous", "ollama_anthropic", "", "/v1/messages", "", true},
		{"Ollama custom direct", "ollama_anthropic", "anthropic/messages", "/custom", "x-api-key", false},
		{"Bedrock custom direct", "anthropic_aws", "anthropic/messages", "/custom", "x-api-key", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var body map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if r.URL.Path != tc.path || string(body["model"]) != `"provider-model"` || len(body["anthropic_version"]) != 0 {
					t.Errorf("Messages request changed path=%s body=%s", r.URL.Path, body)
				}
				for _, header := range []string{"Authorization", "x-api-key", "x-goog-api-key"} {
					want := ""
					if header == tc.wantAuth {
						want = "fixture-media-key"
						if header == "Authorization" {
							want = "Bearer " + want
						}
					}
					if r.Header.Get(header) != want {
						t.Errorf("Messages credential header %s was not selected correctly", header)
					}
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"id":"message-fixture","type":"message","role":"assistant","model":"provider-model","content":[{"type":"text","text":"native-receipt"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":2}}`)
			}))
			defer upstream.Close()
			custom := ""
			if tc.format != "" {
				custom = tc.path
			}
			router := installAxonHubHTTPFixture(t, tc.provider, upstream.URL, tc.format, custom, "chat", func(ch map[string]any) {
				if tc.noAuth {
					ch["credentials"] = map[string]any{}
				}
			})
			out := requestAxonHubNativeHTTP(router, http.MethodPost, "/v1/messages", `{"model":"client-alias","max_tokens":20,"messages":[{"role":"user","content":"hello"}]}`)
			if out.Code != 200 || calls.Load() != 1 || !strings.Contains(out.Body.String(), "native-receipt") {
				t.Fatalf("Messages status=%d calls=%d body=%s", out.Code, calls.Load(), out.Body.String())
			}
		})
	}
}

func TestAxonHubNativeImportHTTPOllama(t *testing.T) {
	for _, noAuth := range []bool{false, true} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("none=%t/stream=%t", noAuth, stream), func(t *testing.T) {
				var calls atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					var body map[string]json.RawMessage
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					wantAuth := "Bearer fixture-media-key"
					if noAuth {
						wantAuth = ""
					}
					if r.URL.Path != "/api/chat" || r.Header.Get("Authorization") != wantAuth || r.Header.Get("x-api-key") != "" || r.Header.Get("x-goog-api-key") != "" || string(body["model"]) != `"provider-model"` || string(body["stream"]) != fmt.Sprint(stream) {
						t.Errorf("Ollama wire mismatch path=%s body=%s", r.URL.Path, body)
					}
					if stream {
						w.Header().Set("Content-Type", "application/x-ndjson")
						_, _ = io.WriteString(w, "{\"model\":\"provider-model\",\"message\":{\"role\":\"assistant\",\"content\":\"native-receipt\"},\"done\":false}\n{\"model\":\"provider-model\",\"message\":{\"role\":\"assistant\",\"content\":\"\"},\"done\":true,\"done_reason\":\"stop\",\"prompt_eval_count\":3,\"eval_count\":2}\n")
					} else {
						w.Header().Set("Content-Type", "application/json")
						_, _ = io.WriteString(w, `{"model":"provider-model","message":{"role":"assistant","content":"native-receipt"},"done":true,"done_reason":"stop","prompt_eval_count":3,"eval_count":2}`)
					}
				}))
				defer upstream.Close()
				router := installAxonHubHTTPFixture(t, "ollama", upstream.URL, "", "", "chat", func(ch map[string]any) {
					if noAuth {
						ch["credentials"] = map[string]any{}
					}
				})
				out := requestAxonHubNativeHTTP(router, http.MethodPost, "/v1/chat/completions", fmt.Sprintf(`{"model":"client-alias","messages":[{"role":"user","content":"hello"}],"stream":%t}`, stream))
				if out.Code != 200 || calls.Load() != 1 || !strings.Contains(out.Body.String(), "native-receipt") {
					t.Fatalf("Ollama status=%d calls=%d body=%s", out.Code, calls.Load(), out.Body.String())
				}
				if stream && !strings.Contains(out.Body.String(), "[DONE]") {
					t.Fatal("Ollama stream did not terminate")
				}
			})
		}
	}
}

func TestAxonHubNativeImportHTTPOllamaClientFormats(t *testing.T) {
	for _, tc := range []struct{ name, path, body string }{
		{"Responses", "/v1/responses", `{"model":"client-alias","input":"hello"}`},
		{"Messages", "/v1/messages", `{"model":"client-alias","max_tokens":20,"messages":[{"role":"user","content":"hello"}]}`},
		{"Gemini", "/v1beta/models/client-alias:generateContent", `{"contents":[{"role":"user","parts":[{"text":"hello"}]}]}`},
	} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", tc.name, stream), func(t *testing.T) {
				var calls atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					var body map[string]json.RawMessage
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if r.URL.Path != "/api/chat" || string(body["model"]) != `"provider-model"` || string(body["stream"]) != fmt.Sprint(stream) || !strings.Contains(string(body["messages"]), "hello") {
						t.Errorf("Ollama conversion path=%s body=%s", r.URL.Path, body)
					}
					for _, header := range []string{"Authorization", "x-api-key", "x-goog-api-key"} {
						if r.Header.Get(header) != "" {
							t.Errorf("Ollama conversion leaked %s", header)
						}
					}
					if stream {
						w.Header().Set("Content-Type", "application/x-ndjson")
						_, _ = io.WriteString(w, "{\"model\":\"provider-model\",\"message\":{\"role\":\"assistant\",\"content\":\"native-receipt\"},\"done\":false}\n{\"model\":\"provider-model\",\"message\":{\"role\":\"assistant\",\"content\":\"\"},\"done\":true,\"done_reason\":\"stop\",\"prompt_eval_count\":3,\"eval_count\":2}\n")
					} else {
						w.Header().Set("Content-Type", "application/json")
						_, _ = io.WriteString(w, `{"model":"provider-model","message":{"role":"assistant","content":"native-receipt"},"done":true,"done_reason":"stop","prompt_eval_count":3,"eval_count":2}`)
					}
				}))
				defer upstream.Close()
				router := installAxonHubHTTPFixture(t, "ollama", upstream.URL, "", "", "chat", func(ch map[string]any) { ch["credentials"] = map[string]any{} })
				path, body := tc.path, tc.body
				if stream {
					if tc.name == "Gemini" {
						path = strings.Replace(path, ":generateContent", ":streamGenerateContent", 1)
					} else {
						body = strings.TrimSuffix(body, "}") + `,"stream":true}`
					}
				}
				out := requestAxonHubNativeHTTP(router, http.MethodPost, path, body)
				if out.Code != 200 || calls.Load() != 1 || !strings.Contains(out.Body.String(), "native-receipt") {
					t.Fatalf("Ollama %s status=%d calls=%d body=%s", tc.name, out.Code, calls.Load(), out.Body.String())
				}
			})
		}
	}
}

func TestAxonHubNativeImportHTTPVideo(t *testing.T) {
	for _, tc := range []struct {
		provider, path string
		canDelete      bool
	}{{"doubao", "/v3/contents/generations/tasks", true}, {"zenmux_video", "/v1/videos", false}} {
		t.Run(tc.provider, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Header.Get("Authorization") != "Bearer fixture-media-key" || r.Header.Get("x-api-key") != "" {
					t.Error("native video authentication changed")
				}
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodPost {
					var body map[string]json.RawMessage
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if r.URL.Path != tc.path || string(body["model"]) != `"provider-model"` || len(body["content"]) == 0 || len(body["prompt"]) != 0 {
						t.Errorf("native video creation path=%s body=%s", r.URL.Path, body)
					}
					_, _ = io.WriteString(w, `{"id":"fixture-task","status":"queued"}`)
				} else {
					if r.URL.Path != tc.path+"/fixture-task" {
						t.Errorf("native video task path=%s", r.URL.Path)
					}
					if r.Method == http.MethodDelete {
						_, _ = io.WriteString(w, `{}`)
					} else {
						_, _ = io.WriteString(w, `{"id":"fixture-task","status":"running"}`)
					}
				}
			}))
			defer upstream.Close()
			router := installAxonHubMediaHTTP(t, tc.provider, upstream.URL, "", "", "video_generation")
			out := requestAxonHubNativeHTTP(router, http.MethodPost, "/v1/videos", `{"model":"client-alias","prompt":"hello"}`)
			var created struct {
				ID string `json:"id"`
			}
			_ = json.Unmarshal(out.Body.Bytes(), &created)
			if out.Code != 200 || created.ID == "" || created.ID == "fixture-task" || calls.Load() != 1 {
				t.Fatalf("native video create status=%d calls=%d body=%s", out.Code, calls.Load(), out.Body.String())
			}
			out = requestAxonHubNativeHTTP(router, http.MethodGet, "/v1/videos/"+created.ID, "")
			if out.Code != 200 || calls.Load() != 2 || !strings.Contains(out.Body.String(), `"status":"in_progress"`) {
				t.Fatalf("native video get status=%d calls=%d body=%s", out.Code, calls.Load(), out.Body.String())
			}
			out = requestAxonHubNativeHTTP(router, http.MethodPost, "/v1/videos/"+created.ID+"/remix", `{"prompt":"next"}`)
			if out.Code < 400 || calls.Load() != 2 {
				t.Fatalf("native remix reached upstream: status=%d calls=%d", out.Code, calls.Load())
			}
			out = requestAxonHubNativeHTTP(router, http.MethodDelete, "/v1/videos/"+created.ID, "")
			if tc.canDelete {
				if out.Code != 200 || calls.Load() != 3 {
					t.Fatalf("Seedance delete status=%d calls=%d body=%s", out.Code, calls.Load(), out.Body.String())
				}
			} else if out.Code < 400 || calls.Load() != 2 {
				t.Fatalf("ZenMux deletion reached upstream: status=%d calls=%d", out.Code, calls.Load())
			}
		})
	}
}
