package proxyhandler

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
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
	"github.com/deliciousbuding/metapi-go/service/channelplugins"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/tidwall/gjson"
)

var streamingImageModule = strings.Replace(strings.Replace(imageOperationModule, `query:{path`, `stream:true,query:{path`, 1), `{status:"pending"};`, `{status:"pending",partials:r.partial?[{index:0,body:r.partial}]:[]};`, 1)

func TestProviderAsyncImageStreamPublishesPartialBeforeCompletion(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		for _, protocol := range []string{"imageGeneration", "imageEdit", "modelscopeImageGeneration"} {
			t.Run(dialect+"/"+protocol, func(t *testing.T) {
				path, prefix, mask := "/v1/images/generations", "image_generation", store.DirectProtocolImageGeneration
				endpoints := store.DirectEndpoints{}
				endpoint := &store.DirectEndpoint{Auth: store.DirectAuthBearer}
				switch protocol {
				case "imageEdit":
					path, prefix, mask = "/v1/images/edits", "image_edit", store.DirectProtocolImageEdit
					endpoints.ImageEdit = endpoint
				case "modelscopeImageGeneration":
					mask = store.DirectProtocolModelScopeImageGeneration
					endpoint.Profile = "modelscope-image"
					endpoints.ModelScopeImageGeneration = endpoint
				default:
					endpoints.ImageGeneration = endpoint
				}
				var calls atomic.Int32
				release := make(chan struct{})
				var released sync.Once
				finish := func() { released.Do(func() { close(release) }) }
				supplier := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					count := calls.Add(1)
					if count == 1 && r.Method != http.MethodPost || count > 1 && (r.Method != http.MethodGet || r.URL.Path != "/tasks/one") {
						t.Error("stream changed operation or replayed POST")
					}
					if r.Header.Get("Authorization") != "Bearer fixture-media-key" {
						t.Error("stream changed host authentication")
					}
					if count == 1 {
						body, err := io.ReadAll(r.Body)
						if err != nil || gjson.GetBytes(body, "stream").Exists() || r.Header.Get("Accept") != "application/json" {
							t.Error("facade asked the supplier POST for SSE")
						}
					}
					output, status, partial := 0, "PENDING", ""
					if count == 2 {
						output = 1
						partial = `{"b64_json":"cHJldmlldy1pbWFnZQ==","created_at":1,"background":"opaque","output_format":"png","quality":"low","size":"1024x1024","extension":9007199254740993}`
					}
					if count >= 3 {
						select {
						case <-release:
						case <-r.Context().Done():
							return
						}
						output, status = 2, "DONE"
					}
					w.Header().Set("Content-Type", "application/json")
					native := `{"created":1,"data":[{"b64_json":"ZmluYWwtaW1hZ2U="}],"usage":{"input_tokens":7,"output_tokens":2,"total_tokens":9},"extension":9007199254740993}`
					_ = json.NewEncoder(w).Encode(map[string]any{"task_id": "one", "task_status": status, "partial": partial, "native": native, "meter": map[string]int{"input": 7, "output": output, "total": 7 + output}})
				}))
				defer supplier.Close()
				defer finish()
				endpoint.URL = supplier.URL + strings.TrimPrefix(path, "/v1")
				db := installDirectMediaRuntime(t, dialect, endpoints, mask, "")
				source := strings.ReplaceAll(streamingImageModule, "imageGeneration", protocol)
				if _, err := channelplugins.Save(t.Context(), db.DB, "image-operation", source, true, 0); err != nil {
					t.Fatal(err)
				}
				requestConfig := store.DirectRequestConfig{Provider: &store.DirectProviderBinding{Key: "image-operation", Protocols: []string{protocol}, ImageQueries: map[string]store.DirectImageQueryEndpoint{protocol: {URL: supplier.URL + "/tasks/{taskId}", Auth: store.DirectAuthBearer}}}}
				if _, err := db.Exec(db.Rebind(`UPDATE upstream_channels SET request_config=?`), requestConfig); err != nil {
					t.Fatal(err)
				}
				cfg := getUpstreamConfig()
				cfg.ChannelPlugins = channelplugins.New(db.DB)
				logs := make(chan proxy.ProxyLogEntry, 1)
				cfg.LogProxy = func(_ context.Context, entry proxy.ProxyLogEntry) error { logs <- entry; return nil }
				downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					request := r.WithContext(auth.WithProxyAuth(r.Context(), &auth.ProxyAuthContext{Token: "fixture-client", Source: "global", Policy: auth.EmptyDownstreamRoutingPolicy}))
					if protocol == "imageEdit" {
						HandleImagesEdits(w, request)
					} else {
						HandleImagesGenerations(w, request)
					}
				}))
				defer downstream.Close()
				ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
				defer cancel()
				request, err := http.NewRequestWithContext(ctx, http.MethodPost, downstream.URL+path, strings.NewReader(`{"model":"client-alias","prompt":"fixture","stream":true,"partial_images":1}`))
				if err != nil {
					t.Fatal(err)
				}
				request.Header.Set("Content-Type", "application/json")
				response, err := http.DefaultClient.Do(request)
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()
				if response.StatusCode != 200 || !strings.Contains(response.Header.Get("Content-Type"), "text/event-stream") {
					t.Fatalf("image stream never executed: status=%d calls=%d", response.StatusCode, calls.Load())
				}
				reader := bufio.NewReader(response.Body)
				var first strings.Builder
				for !strings.Contains(first.String(), `"b64_json":"cHJldmlldy1pbWFnZQ=="`) {
					line, err := reader.ReadString('\n')
					if err != nil {
						t.Fatalf("partial arrived only after final query was released: %v", err)
					}
					first.WriteString(line)
				}
				if strings.Contains(first.String(), ".completed") || !strings.Contains(first.String(), prefix+".partial_image") || !strings.Contains(first.String(), "9007199254740993") {
					t.Fatal("partial was fabricated, rounded or mislabeled")
				}
				finish()
				rest, err := io.ReadAll(reader)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(rest), prefix+".completed") || !strings.Contains(string(rest), "ZmluYWwtaW1hZ2U=") || strings.Contains(string(rest), "upstream_error") || calls.Load() != 3 {
					t.Fatalf("stream terminal or replay boundary lost: calls=%d tail=%s", calls.Load(), rest)
				}
				select {
				case entry := <-logs:
					if entry.Status != "success" || entry.TotalTokens == nil || *entry.TotalTokens != 9 || entry.FirstOutputLatencyMs == nil {
						t.Fatalf("stream lost original usage/output: %+v", entry)
					}
				case <-ctx.Done():
					t.Fatal("stream settlement missing")
				}
			})
		}
	}
}

