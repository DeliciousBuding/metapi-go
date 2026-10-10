package backup_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	proxyhandler "github.com/deliciousbuding/metapi-go/handler/proxy"
	"github.com/deliciousbuding/metapi-go/internal/pgtest"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/service/backup"
	"github.com/deliciousbuding/metapi-go/service/oauth"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

// These fixtures enter through the real importer and router. No SQL edits to
// endpoints, protocol bits or aliases may repair the imported graph afterward.
func installAxonHubMediaHTTP(t *testing.T, provider, base, format, path, kind string) http.Handler {
	t.Helper()
	return installAxonHubHTTPFixture(t, provider, base, format, path, kind, nil)
}

func installAxonHubHTTPFixture(t *testing.T, provider, base, format, path, kind string, configure func(map[string]any)) http.Handler {
	t.Helper()
	dialect, dsn := store.DialectSQLite, ":memory:"
	if pgDSN := os.Getenv("PG_TEST_DSN"); pgDSN != "" {
		dialect, dsn = store.DialectPostgres, pgDSN
	}
	db, err := store.Open(dialect, dsn, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if dialect == store.DialectPostgres {
		pgtest.Reset(t, db.DB)
	}
	if err := store.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	channel := map[string]any{
		"id": 1, "type": provider, "name": "media HTTP fixture", "base_url": base,
		"credentials":        map[string]any{"apiKey": "fixture-media-key"},
		"supported_models":   []string{"provider-model"},
		"default_test_model": "fixture-main-model",
	}
	if format != "" {
		channel["endpoints"] = []any{map[string]any{"api_format": format, "path": path}}
		channel["settings"] = map[string]any{"modelProtocols": []any{map[string]any{"model": "client-alias", "apiFormats": []string{format}}}}
	}
	if configure != nil {
		configure(channel)
	}
	raw, err := json.Marshal(map[string]any{
		"version": "1.4", "timestamp": "2026-10-10T00:00:00Z", "channels": []any{channel},
		"models": []any{map[string]any{"id": 1, "model_id": "client-alias", "type": kind,
			"settings": map[string]any{"associations": []any{map[string]any{"type": "channel_model", "channelModel": map[string]any{"channelId": 1, "modelId": "provider-model"}}}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	counts, err := backup.ImportAxonHubV14(db, raw, "media-http", false)
	if err != nil || counts["routes"] != 1 {
		t.Fatalf("media fixture import: %v counts=%v", err, counts)
	}
	previousConfig, previousRuntime := config.GetSafe(), config.RuntimeSafe()
	previousDB := store.GetDB()
	store.OverrideDB(db)
	cfg, runtime := config.Load(map[string]string{"AUTH_TOKEN": "fixture-admin-token", "ACCOUNT_CREDENTIAL_SECRET": "fixture-credential-secret"})
	config.Set(cfg)
	config.SetRuntime(runtime)
	t.Cleanup(func() {
		proxyhandler.SetUpstreamConfig(nil)
		config.Set(previousConfig)
		config.SetRuntime(previousRuntime)
		store.OverrideDB(previousDB)
		routing.SetGlobalCache(nil)
	})
	proxyhandler.SetUpstreamConfig(&proxyhandler.UpstreamConfig{
		Router:   routing.NewTokenRouter(service.NewProxyRoutingStore(db), cfg, nil, nil),
		LogProxy: func(context.Context, proxy.ProxyLogEntry) error { return nil },
		ResolveDirectCredential: func(ctx context.Context, id int64, proxyURL *string, force bool) (*oauth.DirectCredentialResult, error) {
			return oauth.ResolveDirectCredential(ctx, db.DB, id, proxyURL, force)
		},
	})
	router := chi.NewRouter()
	router.Route("/v1", proxyhandler.RegisterProxyRoutes)
	proxyhandler.RegisterNonV1ProxyRoutes(router)
	return router
}

func requestAxonHubMediaHTTP(router http.Handler, path, contentType string, body io.Reader) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, body)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", "Bearer fixture-downstream-key")
	req = req.WithContext(auth.WithProxyAuth(req.Context(), &auth.ProxyAuthContext{Source: "global", Token: "fixture-downstream-key"}))
	out := httptest.NewRecorder()
	router.ServeHTTP(out, req)
	return out
}

func TestAxonHubMediaImportHTTPJSON(t *testing.T) {
	for _, tc := range []struct{ provider, format, base, custom, requestPath, upstreamPath, kind, task string }{
		{"deepseek", "", "/v1", "", "/v1/completions", "/beta/completions", "chat", ""},
		{"openai", "openai/completions", "/raw##", "/ignored", "/v1/completions", "/raw", "chat", ""},
		{"openai", "openai/embeddings", "/raw/##", "", "/v1/embeddings", "/raw//embeddings", "embedding", ""},
		{"jina", "jina/embeddings", "/prefix", "/native-vector", "/v1/embeddings", "/prefix/native-vector", "embedding", "text-matching"},
		{"jina", "openai/embeddings", "/prefix", "/generic-vector", "/v1/embeddings", "/prefix/generic-vector", "embedding", ""},
		{"jina", "jina/rerank", "/prefix", "/rank", "/v1/rerank", "/prefix/rank", "rerank", ""},
		{"openai", "openai/image_generation", "/prefix", "/image", "/v1/images/generations", "/prefix/image", "image_generation", ""},
		{"modelscope", "openai/image_generation", "/prefix", "/generic-image", "/v1/images/generations", "/prefix/generic-image", "image_generation", ""},
		{"openai", "openai/moderations", "/prefix", "/moderate", "/v1/moderations", "/prefix/moderate", "chat", ""},
	} {
		t.Run(tc.provider+tc.requestPath+tc.custom, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var body map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if r.URL.Path != tc.upstreamPath || r.Header.Get("Authorization") != "Bearer fixture-media-key" || string(body["model"]) != `"provider-model"` || string(body["opaque"]) != "9007199254740993" {
					t.Errorf("imported destination/auth/body mismatch: path=%s body=%s", r.URL.Path, body)
				}
				var task string
				_ = json.Unmarshal(body["task"], &task)
				if task != tc.task {
					t.Errorf("source Jina profile task=%q want=%q", task, tc.task)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"data":[],"receipt":9007199254740993,"usage":{"prompt_tokens":3,"total_tokens":3}}`)
			}))
			defer upstream.Close()
			router := installAxonHubMediaHTTP(t, tc.provider, upstream.URL+tc.base, tc.format, tc.custom, tc.kind)
			out := requestAxonHubMediaHTTP(router, tc.requestPath, "application/json", strings.NewReader(`{"model":"client-alias","input":"hello","prompt":"hello","query":"hello","documents":["hello"],"opaque":9007199254740993}`))
			if out.Code != 200 || calls.Load() != 1 || !strings.Contains(out.Body.String(), `"receipt":9007199254740993`) {
				t.Fatalf("imported media HTTP status=%d calls=%d body=%s", out.Code, calls.Load(), out.Body.String())
			}
		})
	}
}

func TestAxonHubMediaImportHTTPMultipart(t *testing.T) {
	for _, tc := range []struct{ format, path string }{
		{"openai/image_edit", "/v1/images/edits"}, {"openai/image_variation", "/v1/images/variations"},
		{"openai/audio_transcriptions", "/v1/audio/transcriptions"}, {"openai/audio_translations", "/v1/audio/translations"},
	} {
		t.Run(tc.format, func(t *testing.T) {
			fileBytes := []byte{0, 1, 255, 13, 10}
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if err := r.ParseMultipartForm(1 << 20); err != nil {
					t.Error(err)
					return
				}
				defer r.MultipartForm.RemoveAll()
				if r.URL.Path != "/custom/upload" || r.Header.Get("Authorization") != "Bearer fixture-media-key" || r.FormValue("model") != "provider-model" || r.FormValue("opaque") != "9007199254740993" {
					t.Errorf("multipart destination/model/form changed: %s %v", r.URL.Path, r.MultipartForm.Value)
				}
				f, fh, err := r.FormFile("file")
				if err != nil {
					t.Error(err)
					return
				}
				defer f.Close()
				data, _ := io.ReadAll(f)
				if !bytes.Equal(data, fileBytes) || fh.Filename != "fixture.bin" || fh.Header.Get("Content-Type") != "application/octet-stream" {
					t.Error("imported multipart payload changed")
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"receipt":"multipart"}`)
			}))
			defer upstream.Close()
			router := installAxonHubMediaHTTP(t, "openai", upstream.URL+"/custom", tc.format, "/upload", "chat")
			var raw bytes.Buffer
			writer := multipart.NewWriter(&raw)
			_ = writer.WriteField("model", "client-alias")
			_ = writer.WriteField("opaque", "9007199254740993")
			h := textproto.MIMEHeader{}
			h.Set("Content-Disposition", `form-data; name="file"; filename="fixture.bin"`)
			h.Set("Content-Type", "application/octet-stream")
			part, err := writer.CreatePart(h)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = part.Write(fileBytes)
			_ = writer.Close()
			out := requestAxonHubMediaHTTP(router, tc.path, writer.FormDataContentType(), &raw)
			if out.Code != 200 || calls.Load() != 1 || !strings.Contains(out.Body.String(), "multipart") {
				t.Fatalf("imported multipart status=%d calls=%d body=%s", out.Code, calls.Load(), out.Body.String())
			}
		})
	}
}

