package proxyhandler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

// Ownership fixtures shared by the files/videos isolation tests.
//
// Tests that exercise the files or shared-upstream video surfaces need the real
// schema: ownership is read from proxy_file_objects / proxy_video_tasks, so an
// in-memory store with the production migrations is the cheapest faithful
// fixture. store.OverrideDB is process-global, so every fixture restores the
// previous handle on cleanup; none of these tests use t.Parallel.

func openProxyOwnershipDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open ownership db: %v", err)
	}
	if err := store.AutoMigrate(db); err != nil {
		db.Close()
		t.Fatalf("migrate ownership db: %v", err)
	}
	previous := store.GetDB()
	store.OverrideDB(db)
	t.Cleanup(func() {
		store.OverrideDB(previous)
		_ = db.Close()
	})
	return db
}

func managedKeyAuth(keyID int64) *auth.ProxyAuthContext {
	id := keyID
	return &auth.ProxyAuthContext{
		Source: "managed",
		Token:  "test-token",
		KeyID:  &id,
		Policy: auth.EmptyDownstreamRoutingPolicy,
	}
}

func globalTestAuth() *auth.ProxyAuthContext {
	return &auth.ProxyAuthContext{
		Source: "global",
		Token:  "test-token",
		Policy: auth.EmptyDownstreamRoutingPolicy,
	}
}

// testOwner is the owner recorded by injectAuth / makeProxyReqNoBody requests.
func testOwner() string {
	return auth.DirectVideoTaskOwner(globalTestAuth())
}

func managedKeyOwner(keyID int64) string {
	return auth.DirectVideoTaskOwner(managedKeyAuth(keyID))
}

func managedKeyRequest(method, path, body string, keyID int64) *http.Request {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	return req.WithContext(auth.WithProxyAuth(req.Context(), managedKeyAuth(keyID)))
}

// keyAuthMiddleware injects the managed key named by X-Test-Key-ID so one chi
// router can serve several downstream identities in the same test.
func keyAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		keyID, err := strconv.ParseInt(r.Header.Get("X-Test-Key-ID"), 10, 64)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}
		pac := managedKeyAuth(keyID)
		next.ServeHTTP(w, r.WithContext(auth.WithProxyAuth(r.Context(), pac)))
	})
}

func chiRouterForFilesWithKeys() chi.Router {
	r := chi.NewRouter()
	r.Use(keyAuthMiddleware)
	r.Route("/v1", func(r chi.Router) {
		RegisterFilesRoutes(r)
	})
	return r
}

func chiRouterForVideoWithKeys() chi.Router {
	r := chi.NewRouter()
	r.Use(keyAuthMiddleware)
	r.Post("/v1/videos", HandleVideosCreate)
	r.Get("/v1/videos/{id}", HandleVideosGet)
	r.Delete("/v1/videos/{id}", HandleVideosDelete)
	return r
}

func insertFileOwner(t *testing.T, db *store.DB, fileID, owner string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO proxy_file_objects (file_id, owner, created_at, updated_at)
		VALUES (?, ?, '2026-10-18T00:00:00Z', '2026-10-18T00:00:00Z')`, fileID, owner); err != nil {
		t.Fatalf("seed file owner for %s: %v", fileID, err)
	}
}

func fileOwner(db *store.DB, fileID string) (string, bool) {
	var owner string
	if err := db.QueryRow(`SELECT owner FROM proxy_file_objects WHERE file_id = ?`, fileID).Scan(&owner); err != nil {
		return "", false
	}
	return owner, true
}
