package proxyhandler

import (
	"bytes"
	"context"
	"encoding/json"
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
	"github.com/deliciousbuding/metapi-go/internal/pgtest"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/service/backup"
	"github.com/deliciousbuding/metapi-go/service/oauth"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

func installDirectMediaRuntime(t *testing.T, dialect string, endpoints store.DirectEndpoints, protocols int, override string) *store.DB {
	t.Helper()
	dsn := ":memory:"
	if dialect == store.DialectPostgres {
		dsn = os.Getenv("PG_TEST_DSN")
		if dsn == "" {
			t.Skip("PG_TEST_DSN not set")
		}
	}
	db, err := store.Open(dialect, dsn, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if dialect == store.DialectPostgres {
		pgtest.Reset(t, db.DB)
	}
	if err = store.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"version":"1.4","channels":[{"id":1,"type":"moonshot","name":"media fixture","base_url":"https://unused.invalid","credentials":{"apiKey":"fixture-media-key"},"supported_models":["provider-model"]}],"models":[{"id":1,"model_id":"client-alias","settings":{"associations":[{"type":"channel_model","channelModel":{"channelId":1,"modelId":"provider-model"}}]}}]}`)
	if _, err = backup.ImportAxonHubV14(db, raw, "media-fixture", false); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(db.Rebind(`UPDATE upstream_channels SET endpoint_config=?,param_override=?`), endpoints, override); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(db.Rebind(`UPDATE upstream_grants SET protocols=?`), protocols); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE upstream_group_items SET protocol_order='[]'`); err != nil {
		t.Fatal(err)
	}
	old := config.RuntimeSafe()
	config.SetRuntime(&config.RuntimeSettings{})
	t.Cleanup(func() { config.SetRuntime(old); routing.SetGlobalCache(nil) })
	router := routing.NewTokenRouter(service.NewProxyRoutingStore(db), &config.Config{}, nil, nil)
	previous := getUpstreamConfig()
	SetUpstreamConfig(&UpstreamConfig{Router: router, LogProxy: func(context.Context, proxy.ProxyLogEntry) error { return nil }, ResolveDirectCredential: func(ctx context.Context, id int64, proxyURL *string, force bool) (*oauth.DirectCredentialResult, error) {
		return oauth.ResolveDirectCredential(ctx, db.DB, id, proxyURL, force)
	}})
	t.Cleanup(func() { SetUpstreamConfig(previous) })
	return db
}

func directMediaRouter() http.Handler {
	r := chi.NewRouter()
	r.Route("/v1", RegisterProxyRoutes)
	RegisterNonV1ProxyRoutes(r)
	return r
}

func TestDirectMediaRuntimeJSON(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			for _, tc := range []struct {
				key, path string
				bit       int
			}{
				{"completions", "/v1/completions", store.DirectProtocolCompletions},
				{"embeddings", "/v1/embeddings", store.DirectProtocolEmbeddings},
				{"rerank", "/v1/rerank", store.DirectProtocolRerank},
				{"imageGeneration", "/v1/images/generations", store.DirectProtocolImageGeneration},
				{"imageEdit", "/v1/images/edits", store.DirectProtocolImageEdit},
				{"imageVariation", "/v1/images/variations", store.DirectProtocolImageVariation},
				{"audioSpeech", "/v1/audio/speech", store.DirectProtocolAudioSpeech},
				{"audioTranscription", "/v1/audio/transcriptions", store.DirectProtocolAudioTranscription},
				{"audioTranslation", "/v1/audio/translations", store.DirectProtocolAudioTranslation},
				{"moderations", "/v1/moderations", store.DirectProtocolModerations},
			} {
				t.Run(tc.key, func(t *testing.T) {
					var calls atomic.Int32
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						raw, _ := io.ReadAll(r.Body)
						if r.URL.Path != "/precise/"+tc.key || r.Header.Get("Authorization") != "Bearer fixture-media-key" {
							t.Errorf("wrong destination/auth: %s", r.URL.Path)
						}
						if !strings.Contains(string(raw), `"model":"provider-model"`) || !strings.Contains(string(raw), "9007199254740993") || !strings.Contains(string(raw), `"quality":"high"`) {
							t.Errorf("body model/opaque/override changed: %s", raw)
						}
						w.Header().Set("Content-Type", "application/json")
						_, _ = io.WriteString(w, `{"receipt":"media","vendor":{"id":9007199254740993},"usage":{"prompt_tokens":3,"total_tokens":3}}`)
					}))
					defer upstream.Close()
					var endpoints store.DirectEndpoints
					encoded, _ := json.Marshal(map[string]any{tc.key: &store.DirectEndpoint{URL: upstream.URL + "/precise/" + tc.key, Auth: store.DirectAuthBearer}})
					if err := json.Unmarshal(encoded, &endpoints); err != nil {
						t.Fatal(err)
					}
					installDirectMediaRuntime(t, dialect, endpoints, tc.bit, `{"quality":"high","model":"must-not-win"}`)
					r := makeProxyReq("POST", tc.path, `{"model":"client-alias","input":"hi","prompt":"hi","vendor":{"id":9007199254740993}}`)
					out := httptest.NewRecorder()
					directMediaRouter().ServeHTTP(out, r)
					if out.Code != 200 || calls.Load() != 1 || !strings.Contains(out.Body.String(), "9007199254740993") {
						t.Fatalf("media relay failed: %d calls=%d %s", out.Code, calls.Load(), out.Body.String())
					}
				})
			}
		})
	}
}

