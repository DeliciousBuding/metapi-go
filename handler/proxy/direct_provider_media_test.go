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

const providerMediaSource = `export const meta={apiVersion:2,kind:"provider",key:"media",name:"Media",version:"1.0.0",protocols:["video"],credentialKinds:["api_key"],hooks:["submit","query","content","taskResult"],observations:{video:{response:{usagePaths:{input_tokens:"/vendor/meter/input",output_tokens:"/vendor/meter/output",total_tokens:"/vendor/meter/total"},modelPath:"/vendor/model",errorPath:"/vendor/error"}}}};export function submit(i){return {method:"POST",body:i.body,headers:{"X-Media-Driver":"v1"}};}export function query(){return {method:"GET",headers:{"X-Media-Driver":"v1"}};}export function content(){return {method:"GET",headers:{"X-Media-Driver":"v1"}};}export function taskResult(i){return {body:JSON.parse(i.body).vendor.native};}`

func TestProviderMediaPendingTaskSurvivesUpgradeAndReopen(t *testing.T) {
	for _, protocol := range []string{"video", "seedanceVideo", "zenmuxVideo"} {
		for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
			t.Run(protocol+"/"+dialect, func(t *testing.T) {
				module := strings.ReplaceAll(providerMediaSource, `protocols:["video"]`, `protocols:["`+protocol+`"]`)
				module = strings.ReplaceAll(module, `observations:{video:`, `observations:{`+protocol+`:`)
				var calls atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					if r.Header.Get("Authorization") != "Bearer fixture-upstream-key" || r.Header.Get("X-Media-Driver") != "v1" {
						t.Error("pending task switched its captured driver or credential")
					}
					if r.URL.Path == "/tenant/videos/job-one/content" {
						w.Header().Set("Content-Type", "video/mp4")
						fmt.Fprint(w, "fixture-video-bytes")
						return
					}
					status, output := "queued", 0
					if r.Method == "GET" {
						if r.URL.Path != "/tenant/videos/job-one" {
							t.Errorf("wrong query target: %s", r.URL.Path)
						}
						status, output = "completed", 2
					} else if r.Method != "POST" || r.URL.Path != "/tenant/videos" {
						t.Errorf("wrong submission: %s %s", r.Method, r.URL.Path)
					}
					w.Header().Set("Content-Type", "application/json")
					native := fmt.Sprintf(`{"id":"job-one","object":"video","model":"provider-model","status":%q,"usage":{"input_tokens":7,"output_tokens":%d,"total_tokens":%d},"extension":9007199254740993}`, status, output, 7+output)
					_ = json.NewEncoder(w).Encode(map[string]any{"vendor": map[string]any{"native": native, "model": "provider-model", "meter": map[string]int{"input": 7, "output": output, "total": 7 + output}}})
				}))
				defer upstream.Close()
				db, server, reopen := directVideoFixture(t, dialect, upstream.URL)
				if protocol != "video" {
					ep := &store.DirectEndpoint{URL: upstream.URL + "/tenant/videos", Auth: store.DirectAuthBearer}
					var endpoints store.DirectEndpoints
					bit := store.DirectProtocolSeedanceVideo
					if protocol == "seedanceVideo" {
						ep.Profile = "seedance-video"
						endpoints.SeedanceVideo = ep
					} else {
						ep.Profile = "zenmux-video"
						endpoints.ZenmuxVideo = ep
						bit = store.DirectProtocolZenmuxVideo
					}
					videoExec(t, db, `UPDATE upstream_channels SET endpoint_config=?`, endpoints)
					videoExec(t, db, `UPDATE upstream_grants SET protocols=?`, bit)
				}
				if _, err := channelplugins.Save(t.Context(), db.DB, "media", module, true, 0); err != nil {
					t.Fatal(err)
				}
				binding := store.DirectRequestConfig{Provider: &store.DirectProviderBinding{Key: "media", Protocols: []string{protocol}}}
				videoExec(t, db, `UPDATE upstream_channels SET request_config=?`, binding)
				getUpstreamConfig().ChannelPlugins = channelplugins.New(db.DB)
				status, _, body := videoHTTP(t, server, "POST", "/v1/videos", `{"model":"client-alias","prompt":"fixture video"}`, "client-one")
				if status != 200 || calls.Load() != 1 {
					t.Fatalf("provider media submission was not executed: status=%d calls=%d body=%s", status, calls.Load(), body)
				}
				var result struct {
					ID string `json:"id"`
				}
				if json.Unmarshal(body, &result) != nil || !strings.HasPrefix(result.ID, directVideoIDPrefix) {
					t.Fatalf("submission did not persist an owned task: %s", body)
				}
				if _, err := channelplugins.Save(t.Context(), db.DB, "media", strings.ReplaceAll(module, `"v1"`, `"v2"`), true, 1); err != nil {
					t.Fatal(err)
				}
				reopen()
				getUpstreamConfig().ChannelPlugins = channelplugins.New(store.GetDB().DB)
				status, _, body = videoHTTP(t, server, "GET", "/v1/videos/"+result.ID, "", "client-one")
				if status != 200 || !strings.Contains(string(body), `"completed"`) || !strings.Contains(string(body), "9007199254740993") {
					t.Fatalf("captured task did not survive upgrade/reopen: %d %s", status, body)
				}
				status, _, body = videoHTTP(t, server, "GET", "/v1/videos/"+result.ID+"/content", "", "client-one")
				if status != 200 || string(body) != "fixture-video-bytes" || calls.Load() != 3 {
					t.Fatalf("captured content operation failed: %d calls=%d %s", status, calls.Load(), body)
				}
				var state struct {
					Usage store.VideoTaskUsage `json:"usage"`
				}
				var rawState string
				if err := store.GetDB().Get(&rawState, store.GetDB().Rebind(`SELECT accounting_state FROM proxy_video_tasks WHERE public_id=?`), result.ID); err != nil || json.Unmarshal([]byte(rawState), &state) != nil || state.Usage.TotalTokens != 9 {
					t.Fatalf("cumulative task usage replayed or lost: %s %v", rawState, err)
				}
				status, _, _ = videoHTTP(t, server, "GET", "/v1/videos/"+result.ID, "", "client-two")
				if status != 404 || calls.Load() != 3 {
					t.Fatalf("task crossed owner boundary: %d calls=%d", status, calls.Load())
				}
				videoExec(t, store.GetDB(), `UPDATE upstream_channels SET request_config='{}'`)
				status, _, _ = videoHTTP(t, server, "GET", "/v1/videos/"+result.ID, "", "client-one")
				if status != 503 || calls.Load() != 3 {
					t.Fatalf("removed binding reverted to native dispatch: %d calls=%d", status, calls.Load())
				}
				videoExec(t, store.GetDB(), `UPDATE upstream_channels SET request_config=?`, binding)
				var originalSource string
				if err := store.GetDB().Get(&originalSource, `SELECT source FROM channel_plugin_versions LIMIT 1`); err != nil {
					t.Fatal(err)
				}
				videoExec(t, store.GetDB(), `UPDATE channel_plugin_versions SET source='damaged'`)
				status, _, _ = videoHTTP(t, server, "GET", "/v1/videos/"+result.ID, "", "client-one")
				if status != 503 || calls.Load() != 3 {
					t.Fatalf("damaged captured program was executed: %d calls=%d", status, calls.Load())
				}
				videoExec(t, store.GetDB(), `UPDATE channel_plugin_versions SET source=?`, originalSource)
				if _, err := channelplugins.Save(t.Context(), store.GetDB().DB, "media", strings.ReplaceAll(module, `"v1"`, `"v2"`), false, 2); err != nil {
					t.Fatal(err)
				}
				status, _, _ = videoHTTP(t, server, "GET", "/v1/videos/"+result.ID, "", "client-one")
				if status != 503 || calls.Load() != 3 {
					t.Fatalf("disabled media driver was bypassed: %d calls=%d", status, calls.Load())
				}
			})
		}
	}
}