func TestAxonHubMediaImportHTTPBinary(t *testing.T) {
	want := []byte{'I', 'D', '3', 0, 255, 10}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/audio/speech" || r.Header.Get("Authorization") != "Bearer fixture-media-key" {
			t.Error("speech import path/auth mismatch")
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write(want)
	}))
	defer upstream.Close()
	router := installAxonHubMediaHTTP(t, "groq", upstream.URL+"/api", "", "", "chat")
	out := requestAxonHubMediaHTTP(router, "/v1/audio/speech", "application/json", strings.NewReader(`{"model":"client-alias","input":"hello","voice":"alloy"}`))
	if out.Code != 200 || !bytes.Equal(out.Body.Bytes(), want) || out.Header().Get("Content-Type") != "audio/mpeg" {
		t.Fatalf("binary output changed: %d %q", out.Code, out.Body.Bytes())
	}
}

func TestAxonHubMediaImportHTTPImageProfiles(t *testing.T) {
	t.Run("minimax", func(t *testing.T) {
		var calls atomic.Int32
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if r.URL.Path != "/api/v1/image_generation" || r.Header.Get("Authorization") != "Bearer fixture-media-key" || body["model"] != "provider-model" || body["width"] != float64(1024) || body["height"] != float64(1024) || body["response_format"] != "base64" {
				t.Errorf("MiniMax imported profile not executed: %s %v", r.URL.Path, body)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"base_resp":{"status_code":0,"status_msg":"success"},"data":{"image_base64":["aW1hZ2U="]}}`)
		}))
		defer upstream.Close()
		router := installAxonHubMediaHTTP(t, "minimax", upstream.URL+"/api", "", "", "image_generation")
		out := requestAxonHubMediaHTTP(router, "/v1/images/generations", "application/json", strings.NewReader(`{"model":"client-alias","prompt":"draw","size":"1024x1024","response_format":"b64_json"}`))
		if out.Code != 200 || calls.Load() != 1 || !strings.Contains(out.Body.String(), `"b64_json":"aW1hZ2U="`) {
			t.Fatalf("MiniMax imported response=%d calls=%d %s", out.Code, calls.Load(), out.Body.String())
		}
	})
	t.Run("modelscope", func(t *testing.T) {
		var submitted, polled, downloaded atomic.Int32
		var upstream *httptest.Server
		upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch r.URL.Path {
			case "/prefix/images/generations":
				submitted.Add(1)
				var body map[string]any
				_ = json.NewDecoder(r.Body).Decode(&body)
				if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer fixture-media-key" || r.Header.Get("X-ModelScope-Async-Mode") != "true" || body["model"] != "provider-model" {
					t.Errorf("ModelScope imported submit mismatch: %s %v", r.URL.Path, body)
				}
				_, _ = io.WriteString(w, `{"task_id":"fixture-task","task_status":"PENDING"}`)
			case "/prefix/tasks/fixture-task":
				polled.Add(1)
				if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer fixture-media-key" || r.Header.Get("X-ModelScope-Task-Type") != "image_generation" {
					t.Error("ModelScope poll lost path/auth/headers")
				}
				_, _ = fmt.Fprintf(w, `{"task_status":"SUCCEED","output_images":[%q]}`, upstream.URL+"/image.png")
			case "/image.png":
				downloaded.Add(1)
				if r.Header.Get("Authorization") != "" {
					t.Error("ModelScope image download leaked credential")
				}
				w.Header().Set("Content-Type", "image/png")
				_, _ = w.Write([]byte("fixture-image"))
			default:
				t.Errorf("unexpected ModelScope path %s", r.URL.Path)
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer upstream.Close()
		router := installAxonHubMediaHTTP(t, "modelscope", upstream.URL+"/prefix", "", "", "image_generation")
		out := requestAxonHubMediaHTTP(router, "/v1/images/generations", "application/json", strings.NewReader(`{"model":"client-alias","prompt":"draw","response_format":"b64_json"}`))
		if out.Code != 200 || submitted.Load() != 1 || polled.Load() != 1 || downloaded.Load() != 1 || !strings.Contains(out.Body.String(), base64.StdEncoding.EncodeToString([]byte("fixture-image"))) {
			t.Fatalf("ModelScope import lifecycle=%d/%d/%d status=%d %s", submitted.Load(), polled.Load(), downloaded.Load(), out.Code, out.Body.String())
		}
	})
	t.Run("codex", func(t *testing.T) {
		var calls atomic.Int32
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if r.URL.Path != "/prefix/responses" || r.Header.Get("Authorization") != "Bearer fixture-media-key" || body["model"] != "fixture-main-model" || body["stream"] != true || body["store"] != false {
				t.Errorf("Codex imported Responses contract mismatch: %s %v", r.URL.Path, body)
			}
			tools, _ := body["tools"].([]any)
			if len(tools) != 1 {
				t.Error("Codex image tool missing")
			} else if tool, _ := tools[0].(map[string]any); tool["type"] != "image_generation" || tool["model"] != "provider-model" {
				t.Error("Codex image tool model lost alias")
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"fixture-response\",\"created_at\":123,\"status\":\"completed\",\"output\":[{\"type\":\"image_generation_call\",\"id\":\"fixture-image\",\"status\":\"completed\",\"result\":\"aW1hZ2U=\"}]}}\n\n")
		}))
		defer upstream.Close()
		router := installAxonHubMediaHTTP(t, "codex", upstream.URL+"/prefix#", "", "", "image_generation")
		out := requestAxonHubMediaHTTP(router, "/v1/images/generations", "application/json", strings.NewReader(`{"model":"client-alias","prompt":"draw","response_format":"b64_json"}`))
		if out.Code != 200 || calls.Load() != 1 || !strings.Contains(out.Body.String(), `"b64_json":"aW1hZ2U="`) {
			t.Fatalf("Codex imported output=%d calls=%d %s", out.Code, calls.Load(), out.Body.String())
		}
	})
}

func TestAxonHubMediaImportHTTPGeminiEmbedding(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		raw, _ := io.ReadAll(r.Body)
		if r.URL.Path != "/prefix/v1/models/provider-model:embedContent" || r.Header.Get("x-goog-api-key") != "fixture-media-key" || r.Header.Get("Authorization") != "" || !bytes.Contains(raw, []byte(`"text":"hello"`)) {
			t.Errorf("Gemini import path/auth/body mismatch: %s %s", r.URL.Path, raw)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"embedding":{"values":[0.1,0.2]}}`)
	}))
	defer upstream.Close()
	router := installAxonHubMediaHTTP(t, "gemini", upstream.URL+"/prefix/v1", "gemini/embeddings", "/ignored-custom", "embedding")
	out := requestAxonHubMediaHTTP(router, "/v1beta/models/client-alias:embedContent", "application/json", strings.NewReader(`{"content":{"parts":[{"text":"hello"}]}}`))
	if out.Code != 200 || calls.Load() != 1 || !strings.Contains(out.Body.String(), `"values":[0.1,0.2]`) {
		t.Fatalf("Gemini import HTTP=%d calls=%d %s", out.Code, calls.Load(), out.Body.String())
	}
}
