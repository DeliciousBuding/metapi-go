package proxyhandler

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
)

// Upstream fixture shared by the isolation tests: every Files/videos call
// reaches the same credential, exactly like a real shared account. Ownership is
// the only thing that decides who may see which object.

func newOwnershipUpstream(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	return server, &hits
}

func wireOwnershipUpstream(t *testing.T, url string) {
	t.Helper()
	// The shared-upstream mapping pins the channel recorded at create time, so
	// the fixture must honor SelectPreferredChannel like the production router.
	SetUpstreamConfig(&UpstreamConfig{
		Router: &preferredTrackRouter{selected: routing.SelectedChannel{
			Channel:     store.RouteChannel{ID: 1, Enabled: true},
			Account:     store.Account{ID: 2, Status: "active"},
			Site:        store.Site{ID: 3, URL: url, Status: "active"},
			TokenValue:  "upstream-token",
			ActualModel: "gpt-4o",
		}},
	})
	t.Cleanup(func() { SetUpstreamConfig(nil) })
}

func uploadAsKey(t *testing.T, router http.Handler, keyID int64, filename, content string) *httptest.ResponseRecorder {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if err := writer.WriteField("purpose", "assistants"); err != nil {
		t.Fatalf("write purpose: %v", err)
	}
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create file: %v", err)
	}
	if _, err := part.Write([]byte(content)); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/files", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req = req.WithContext(auth.WithProxyAuth(req.Context(), managedKeyAuth(keyID)))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestFilesOwnershipIsolatesKeys(t *testing.T) {
	db := openProxyOwnershipDB(t)
	upstream, hits := newOwnershipUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost:
			_, _ = w.Write([]byte(`{"id":"file_shared","object":"file","filename":"shared.txt"}`))
		case r.Method == http.MethodDelete:
			_, _ = w.Write([]byte(`{"id":"file_shared","object":"file","deleted":true}`))
		default:
			_, _ = w.Write([]byte(`{"id":"file_shared","object":"file","filename":"shared.txt"}`))
		}
	})
	wireOwnershipUpstream(t, upstream.URL)
	router := chiRouterForFilesWithKeys()

	const keyOne, keyTwo = int64(101), int64(202)

	upload := uploadAsKey(t, router, keyOne, "shared.txt", "secret-content")
	if upload.Code != http.StatusOK {
		t.Fatalf("upload status = %d: %s", upload.Code, upload.Body.String())
	}
	if owner, ok := fileOwner(db, "file_shared"); !ok || owner != managedKeyOwner(keyOne) {
		t.Fatalf("uploaded owner = %q (present=%v), want %q", owner, ok, managedKeyOwner(keyOne))
	}

	// The owner can read metadata and content.
	for _, path := range []string{"/v1/files/file_shared", "/v1/files/file_shared/content"} {
		req := managedKeyRequest(http.MethodGet, path, "", keyOne)
		req.Header.Set("X-Test-Key-ID", "101")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("owner GET %s = %d: %s", path, rec.Code, rec.Body.String())
		}
	}

	// A different key sees the same object as nonexistent, and no request of
	// theirs reaches the shared upstream account for that id.
	before := hits.Load()
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/v1/files/file_shared"},
		{http.MethodGet, "/v1/files/file_shared/content"},
		{http.MethodDelete, "/v1/files/file_shared"},
	} {
		req := managedKeyRequest(tc.method, tc.path, "", keyTwo)
		req.Header.Set("X-Test-Key-ID", "202")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("other key %s %s = %d, want 404: %s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
		if got := hits.Load(); got != before {
			t.Fatalf("other key %s %s reached the upstream (%d hit(s))", tc.method, tc.path, got-before)
		}
	}

	// The owner can delete; the mapping is removed only after upstream 2xx.
	req := managedKeyRequest(http.MethodDelete, "/v1/files/file_shared", "", keyOne)
	req.Header.Set("X-Test-Key-ID", "101")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("owner delete = %d: %s", rec.Code, rec.Body.String())
	}
	if _, ok := fileOwner(db, "file_shared"); ok {
		t.Fatal("file ownership survived a successful delete")
	}

	// With the mapping gone the owner is denied too: no pass-through.
	req = managedKeyRequest(http.MethodGet, "/v1/files/file_shared", "", keyOne)
	req.Header.Set("X-Test-Key-ID", "101")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("deleted file GET = %d, want 404: %s", rec.Code, rec.Body.String())
	}
}