func TestImageTaskStreamEventsValidateOptionsAndOriginalFields(t *testing.T) {
	for _, protocol := range []string{"imageGeneration", "imageEdit", "modelscopeImageGeneration"} {
		t.Run(protocol, func(t *testing.T) {
			options, err := newImageTaskStreamOptions(protocol, []byte(`{"partial_images":3,"n":2,"quality":"high"}`))
			if err != nil {
				t.Fatal(err)
			}
			frames, err := options.completed([]byte(`{"created":1,"data":[{"b64_json":"b25l"},{"b64_json":"dHdv"}],"usage":{"input_tokens":7,"output_tokens":2,"total_tokens":9},"extension":9007199254740993}`))
			if err != nil || len(frames) != 2 {
				t.Fatalf("multiple image results lost: %v", err)
			}
			for _, frame := range frames {
				if !strings.Contains(string(frame), "9007199254740993") || !strings.Contains(string(frame), `"quality":"high"`) || !strings.Contains(string(frame), `"created_at":1`) {
					t.Fatal("stream rounded numbers or lost defaults")
				}
			}
			for _, body := range []string{
				`{"b64_json":"b25l","usage":{"total_tokens":999}}`,
				`{"b64_json":"b25l","type":"image_edit.completed"}`,
				`{"b64_json":"b25l","partial_image_index":2}`,
				`{"b64_json":"bad base64"}`,
				`{"b64_json":"b25l","quality":null}`,
				`{"b64_json":"b25l","B64_JSON":"different"}`,
				`{"b64_json":"b25l","error":{"message":"fixture"}}`,
			} {
				if _, err = options.partial(0, []byte(body)); err == nil {
					t.Fatalf("invalid partial accepted: %s", body)
				}
			}
			for _, body := range []string{
				`{"created":1,"data":[{"b64_json":"b25l","usage":{"total_tokens":999}}]}`,
				`{"created":1,"created_at":2,"data":[{"b64_json":"b25l"}]}`,
			} {
				if _, err = options.completed([]byte(body)); err == nil {
					t.Fatal("completion introduced shadow usage or timestamp")
				}
			}
		})
	}
	for _, body := range []string{
		`{"partial_images":4}`, `{"partial_images":-1}`, `{"partial_images":"1"}`, `{"Partial_images":1}`,
		`{"n":0}`, `{"n":11}`, `{"output_format":"svg"}`, `{"output_format":"gif"}`, `{"response_format":"url"}`,
	} {
		if _, err := newImageTaskStreamOptions("imageGeneration", []byte(body)); err == nil {
			t.Fatalf("invalid stream request accepted: %s", body)
		}
	}
}

