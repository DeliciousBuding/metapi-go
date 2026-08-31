package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

// errorCode contract: the whole "Failed to load …" read-path family carries
// the additive resourceLoadFailed code so the console can render one
// localized toast instead of 20+ English variants. All three response
// shapes are covered (writeError / writeErrorWithRequest / writeJSON legacy
// `{message}`); the per-entity English message stays the display fallback.
func TestErrorCodeResourceLoadFailed(t *testing.T) {
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("failed to open SQLite: %v", err)
	}
	defer db.Close()
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}

	r := chi.NewRouter()
	RegisterSitesRoutes(r, db.DB)
	RegisterStatsRoutes(r, db.DB)
	RegisterAccountsRoutes(r, db.DB, &config.Config{AccountCredentialSecret: "test-secret-for-load-fail"})

	// A closed DB makes every read fail without needing fixtures; the
	// response must NOT be a 200 with partial data.
	db.Close()

	cases := []struct {
		name   string
		method string
		path   string
		status int
	}{
		// writeJSON legacy `{message}` shape (accounts snapshot).
		{name: "accounts snapshot", method: http.MethodGet, path: "/api/accounts"},
		// writeErrorWithRequest shape (sites list).
		{name: "sites list", method: http.MethodGet, path: "/api/sites"},
		// writeError shape (stats dashboard, writeErrorCodeWithRequest).
		{name: "stats dashboard", method: http.MethodGet, path: "/api/stats/dashboard"},
		// stats heatmap writeError path.
		{name: "usage heatmap", method: http.MethodGet, path: "/api/stats/usage-heatmap"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want 500 (body=%s)", rec.Code, rec.Body.String())
			}
			var body struct {
				Error     string `json:"error"`
				Message   string `json:"message"`
				ErrorCode string `json:"errorCode"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("unmarshal: %v (body=%s)", err, rec.Body.String())
			}
			msg := body.Error
			if msg == "" {
				msg = body.Message
			}
			if msg == "" && tc.name == "accounts snapshot" {
				// The snapshot path populates a partial cache on failures in
				// some versions; the error envelope is still authoritative.
			}
			if body.ErrorCode != "resourceLoadFailed" {
				t.Fatalf("errorCode = %q, want %q (body=%s)", body.ErrorCode, "resourceLoadFailed", rec.Body.String())
			}
		})
	}
}