func TestFilesListReturnsOnlyOwnedObjects(t *testing.T) {
	db := openProxyOwnershipDB(t)
	upstream, _ := newOwnershipUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[` +
			`{"id":"file_key_one","object":"file","filename":"one.txt"},` +
			`{"id":"file_key_two","object":"file","filename":"two.txt"}],` +
			`"first_id":"file_key_one","last_id":"file_key_two","has_more":false}`))
	})
	wireOwnershipUpstream(t, upstream.URL)
	router := chiRouterForFilesWithKeys()

	insertFileOwner(t, db, "file_key_one", managedKeyOwner(101))
	insertFileOwner(t, db, "file_key_two", managedKeyOwner(202))

	list := func(keyID int64) string {
		req := managedKeyRequest(http.MethodGet, "/v1/files", "", keyID)
		req.Header.Set("X-Test-Key-ID", strconv.FormatInt(keyID, 10))
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("list as key %d = %d: %s", keyID, rec.Code, rec.Body.String())
		}
		return rec.Body.String()
	}

	one := list(101)
	if !strings.Contains(one, "file_key_one") {
		t.Fatalf("owner's list omits its own file: %s", one)
	}
	if strings.Contains(one, "file_key_two") {
		t.Fatalf("owner's list leaks another key's file: %s", one)
	}
	if !strings.Contains(one, `"first_id":"file_key_one"`) || !strings.Contains(one, `"last_id":"file_key_one"`) {
		t.Fatalf("filtered list keeps stale pagination ids: %s", one)
	}

	two := list(202)
	if !strings.Contains(two, "file_key_two") || strings.Contains(two, "file_key_one") {
		t.Fatalf("second key's list is not isolated: %s", two)
	}
}

func TestFilesListFailsClosedOnUnparseableResponse(t *testing.T) {
	openProxyOwnershipDB(t)
	for _, tc := range []struct {
		name string
		body string
	}{
		// A shape this build cannot filter. Relaying it would disclose the
		// account-wide inventory.
		{"wrong shape", `{"object":"list","data":{"id":"file_key_two"}}`},
		{"invalid JSON", `{"object":"list","data":[`},
		{"missing data", `{"object":"list"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream, _ := newOwnershipUpstream(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			})
			wireOwnershipUpstream(t, upstream.URL)
			router := chiRouterForFilesWithKeys()

			req := managedKeyRequest(http.MethodGet, "/v1/files", "", 101)
			req.Header.Set("X-Test-Key-ID", "101")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadGateway {
				t.Fatalf("status = %d, want 502; body = %s", rec.Code, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "file_key_two") {
				t.Fatalf("fail-closed response leaked upstream data: %s", rec.Body.String())
			}
		})
	}
}

