package router

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/deliciousbuding/metapi-go/web"
)

func TestTaskProgramRecoveryIsAdminOnly(t *testing.T) {
	cfg := &config.Config{RequestBodyLimit: config.DefaultRequestBodyLimit, DataDir: t.TempDir()}
	config.SetRuntime(&config.RuntimeSettings{AuthToken: "fixture-admin", ProxyToken: "fixture-proxy", DbType: store.DialectSQLite, DbUrl: filepath.Join(cfg.DataDir, "recovery.db")})
	t.Cleanup(func() { config.SetRuntime(nil) })
	if err := store.EnsureRuntimeDatabase(cfg, config.RuntimeSafe()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.CloseDatabase() })
	mux := New(cfg, web.Dist)
	for _, endpoint := range []struct{ method, path string }{{"GET", "/api/channel-plugin-versions"}, {"POST", "/api/channel-plugin-versions/restore"}} {
		for _, token := range []string{"", "fixture-proxy"} {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(endpoint.method, endpoint.path, strings.NewReader(`{}`))
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			mux.ServeHTTP(rec, req)
			want := http.StatusUnauthorized
			if token != "" {
				want = http.StatusForbidden
			}
			if rec.Code != want {
				t.Fatalf("%s %s non-admin got %d", endpoint.method, endpoint.path, rec.Code)
			}
		}
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/channel-plugin-versions", nil)
	req.Header.Set("Authorization", "Bearer fixture-admin")
	mux.ServeHTTP(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Header().Get("Cache-Control"), "no-store") || !strings.Contains(rec.Body.String(), `"items":[]`) {
		t.Fatalf("admin index unavailable: %d %s", rec.Code, rec.Body.String())
	}
}
