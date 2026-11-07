package proxyhandler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/service/channelplugins"
	"github.com/deliciousbuding/metapi-go/store"
)

const captureWindowModule = `export const meta={apiVersion:2,kind:"provider",key:"capture-window",name:"Capture window",version:"1.0.0",protocols:["video"],credentialKinds:["api_key"],hooks:["submit","query","content","taskResult"]};export function submit(i){return {method:"POST",body:i.body};}export function query(){return {method:"GET"};}export function content(){return {method:"GET"};}export function taskResult(i){return {body:i.body};}`

func TestProviderSubmissionHasDurableReferenceBeforeResponse(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			started := make(chan struct{}, 1)
			release := make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			t.Cleanup(unblock)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				select {
				case started <- struct{}{}:
				default:
				}
				<-release
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"id":"fixture-task","object":"video","status":"queued","model":"provider-model","usage":{"total_tokens":7}}`)
			}))
			defer upstream.Close()
			defer unblock()
			db, server, _ := directVideoFixture(t, dialect, upstream.URL)
			if _, err := channelplugins.Save(t.Context(), db.DB, "capture-window", captureWindowModule, true, 0); err != nil {
				t.Fatal(err)
			}
			videoExec(t, db, `UPDATE upstream_channels SET request_config=?`, store.DirectRequestConfig{Provider: &store.DirectProviderBinding{Key: "capture-window", Protocols: []string{"video"}}})
			getUpstreamConfig().ChannelPlugins = channelplugins.New(db.DB)
			type result struct {
				status int
				body   []byte
				err    error
			}
			done := make(chan result, 1)
			go func() {
				req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+"/v1/videos", strings.NewReader(`{"model":"client-alias","prompt":"fixture"}`))
				if err != nil {
					done <- result{err: err}
					return
				}
				req.Header.Set("Authorization", "Bearer client-one")
				req.Header.Set("Content-Type", "application/json")
				resp, err := server.Client().Do(req)
				if err != nil {
					done <- result{err: err}
					return
				}
				defer resp.Body.Close()
				body, err := io.ReadAll(resp.Body)
				done <- result{status: resp.StatusCode, body: body, err: err}
			}()
			select {
			case <-started:
			case response := <-done:
				t.Fatalf("submit never entered upstream: %+v", response)
			case <-time.After(5 * time.Second):
				t.Fatal("submit did not start")
			}
			refs, err := store.ChannelPluginVersionReferences(t.Context(), db.DB)
			if err != nil {
				unblock()
				t.Fatal(err)
			}
			var captures int64
			for _, count := range refs {
				captures += count
			}
			if captures != 1 {
				t.Errorf("in-flight submit has %d persistent program references; expected one before any upstream task ID exists", captures)
			}
			unblock()
			select {
			case response := <-done:
				if response.err != nil || response.status != http.StatusOK || !strings.Contains(string(response.body), "video_direct_") {
					t.Fatalf("submit response: %+v", response)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("submit did not finish")
			}
			refs, err = store.ChannelPluginVersionReferences(t.Context(), db.DB)
			if err != nil || len(refs) != 1 {
				t.Fatalf("confirmed task lost its program reference: %v %v", refs, err)
			}
		})
	}
}

func TestProviderCaptureReleaseStorageFailureRetainsPin(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "capture-release.db")
	db, err := store.Open(store.DialectSQLite, dsn, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.AutoMigrate(db); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	identity := `{"version":3,"driverKey":"capture-window","driverFingerprint":"` + strings.Repeat("a", 64) + `"}`
	if _, err = db.Exec(`INSERT INTO proxy_video_tasks(public_id,upstream_video_id,site_url,token_value,direct_identity) VALUES('release-capture','','','',?)`, identity); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	ctx := &Ctx{videoCapture: &directVideoCapture{db: db, publicID: "release-capture", identity: identity}}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	releaseDirectVideoCapture(ctx, "fixture-request")
	peer, err := store.Open(store.DialectSQLite, dsn, false)
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	var count int
	if err = peer.Get(&count, `SELECT COUNT(*) FROM proxy_video_tasks WHERE public_id='release-capture'`); err != nil || count != 1 {
		t.Fatalf("failed release did not retain its durable pin: %d %v", count, err)
	}
}

func TestProviderDurableCaptureFailureAndCancellationLifecycle(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		for _, mode := range []string{"success", "submit-throw", "bad-descriptor", "decoder-known-id", "decoder-missing-id", "invalid-id", "capture-deleted", "cancel"} {
			t.Run(dialect+"/"+mode, func(t *testing.T) {
				var calls atomic.Int32
				started := make(chan struct{}, 1)
				release := make(chan struct{})
				var once sync.Once
				unblock := func() { once.Do(func() { close(release) }) }
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					if mode == "cancel" || mode == "capture-deleted" {
						select {
						case started <- struct{}{}:
						default:
						}
						select {
						case <-release:
						case <-r.Context().Done():
							return
						}
					}
					w.Header().Set("Content-Type", "application/json")
					id := `,"id":"fixture-task"`
					if mode == "decoder-missing-id" {
						id = ""
					}
					if mode == "invalid-id" {
						id = `,"id":"../invalid"`
					}
					_, _ = io.WriteString(w, `{"object":"video","status":"queued","model":"provider-model","usage":{"total_tokens":7}`+id+`}`)
				}))
				defer upstream.Close()
				defer unblock()
				db, server, _ := directVideoFixture(t, dialect, upstream.URL)
				source := strings.Replace(captureWindowModule, `apiVersion:2,`, `apiVersion:2,taskCapture:"durable",`, 1)
				if mode == "submit-throw" {
					source = strings.Replace(source, `return {method:"POST",body:i.body};`, `throw new Error("fixture-private-submit");`, 1)
				}
				if mode == "bad-descriptor" {
					source = strings.Replace(source, `return {method:"POST",body:i.body};`, `return {method:"GET",body:i.body};`, 1)
				}
				if strings.HasPrefix(mode, "decoder-") {
					source = strings.Replace(source, `export function taskResult(i){return {body:i.body};}`, `export function taskResult(i){throw new Error("fixture-private-decoder");}`, 1)
				}
				if _, err := channelplugins.Save(t.Context(), db.DB, "capture-window", source, true, 0); err != nil {
					t.Fatal(err)
				}
				videoExec(t, db, `UPDATE upstream_channels SET request_config=?`, store.DirectRequestConfig{Provider: &store.DirectProviderBinding{Key: "capture-window", Protocols: []string{"video"}}})
				getUpstreamConfig().ChannelPlugins = channelplugins.New(db.DB)
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				type result struct {
					status int
					body   []byte
					err    error
				}
				done := make(chan result, 1)
				go func() {
					req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/v1/videos", strings.NewReader(`{"model":"client-alias","prompt":"fixture"}`))
					if err != nil {
						done <- result{err: err}
						return
					}
					req.Header.Set("Authorization", "Bearer client-one")
					req.Header.Set("Content-Type", "application/json")
					resp, err := server.Client().Do(req)
					if err != nil {
						done <- result{err: err}
						return
					}
					defer resp.Body.Close()
					body, err := io.ReadAll(resp.Body)
					done <- result{status: resp.StatusCode, body: body, err: err}
				}()
				if mode == "cancel" || mode == "capture-deleted" {
					select {
					case <-started:
					case <-time.After(5 * time.Second):
						t.Fatal("submit did not enter upstream")
					}
					var id string
					if err := db.Get(&id, `SELECT public_id FROM proxy_video_tasks WHERE upstream_video_id=''`); err != nil {
						t.Fatal(err)
					}
					if _, err := loadDirectVideoTask(db, id); err == nil {
						t.Fatal("pending capture was exposed as a task")
					}
					if mode == "capture-deleted" {
						videoExec(t, db, `DELETE FROM proxy_video_tasks WHERE public_id=?`, id)
						unblock()
					} else {
						cancel()
					}
				}
				var response result
				select {
				case response = <-done:
				case <-time.After(5 * time.Second):
					t.Fatal("submit did not finish")
				}
				if mode == "cancel" {
					if response.err == nil {
						t.Fatal("cancelled client got a successful result")
					}
				} else {
					want := http.StatusBadGateway
					if mode == "success" {
						want = http.StatusOK
					}
					if response.err != nil || response.status != want || strings.Contains(string(response.body), "fixture-private") {
						t.Fatalf("unexpected response: %+v", response)
					}
				}
				wantCalls := int32(1)
				if mode == "submit-throw" || mode == "bad-descriptor" {
					wantCalls = 0
				}
				if calls.Load() != wantCalls {
					t.Fatalf("wrong dispatch/replay count: %d", calls.Load())
				}
				var pending int
				deadline := time.Now().Add(3 * time.Second)
				for {
					if err := db.Get(&pending, `SELECT COUNT(*) FROM proxy_video_tasks WHERE upstream_video_id=''`); err != nil {
						t.Fatal(err)
					}
					if pending == 0 {
						break
					}
					if time.Now().After(deadline) {
						t.Fatal("ended request did not release its pending capture")
					}
					time.Sleep(10 * time.Millisecond)
				}
				var confirmed int
				if err := db.Get(&confirmed, `SELECT COUNT(*) FROM proxy_video_tasks WHERE upstream_video_id<>''`); err != nil {
					t.Fatal(err)
				}
				wantConfirmed := 0
				if mode == "success" || mode == "decoder-known-id" {
					wantConfirmed = 1
				}
				if confirmed != wantConfirmed {
					t.Fatalf("wrong confirmed capture count: %d want %d", confirmed, wantConfirmed)
				}
				if confirmed == 1 {
					var state string
					if err := db.Get(&state, `SELECT accounting_state FROM proxy_video_tasks`); err != nil {
						t.Fatal(err)
					}
					var accounting struct {
						Usage store.VideoTaskUsage `json:"usage"`
					}
					if json.Unmarshal([]byte(state), &accounting) != nil || accounting.Usage.TotalTokens != 7 {
						t.Fatalf("raw task accounting changed: %s", state)
					}
				}
			})
		}
	}
}