// After a real preview no failure may replay the submission or erase original
// billing. Repeated polling snapshots emit a preview only once; a changed image
// at the same index is a protocol failure, not another frame with a reused ID.
func TestProviderImageStreamFailuresKeepOneSubmissionAndRawUsage(t *testing.T) {
	for _, mode := range []string{"duplicate", "changed-partial", "out-of-order", "query-error", "decoder-error", "byte-limit", "large-frame", "disabled-after-partial", "no-partials"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int32
			var db *store.DB
			previousConfig := config.GetSafe()
			limit := 8 << 20
			if mode == "byte-limit" {
				limit = 1024
			}
			config.Set(&config.Config{ProxyMaxStreamResponseBytes: limit})
			t.Cleanup(func() { config.Set(previousConfig) })
			encoded := "cHJldmlldy1pbWFnZQ=="
			if mode == "large-frame" {
				encoded = strings.Repeat("A", maxIncrementalSsePendingBytes+4)
			}
			vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				count := calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				if mode == "query-error" && count == 3 {
					w.WriteHeader(403)
					_, _ = io.WriteString(w, `{"error":{"message":"synthetic supplier denial","code":"fixture_denied"}}`)
					return
				}
				if mode == "disabled-after-partial" && count == 2 {
					if _, err := db.Exec(`UPDATE channel_plugins SET enabled=FALSE WHERE plugin_key='image-operation'`); err != nil {
						t.Error(err)
					}
				}
				status, output, partial := "PENDING", count-1, ""
				if count >= 2 {
					image := encoded
					if mode == "changed-partial" && count == 3 {
						image = "Y2hhbmdlZA=="
					}
					partial = `{"b64_json":"` + image + `","created_at":1}`
					if mode == "byte-limit" {
						partial = `{"b64_json":"` + strings.Repeat("A", 2048) + `","created_at":1}`
					}
				}
				if mode == "no-partials" {
					partial = ""
				}
				if count >= 4 {
					status = "DONE"
				}
				// Usage remains cumulative and capped at the final real value.
				output = min(output, 2)
				native := `{"created":1,"data":[{"b64_json":"ZmluYWwtaW1hZ2U="}],"usage":{"input_tokens":7,"output_tokens":2,"total_tokens":9}}`
				_ = json.NewEncoder(w).Encode(map[string]any{"task_id": "one", "task_status": status, "partial": partial, "native": native, "meter": map[string]int32{"input": 7, "output": output, "total": 7 + output}})
			}))
			defer vendor.Close()
			endpoints := store.DirectEndpoints{ImageGeneration: &store.DirectEndpoint{URL: vendor.URL + "/images/generations", Auth: store.DirectAuthBearer}}
			db = installDirectMediaRuntime(t, store.DialectSQLite, endpoints, store.DirectProtocolImageGeneration, "")
			source := streamingImageModule
			if mode == "decoder-error" {
				source = strings.Replace(source, `const r=JSON.parse(i.body);`, `const r=JSON.parse(i.body);if(r.meter.output===2)throw new Error("synthetic-private-error");`, 1)
			}
			if mode == "out-of-order" {
				source = strings.Replace(source, `index:0`, `index:1`, 1)
			}
			if _, err := channelplugins.Save(t.Context(), db.DB, "image-operation", source, true, 0); err != nil {
				t.Fatal(err)
			}
			settings := store.DirectRequestConfig{Provider: &store.DirectProviderBinding{Key: "image-operation", Protocols: []string{"imageGeneration"}, ImageQueries: map[string]store.DirectImageQueryEndpoint{"imageGeneration": {URL: vendor.URL + "/tasks/{taskId}", Auth: store.DirectAuthBearer}}}}
			if _, err := db.Exec(db.Rebind(`UPDATE upstream_channels SET request_config=?`), settings); err != nil {
				t.Fatal(err)
			}
			cfg := getUpstreamConfig()
			cfg.ChannelPlugins = channelplugins.New(db.DB)
			var entries []proxy.ProxyLogEntry
			cfg.LogProxy = func(_ context.Context, entry proxy.ProxyLogEntry) error { entries = append(entries, entry); return nil }
			body := `{"model":"client-alias","prompt":"fixture","stream":true,"partial_images":1}`
			if mode == "no-partials" {
				body = `{"model":"client-alias","prompt":"fixture","stream":true,"partial_images":0}`
			}
			out := httptest.NewRecorder()
			HandleImagesGenerations(out, makeProxyReq(http.MethodPost, "/v1/images/generations", body))
			success := mode == "duplicate" || mode == "large-frame" || mode == "no-partials"
			wantTokens := int64(9)
			if mode == "disabled-after-partial" || mode == "byte-limit" || mode == "out-of-order" {
				wantTokens = 8
			}
			if mode == "query-error" {
				wantTokens = 8
			}
			wantStatus := "failed"
			if success {
				wantStatus = "success"
			}
			if len(entries) != 1 || entries[0].Status != wantStatus || entries[0].TotalTokens == nil || *entries[0].TotalTokens != wantTokens || strings.Contains(out.Body.String(), "synthetic-private-error") {
				t.Fatalf("stream lost billing/failure: calls=%d logs=%+v tail=%s", calls.Load(), entries, out.Body.String()[max(0, out.Body.Len()-400):])
			}
			if success {
				if calls.Load() != 4 || strings.Count(out.Body.String(), "event: image_generation.completed") != 1 {
					t.Fatal("stream replayed or lost final frame")
				}
				wantPartial := 1
				if mode == "no-partials" {
					wantPartial = 0
				}
				if strings.Count(out.Body.String(), "event: image_generation.partial_image") != wantPartial {
					t.Fatal("stream repeated or fabricated partial frames")
				}
			} else {
				if calls.Load() > 3 || strings.Contains(out.Body.String(), "event: image_generation.completed") || !strings.Contains(out.Body.String(), `"error":`) {
					t.Fatalf("failed stream was replayed or completed: calls=%d", calls.Load())
				}
				if mode == "query-error" && !strings.Contains(out.Body.String(), "fixture_denied") {
					t.Fatal("original query error body lost")
				}
			}
		})
	}
}