func TestVideosOwnershipIsolatesKeysAcrossRestart(t *testing.T) {
	db := openProxyOwnershipDB(t)
	var (
		pathsMu  sync.Mutex
		gotPaths []string
	)
	upstream, hits := newOwnershipUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		pathsMu.Lock()
		gotPaths = append(gotPaths, r.Method+" "+r.URL.Path)
		pathsMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"id":"upstream_vid_shared","object":"video","status":"queued"}`))
			return
		}
		if r.Method == http.MethodDelete {
			_, _ = w.Write([]byte(`{"id":"upstream_vid_shared","deleted":true}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"upstream_vid_shared","object":"video","status":"completed"}`))
	})
	wireOwnershipUpstream(t, upstream.URL)
	router := chiRouterForVideoWithKeys()

	const keyOne, keyTwo = int64(101), int64(202)

	create := managedKeyRequest(http.MethodPost, "/v1/videos", `{"model":"sora-2","prompt":"a cat"}`, keyOne)
	create.Header.Set("X-Test-Key-ID", "101")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, create)
	if rec.Code != http.StatusOK {
		t.Fatalf("create = %d: %s", rec.Code, rec.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil || created.ID == "" {
		t.Fatalf("create response = %s", rec.Body.String())
	}
	publicID := created.ID

	var owner string
	if err := db.QueryRow(`SELECT owner FROM proxy_video_tasks WHERE public_id = ?`, publicID).Scan(&owner); err != nil {
		t.Fatalf("durable owner: %v", err)
	}
	if owner != managedKeyOwner(keyOne) {
		t.Fatalf("durable owner = %q, want %q", owner, managedKeyOwner(keyOne))
	}

	// Cold cache: the decision must come from the database, as it does after a
	// restart or on another instance.
	dropVideoTaskMemory(publicID)

	req := managedKeyRequest(http.MethodGet, "/v1/videos/"+publicID, "", keyOne)
	req.Header.Set("X-Test-Key-ID", "101")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("owner GET after cold cache = %d: %s", rec.Code, rec.Body.String())
	}

	before := hits.Load()
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/v1/videos/" + publicID},
		{http.MethodGet, "/v1/videos/upstream_vid_shared"},
		{http.MethodDelete, "/v1/videos/" + publicID},
	} {
		req := managedKeyRequest(tc.method, tc.path, "", keyTwo)
		req.Header.Set("X-Test-Key-ID", "202")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("other key %s %s = %d, want 404: %s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
		if got := hits.Load(); got != before {
			t.Fatalf("other key %s %s reached the upstream (%d hit(s))", tc.method, tc.path, got-before)
		}
	}

	// Owner delete removes the mapping, and the delete path used the upstream
	// id, not the public id.
	req = managedKeyRequest(http.MethodDelete, "/v1/videos/"+publicID, "", keyOne)
	req.Header.Set("X-Test-Key-ID", "101")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("owner delete = %d: %s", rec.Code, rec.Body.String())
	}
	var mapping int
	if err := db.QueryRow(`SELECT COUNT(*) FROM proxy_video_tasks WHERE public_id = ?`, publicID).Scan(&mapping); err != nil {
		t.Fatalf("count mapping: %v", err)
	}
	if mapping != 0 {
		t.Fatalf("mapping rows after successful delete = %d, want 0", mapping)
	}
	pathsMu.Lock()
	lastPath := ""
	if len(gotPaths) > 0 {
		lastPath = gotPaths[len(gotPaths)-1]
	}
	pathsMu.Unlock()
	if lastPath != "DELETE /v1/videos/upstream_vid_shared" {
		t.Fatalf("last upstream path = %q, want the DELETE to use the upstream id (all: %v)", lastPath, gotPaths)
	}
}

func TestVideosOwnerlessLegacyRowIsDenied(t *testing.T) {
	db := openProxyOwnershipDB(t)
	upstream, hits := newOwnershipUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("ownerless task reached the upstream: %s %s", r.Method, r.URL.Path)
	})
	wireOwnershipUpstream(t, upstream.URL)
	router := chiRouterForVideoWithKeys()

	const publicID = "video_legacy_ownerless"
	if _, err := db.Exec(`INSERT INTO proxy_video_tasks (public_id, upstream_video_id, site_url, token_value)
		VALUES (?, 'upstream_legacy_ownerless', '', '')`, publicID); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}
	dropVideoTaskMemory(publicID)

	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		req := managedKeyRequest(method, "/v1/videos/"+publicID, "", 101)
		req.Header.Set("X-Test-Key-ID", "101")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s ownerless row = %d, want 404 (fail closed): %s", method, rec.Code, rec.Body.String())
		}
	}
	if got := hits.Load(); got != 0 {
		t.Fatalf("ownerless task reached the upstream %d time(s)", got)
	}
}

func TestVideosDeleteUpstreamFailureKeepsDurableMapping(t *testing.T) {
	db := openProxyOwnershipDB(t)
	upstream, _ := newOwnershipUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"nope","type":"server_error"}}`))
	})
	wireOwnershipUpstream(t, upstream.URL)
	router := chiRouterForVideoWithKeys()

	const publicID = "video_delete_failure_durable"
	SaveProxyVideoTask(&ProxyVideoTask{
		PublicID:        publicID,
		UpstreamVideoID: "upstream_delete_failure",
		Owner:           managedKeyOwner(101),
		RequestedModel:  "sora-2",
	})
	t.Cleanup(func() { DeleteProxyVideoTaskByPublicID(publicID) })

	req := managedKeyRequest(http.MethodDelete, "/v1/videos/"+publicID, "", 101)
	req.Header.Set("X-Test-Key-ID", "101")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("delete = %d, want the upstream 500: %s", rec.Code, rec.Body.String())
	}
	var owner string
	if err := db.QueryRow(`SELECT owner FROM proxy_video_tasks WHERE public_id = ?`, publicID).Scan(&owner); err != nil {
		t.Fatalf("mapping must survive a failed upstream delete: %v", err)
	}
}

func dropVideoTaskMemory(publicID string) {
	videoTaskStoreMu.Lock()
	delete(videoTaskStore, publicID)
	videoTaskStoreMu.Unlock()
}
