package proxyhandler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/deliciousbuding/metapi-go/service/channelplugins"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestProviderMediaQueryThenContentIsCredentialFree(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		for _, mode := range []string{"normal", "decoder-failure", "download-failure", "credential-injection", "private-target"} {
			t.Run(dialect+"/"+mode, func(t *testing.T) {
				var requests, downloads atomic.Int32
				cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					downloads.Add(1)
					for _, key := range []string{"Authorization", "Cookie", "X-Api-Key", "X-Goog-Api-Key", "X-Vendor-Private", "X-Media-Driver"} {
						if r.Header.Get(key) != "" {
							t.Errorf("CDN received %s", key)
						}
					}
					if r.Header.Get("Range") != "bytes=0-3" {
						t.Error("CDN lost Range")
					}
					if mode == "download-failure" {
						w.WriteHeader(503)
						fmt.Fprint(w, "fixture-private-cdn-error")
						return
					}
					w.Header().Set("Content-Type", "video/mp4")
					w.Header().Set("Content-Range", "bytes 0-3/4")
					w.WriteHeader(206)
					_, _ = w.Write([]byte{0, 1, 2, 3})
				}))
				defer cdn.Close()
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					requests.Add(1)
					if r.Header.Get("Authorization") != "Bearer fixture-upstream-key" || r.Header.Get("X-Vendor-Private") != "fixture-private-vendor" {
						t.Error("task query lost selected credential/private headers")
					}
					output, status := 0, "queued"
					if r.Method == "GET" {
						output, status = 2, "completed"
						if r.URL.Path != "/tenant/videos/job-one" || r.Header.Get("Range") != "" {
							t.Errorf("wrong metadata query: %s", r.URL.Path)
						}
					}
					native := fmt.Sprintf(`{"id":"job-one","object":"video","status":%q,"model":"provider-model","usage":{"input_tokens":7,"output_tokens":%d,"total_tokens":%d}}`, status, output, 7+output)
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(map[string]any{"vendor": map[string]any{"id": "job-one", "native": native, "url": cdn.URL + "/movie?signature=fixture-private-signed-url", "model": "provider-model", "meter": map[string]int{"input": 7, "output": output, "total": 7 + output}}})
				}))
				defer upstream.Close()
				db, server, _ := directVideoFixture(t, dialect, upstream.URL)
				videoExec(t, db, `UPDATE upstream_channels SET custom_header=?`, `[{"header_key":"X-Vendor-Private","header_value":"fixture-private-vendor"}]`)
				source := strings.Replace(providerMediaSource, `modelPath:"/vendor/model"`, `modelPath:"/vendor/model",taskIdPath:"/vendor/id"`, 1)
				source = strings.Replace(source, `export function content(){return {method:"GET",headers:{"X-Media-Driver":"v1"}};}`, `export function content(i){if(!i.body)return {query:true};return {url:JSON.parse(i.body).vendor.url};}`, 1)
				if mode == "decoder-failure" {
					source = strings.Replace(source, `return {body:JSON.parse(i.body).vendor.native};`, `if(i.taskId)throw new Error("fixture-private-decoder");return {body:JSON.parse(i.body).vendor.native};`, 1)
				}
				if mode == "credential-injection" {
					source = strings.Replace(source, `return {url:JSON.parse(i.body).vendor.url};`, `return {url:JSON.parse(i.body).vendor.url,headers:{Authorization:"forbidden"}};`, 1)
				}
				if _, err := channelplugins.Save(t.Context(), db.DB, "media", source, true, 0); err != nil {
					t.Fatal(err)
				}
				videoExec(t, db, `UPDATE upstream_channels SET request_config=?`, store.DirectRequestConfig{Provider: &store.DirectProviderBinding{Key: "media", Protocols: []string{"video"}}})
				getUpstreamConfig().ChannelPlugins = channelplugins.New(db.DB)
				code, _, body := videoHTTP(t, server, "POST", "/v1/videos", `{"model":"client-alias","prompt":"fixture"}`, "client-one")
				var created struct {
					ID string `json:"id"`
				}
				if code != 200 || json.Unmarshal(body, &created) != nil {
					t.Fatalf("create failed: %d %s", code, body)
				}
				if mode == "private-target" {
					old := mediaTargetAllowPrivate
					mediaTargetAllowPrivate = false
					t.Cleanup(func() { mediaTargetAllowPrivate = old })
				}
				req, _ := http.NewRequest("GET", server.URL+"/v1/videos/"+created.ID+"/content", nil)
				req.Header.Set("Authorization", "Bearer client-one")
				req.Header.Set("Range", "bytes=0-3")
				resp, err := server.Client().Do(req)
				if err != nil {
					t.Fatal(err)
				}
				defer resp.Body.Close()
				wantStatus, wantDownloads := 502, int32(0)
				if mode == "normal" {
					wantStatus, wantDownloads = 206, 1
				}
				if mode == "download-failure" {
					wantDownloads = 1
				}
				if resp.StatusCode != wantStatus || requests.Load() != 2 || downloads.Load() != wantDownloads {
					t.Fatalf("content protocol/failure changed: status=%d queries=%d downloads=%d", resp.StatusCode, requests.Load(), downloads.Load())
				}
				var raw string
				var state struct {
					Usage store.VideoTaskUsage `json:"usage"`
				}
				if err = db.Get(&raw, db.Rebind(`SELECT accounting_state FROM proxy_video_tasks WHERE public_id=?`), created.ID); err != nil || json.Unmarshal([]byte(raw), &state) != nil || state.Usage.TotalTokens != 9 {
					t.Fatalf("content failure erased/replayed raw usage: %s %v", raw, err)
				}
			})
		}
	}
}