func TestDirectMediaRuntimeMultipart(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			for _, path := range []string{"/v1/images/edits", "/v1/images/variations", "/v1/audio/transcriptions", "/v1/audio/translations"} {
				t.Run(path, func(t *testing.T) {
					fileBytes := []byte{0, 1, 2, 255, 13, 10, 0}
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if err := r.ParseMultipartForm(1 << 20); err != nil {
							t.Error(err)
							return
						}
						if r.FormValue("model") != "provider-model" || r.FormValue("prompt") != "overridden" || r.FormValue("new_field") != "9007199254740993" || len(r.MultipartForm.Value["timestamp_granularities[]"]) != 2 {
							t.Errorf("multipart values lost: %v", r.MultipartForm.Value)
						}
						fh := r.MultipartForm.File["file"][0]
						f, err := fh.Open()
						if err != nil {
							t.Error(err)
							return
						}
						defer f.Close()
						b, _ := io.ReadAll(f)
						if !bytes.Equal(b, fileBytes) || fh.Header.Get("Content-Type") != "audio/wav" || fh.Filename != "source.wav" {
							t.Error("multipart file bytes/type/name changed")
						}
						w.Header().Set("Content-Type", "application/json")
						_, _ = io.WriteString(w, `{"text":"receipt"}`)
					}))
					defer upstream.Close()
					bit := proxy.DirectProtocolForPath(path)
					var endpoints store.DirectEndpoints
					for _, entry := range endpoints.Entries() {
						if entry.Protocol == bit {
							raw, _ := json.Marshal(map[string]any{entry.Key: &store.DirectEndpoint{URL: upstream.URL + path, Auth: store.DirectAuthBearer}})
							_ = json.Unmarshal(raw, &endpoints)
						}
					}
					installDirectMediaRuntime(t, dialect, endpoints, bit, `{"prompt":"overridden","new_field":9007199254740993}`)
					var raw bytes.Buffer
					writer := multipart.NewWriter(&raw)
					_ = writer.WriteField("model", "client-alias")
					_ = writer.WriteField("prompt", "original")
					_ = writer.WriteField("timestamp_granularities[]", "word")
					_ = writer.WriteField("timestamp_granularities[]", "segment")
					header := textproto.MIMEHeader{}
					header.Set("Content-Disposition", `form-data; name="file"; filename="source.wav"`)
					header.Set("Content-Type", "audio/wav")
					part, _ := writer.CreatePart(header)
					_, _ = part.Write(fileBytes)
					_ = writer.Close()
					r := httptest.NewRequest("POST", path, &raw)
					r.Header.Set("Content-Type", writer.FormDataContentType())
					r = r.WithContext(auth.WithProxyAuth(r.Context(), &auth.ProxyAuthContext{Source: "global", Token: "fixture-client"}))
					out := httptest.NewRecorder()
					directMediaRouter().ServeHTTP(out, r)
					if out.Code != 200 || !strings.Contains(out.Body.String(), "receipt") {
						t.Fatalf("multipart failed: %d %s", out.Code, out.Body.String())
					}
				})
			}

		})
	}
}

func TestDirectMediaRuntimeBinaryAndSSE(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			for _, stream := range []bool{false, true} {
				t.Run(map[bool]string{false: "binary", true: "sse"}[stream], func(t *testing.T) {
					payload := []byte{'I', 'D', '3', 0, 255, '{', '}', 10}
					contentType := "audio/mpeg"
					if stream {
						payload = []byte("data: {\"id\":\"cmpl\",\"choices\":[{\"text\":\"receipt\",\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
						contentType = "text/event-stream"
					}
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.Header().Set("Content-Type", contentType)
						w.WriteHeader(200)
						_, _ = w.Write(payload[:4])
						if f, ok := w.(http.Flusher); ok {
							f.Flush()
						}
						_, _ = w.Write(payload[4:])
					}))
					defer upstream.Close()
					endpoints := store.DirectEndpoints{AudioSpeech: &store.DirectEndpoint{URL: upstream.URL + "/speech", Auth: store.DirectAuthBearer}}
					bit := store.DirectProtocolAudioSpeech
					path := "/v1/audio/speech"
					if stream {
						endpoints = store.DirectEndpoints{Completions: &store.DirectEndpoint{URL: upstream.URL + "/completions", Auth: store.DirectAuthBearer}}
						bit = store.DirectProtocolCompletions
						path = "/v1/completions"
					}
					installDirectMediaRuntime(t, dialect, endpoints, bit, "")
					out := httptest.NewRecorder()
					directMediaRouter().ServeHTTP(out, makeProxyReq("POST", path, `{"model":"client-alias","input":"hi","prompt":"hi","stream":true}`))
					if out.Code != 200 || !bytes.Equal(out.Body.Bytes(), payload) || !strings.HasPrefix(out.Header().Get("Content-Type"), contentType) {
						t.Fatalf("media bytes/type changed: %d %q %q", out.Code, out.Header().Get("Content-Type"), out.Body.Bytes())
					}
				})
			}

		})
	}
}
