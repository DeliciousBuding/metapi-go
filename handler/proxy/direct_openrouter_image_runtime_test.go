package proxyhandler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestDirectOpenRouterImageRuntime(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		for _, edit := range []bool{false, true} {
			t.Run(dialect+map[bool]string{false: "/generate", true: "/edit"}[edit], func(t *testing.T) {
				var referenceDownloads atomic.Int32
				reference := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					referenceDownloads.Add(1)
					t.Error("gateway must not download remote reference images")
				}))
				defer reference.Close()
				referenceURL := reference.URL + "/ref.png?signature=a%2Fb%2Bcd&expires=123"
				var calls atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					if r.Method != "POST" || r.URL.Path != "/api/v1/images" || r.Header.Get("Authorization") != "Bearer fixture-media-key" || r.Header.Get("X-Api-Key") != "" {
						t.Errorf("wrong native destination/auth: %s %s", r.Method, r.URL)
					}
					var body map[string]json.RawMessage
					_ = json.NewDecoder(r.Body).Decode(&body)
					if mediaString(body, "model") != "provider-model" || mediaString(body, "quality") != "high" || string(body["seed"]) != "9007199254740993" || len(body["input_references"]) == 0 || len(body["image"]) != 0 || len(body["response_format"]) != 0 {
						t.Errorf("wrong native body: %v", body)
					}
					if !edit {
						var refs []struct {
							ImageURL struct {
								URL string `json:"url"`
							} `json:"image_url"`
						}
						if json.Unmarshal(body["input_references"], &refs) != nil || len(refs) != 1 || refs[0].ImageURL.URL != referenceURL {
							t.Errorf("remote signed reference not preserved: %s", body["input_references"])
						}
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"created":123,"data":[{"b64_json":"iVBORw0KGgo=","media_type":"image/png"}],"usage":{"prompt_tokens":4,"completion_tokens":24,"total_tokens":28}}`)
				}))
				defer upstream.Close()
				endpoint := &store.DirectEndpoint{URL: upstream.URL + "/api/v1/images", Auth: store.DirectAuthBearer, Profile: "openrouter-image"}
				endpoints := store.DirectEndpoints{ImageGeneration: endpoint}
				bit := store.DirectProtocolImageGeneration
				path := "/v1/images/generations"
				if edit {
					endpoints = store.DirectEndpoints{ImageEdit: endpoint}
					bit = store.DirectProtocolImageEdit
					path = "/v1/images/edits"
				}
				db := installDirectMediaRuntime(t, dialect, endpoints, bit, `{"quality":"high","seed":9007199254740993}`)
				var mu sync.Mutex
				var logs []proxy.ProxyLogEntry
				getUpstreamConfig().LogProxy = func(_ context.Context, entry proxy.ProxyLogEntry) error {
					mu.Lock()
					defer mu.Unlock()
					logs = append(logs, entry)
					return nil
				}
				gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					r = r.WithContext(auth.WithProxyAuth(r.Context(), &auth.ProxyAuthContext{Source: "global", Token: "fixture-client"}))
					directMediaRouter().ServeHTTP(w, r)
				}))
				defer gateway.Close()
				for _, format := range []string{"b64_json", "url"} {
					var body io.Reader
					ct := "application/json"
					if edit {
						var raw bytes.Buffer
						writer := multipart.NewWriter(&raw)
						_ = writer.WriteField("model", "client-alias")
						_ = writer.WriteField("prompt", "cat")
						_ = writer.WriteField("response_format", format)
						file, _ := writer.CreateFormFile("image", "ref.png")
						_, _ = file.Write([]byte{137, 80, 78, 71, 13, 10, 26, 10})
						_ = writer.Close()
						ct = writer.FormDataContentType()
						body = &raw
					} else {
						body = strings.NewReader(`{"model":"client-alias","prompt":"cat","images":[{"image_url":"` + referenceURL + `"}],"response_format":"` + format + `"}`)
					}
					req, _ := http.NewRequest("POST", gateway.URL+path, body)
					req.Header.Set("Content-Type", ct)
					req.Header.Set("X-Api-Key", "untrusted-client-key")
					resp, err := gateway.Client().Do(req)
					if err != nil {
						t.Fatal(err)
					}
					data, _ := io.ReadAll(resp.Body)
					_ = resp.Body.Close()
					if resp.StatusCode != 200 || !bytes.Contains(data, []byte(`"input_tokens":4`)) || (bytes.Contains(data, []byte(`"b64_json"`))) != (format == "b64_json") {
						t.Fatalf("runtime response: %d %s", resp.StatusCode, data)
					}
				}
				otherPath := "/v1/images/edits"
				if edit {
					otherPath = "/v1/images/generations"
				}
				other := httptest.NewRecorder()
				directMediaRouter().ServeHTTP(other, makeProxyReq("POST", otherPath, `{"model":"client-alias","prompt":"p","image":"`+openRouterImageFixture+`"}`))
				if other.Code < 400 || calls.Load() != 2 {
					t.Fatal("shared native image wire widened generation/edit grants")
				}
				mu.Lock()
				defer mu.Unlock()
				if calls.Load() != 2 || len(logs) != 2 || referenceDownloads.Load() != 0 {
					t.Fatalf("attempt/log count: %d/%d", calls.Load(), len(logs))
				}
				var expectedCost float64
				for _, entry := range logs {
					if entry.TotalTokens == nil || *entry.TotalTokens != 28 || entry.PromptTokens == nil || *entry.PromptTokens != 4 || entry.CompletionTokens == nil || *entry.CompletionTokens != 24 || entry.EstimatedCost <= 0 || entry.UsageSource != "upstream" || entry.Status != "success" {
						t.Fatalf("image usage did not reach standard accounting: %+v", entry)
					}
					expectedCost += entry.EstimatedCost
				}
				var persistedCost float64
				if err := db.QueryRow(`SELECT SUM(total_cost) FROM upstream_grants`).Scan(&persistedCost); err != nil || math.Abs(persistedCost-expectedCost) > 1e-12 {
					t.Fatalf("image usage counted inconsistently: %g/%g err=%v", persistedCost, expectedCost, err)
				}
			})
		}
	}
}

func TestDirectOpenRouterImageRuntimeFailures(t *testing.T) {
	for _, tc := range []struct {
		name         string
		body         string
		status, want int
		tokens       int64
	}{
		{"application", `{"error":{"message":"provider failed"},"usage":{"total_tokens":7}}`, 200, 502, 7},
		{"invalid-image", `{"data":[{"b64_json":"invalid?"}],"usage":{"total_tokens":7}}`, 200, 502, 7},
		{"http-error", `{"error":{"message":"limited"},"usage":{"total_tokens":7}}`, 429, 429, 7},
		{"http-invalid-usage", `{"error":{"message":"limited"},"usage":{"total_tokens":2.5}}`, 429, 429, 0},
		{"invalid-usage", `{"data":[{"b64_json":"aGk="}],"usage":{"total_tokens":2.5}}`, 200, 502, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer upstream.Close()
			db := installDirectMediaRuntime(t, store.DialectSQLite, store.DirectEndpoints{ImageGeneration: &store.DirectEndpoint{URL: upstream.URL + "/images", Auth: store.DirectAuthBearer, Profile: "openrouter-image"}}, store.DirectProtocolImageGeneration, "")
			// Two eligible members ensure a generic retry cannot silently submit
			// the already-executed image request to another credential.
			for _, query := range []string{
				`INSERT INTO upstream_channels (id,origin_key,source_id,name,dialect,provider,enabled,base_url,endpoint_config,openai_chat_completion_path,openai_response_path,anthropic_message_path,proxy,channel_proxy,custom_header,param_override,match_regex) SELECT id+1000,origin_key,source_id+1000,name||' second',dialect,provider,enabled,base_url,endpoint_config,openai_chat_completion_path,openai_response_path,anthropic_message_path,proxy,channel_proxy,custom_header,param_override,match_regex FROM upstream_channels`,
				`INSERT INTO upstream_credentials (id,origin_key,channel_id,source_id,name,secret,enabled) SELECT id+1000,origin_key,channel_id+1000,source_id+1000,name,secret,enabled FROM upstream_credentials`,
				`INSERT INTO upstream_models (id,origin_key,channel_id,source_id,name,enabled) SELECT id+1000,origin_key,channel_id+1000,source_id+1000,name,enabled FROM upstream_models`,
				`INSERT INTO upstream_grants (id,origin_key,source_id,model_id,credential_id,protocols,enabled) SELECT id+1000,origin_key,source_id+1000,model_id+1000,credential_id+1000,protocols,enabled FROM upstream_grants`,
				`INSERT INTO upstream_group_items (origin_key,group_id,source_id,grant_id,priority,weight,protocol_order) SELECT origin_key,group_id,source_id+1000,grant_id+1000,priority,weight,protocol_order FROM upstream_group_items`,
			} {
				if _, err := db.Exec(query); err != nil {
					t.Fatal(err)
				}
			}
			previous := config.Get()
			copy := *previous
			copy.ProxyMaxChannelAttempts = 4
			config.Set(&copy)
			defer config.Set(previous)
			var logs []proxy.ProxyLogEntry
			getUpstreamConfig().LogProxy = func(_ context.Context, entry proxy.ProxyLogEntry) error { logs = append(logs, entry); return nil }
			out := httptest.NewRecorder()
			directMediaRouter().ServeHTTP(out, makeProxyReq("POST", "/v1/images/generations", `{"model":"client-alias","prompt":"p"}`))
			if out.Code != tc.want || calls.Load() != 1 || len(logs) != 1 {
				t.Fatalf("failure usage lost: status=%d calls=%d logs=%+v", out.Code, calls.Load(), logs)
			}
			if logs[0].Status != "failed" || (tc.tokens > 0 && (logs[0].TotalTokens == nil || *logs[0].TotalTokens != tc.tokens || logs[0].EstimatedCost <= 0)) || (tc.tokens == 0 && ((logs[0].TotalTokens != nil && *logs[0].TotalTokens != 0) || logs[0].EstimatedCost != 0)) {
				t.Fatalf("invalid failure accounting: %+v", logs[0])
			}
		})
	}
}

func TestDirectOpenRouterImageRuntimeCancellation(t *testing.T) {
	started := make(chan struct{})
	upstreamCanceled := make(chan struct{})
	stop := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(started)
		select {
		case <-r.Context().Done():
			close(upstreamCanceled)
		case <-stop:
		}
	}))
	defer upstream.Close()
	defer close(stop)
	db := installDirectMediaRuntime(t, store.DialectSQLite, store.DirectEndpoints{ImageGeneration: &store.DirectEndpoint{URL: upstream.URL + "/images", Auth: store.DirectAuthBearer, Profile: "openrouter-image"}}, store.DirectProtocolImageGeneration, "")
	request := makeProxyReq("POST", "/v1/images/generations", `{"model":"client-alias","prompt":"p"}`)
	ctx, cancel := context.WithCancel(request.Context())
	defer cancel()
	request = request.WithContext(ctx)
	out := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { defer close(done); directMediaRouter().ServeHTTP(out, request) }()
	select {
	case <-started:
		cancel()
	case <-time.After(3 * time.Second):
		t.Fatal("image did not reach upstream")
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("image cancellation did not terminate")
	}
	var failed int
	_ = db.QueryRow(`SELECT SUM(fail_count) FROM upstream_grants`).Scan(&failed)
	if out.Code != 499 || failed != 0 {
		t.Fatalf("client cancellation poisoned image grant: %d failures=%d", out.Code, failed)
	}
	select {
	case <-upstreamCanceled:
	case <-time.After(time.Second):
		t.Fatal("client cancellation did not reach upstream")
	}
}

func TestDirectOpenRouterImageMultipartAmbiguity(t *testing.T) {
	for _, variant := range []string{"multiple-aliases", "text-and-file", "mask"} {
		t.Run(variant, func(t *testing.T) {
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)
			_ = writer.WriteField("model", "m")
			_ = writer.WriteField("prompt", "p")
			if variant == "text-and-file" {
				_ = writer.WriteField("images", `["`+openRouterImageFixture+`"]`)
			}
			keys := []string{"image"}
			if variant == "multiple-aliases" {
				keys = append(keys, "image[]")
			}
			if variant == "mask" {
				keys = append(keys, "mask")
			}
			for _, key := range keys {
				part, _ := writer.CreateFormFile(key, "ref.png")
				_, _ = part.Write([]byte{137, 80, 78, 71, 13, 10, 26, 10})
			}
			_ = writer.Close()
			_, _, err := prepareDirectOpenRouterImage("/v1/images/edits", writer.FormDataContentType(), body.Bytes())
			if err == nil || strings.Contains(err.Error(), "ModelScope") {
				t.Fatalf("ambiguous OpenRouter input was ignored or mislabeled: %v", err)
			}
		})
	}
}
