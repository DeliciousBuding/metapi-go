package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

// ---- B1 (sub2api/cliproxyapi borrow): admin write-operation audit log ----

func TestAuditMiddleware_RecordsWritesSkipsReads(t *testing.T) {
	db, _ := setupStatsSQLiteTest(t)

	r := chi.NewRouter()
	r.Use(AuditMiddleware(db.DB))
	r.Get("/api/test/read", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
	})
	r.Post("/api/test/write", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusCreated, map[string]string{"ok": "true"})
	})
	r.Put("/api/test/update", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	r.Delete("/api/test/delete", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
	})
	r.Patch("/api/test/patch", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/test/read", nil)
	req.Header.Set("Authorization", "Bearer admin-token-abc")
	r.ServeHTTP(httptest.NewRecorder(), req)

	writeReqs := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/test/write"},
		{http.MethodPut, "/api/test/update"},
		{http.MethodDelete, "/api/test/delete"},
		{http.MethodPatch, "/api/test/patch"},
	}
	for _, wr := range writeReqs {
		req := httptest.NewRequest(wr.method, wr.path, nil)
		req.Header.Set("Authorization", "Bearer admin-token-abc")
		r.ServeHTTP(httptest.NewRecorder(), req)
	}

	var total int
	if err := db.Get(&total, "SELECT COUNT(*) FROM admin_audit_logs"); err != nil {
		t.Fatalf("count: %v", err)
	}
	if total != 4 {
		t.Fatalf("audit rows = %d, want 4 (writes only, GET skipped)", total)
	}

	var actor string
	var method, path, requestID, remoteIP, createdAt string
	var status int
	if err := db.Get(&actor, "SELECT actor FROM admin_audit_logs ORDER BY id LIMIT 1"); err != nil {
		t.Fatalf("read actor: %v", err)
	}
	// sha256("admin-token-abc") prefix — stable, never the raw token.
	if len(actor) != 8 || actor == "admin-token-abc" {
		t.Fatalf("actor = %q, want 8-hex prefix of token hash", actor)
	}
	if err := db.Get(&status, "SELECT status FROM admin_audit_logs WHERE method = ?", http.MethodPatch); err != nil {
		t.Fatalf("read patch status: %v", err)
	}
	if status != http.StatusBadRequest {
		t.Fatalf("patch status recorded = %d, want 400", status)
	}
	if err := db.Get(&path, "SELECT path FROM admin_audit_logs WHERE method = ?", http.MethodPut); err != nil {
		t.Fatalf("read put path: %v", err)
	}
	if path != "/api/test/update" {
		t.Fatalf("put path = %q, want /api/test/update", path)
	}
	if err := db.Get(&method, "SELECT method FROM admin_audit_logs WHERE method = ?", http.MethodDelete); err != nil {
		t.Fatalf("read delete method: %v", err)
	}
	if method != http.MethodDelete {
		t.Fatalf("method = %q, want DELETE", method)
	}
	// Fields are populated (request id / ip / timestamp present). request_id
	// comes from the WithRequestID middleware (mounted in the real router);
	// this test router omits it, so only created_at is asserted here.
	_ = db.Get(&requestID, "SELECT COALESCE(request_id,'') FROM admin_audit_logs LIMIT 1")
	_ = db.Get(&remoteIP, "SELECT COALESCE(remote_ip,'') FROM admin_audit_logs LIMIT 1")
	_ = db.Get(&createdAt, "SELECT created_at FROM admin_audit_logs LIMIT 1")
	if createdAt == "" {
		t.Fatalf("audit row incomplete: created_at empty")
	}
}

func TestAuditLogs_ListWithFilters(t *testing.T) {
	db, _ := setupStatsSQLiteTest(t)

	// Seed rows directly (middleware behavior covered above).
	now := time.Now().UTC().Format(time.RFC3339)
	for _, row := range []struct {
		method string
		path   string
		status int
	}{
		{"POST", "/api/accounts", 200},
		{"PUT", "/api/accounts/1/tags", 200},
		{"DELETE", "/api/sites/3", 204},
		{"POST", "/api/models/check/1", 200},
	} {
		if _, err := db.Exec(`INSERT INTO admin_audit_logs (actor, method, path, status, request_id, remote_ip, created_at)
			VALUES ('aabbccdd', ?, ?, ?, 'req-x', '1.2.3.4', ?)`, row.method, row.path, row.status, now); err != nil {
			t.Fatalf("seed audit row: %v", err)
		}
	}

	r := chi.NewRouter()
	RegisterAuditLogsRoutes(r, db.DB)

	// All rows, newest first.
	resp := doGet(t, r, "/api/admin/audit-logs")
	if resp.Code != 200 {
		t.Fatalf("audit-logs returned %d: %s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if int(body["total"].(float64)) != 4 {
		t.Fatalf("total = %v, want 4", body["total"])
	}
	items := body["items"].([]any)
	if len(items) != 4 {
		t.Fatalf("items = %d, want 4", len(items))
	}
	first := items[0].(map[string]any)
	if first["path"] != "/api/models/check/1" {
		t.Fatalf("items[0].path = %v, want newest (models/check/1)", first["path"])
	}
	if first["actor"] != "aabbccdd" || first["remoteIp"] != "1.2.3.4" {
		t.Fatalf("items[0] = %#v, want actor+remoteIp populated", first)
	}

	// Method filter.
	resp = doGet(t, r, "/api/admin/audit-logs?method=POST")
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal filtered: %v", err)
	}
	if int(body["total"].(float64)) != 2 {
		t.Fatalf("POST total = %v, want 2", body["total"])
	}

	// Path substring filter.
	resp = doGet(t, r, "/api/admin/audit-logs?path=accounts")
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal path filter: %v", err)
	}
	if int(body["total"].(float64)) != 2 {
		t.Fatalf("accounts path total = %v, want 2", body["total"])
	}

	// Limit.
	resp = doGet(t, r, "/api/admin/audit-logs?limit=1")
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal limit: %v", err)
	}
	if len(body["items"].([]any)) != 1 {
		t.Fatalf("limit items = %d, want 1", len(body["items"].([]any)))
	}
}

func TestAuditMiddleware_NilDBIsNoop(t *testing.T) {
	r := chi.NewRouter()
	r.Use(AuditMiddleware(nil))
	r.Get("/api/ping", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
	})
	r.Post("/api/ping", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
	})

	req := httptest.NewRequest(http.MethodPost, "/api/ping", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer x")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("nil-db middleware must pass through, got %d", rr.Code)
	}
}

func TestAuditMiddleware_RecordsPanicsAs500(t *testing.T) {
	db, _ := setupStatsSQLiteTest(t)

	r := chi.NewRouter()
	// Recoverer outside AuditMiddleware (real production ordering) — the
	// audit row must still be written for a panicking handler.
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			defer func() {
				if p := recover(); p != nil {
					w.WriteHeader(http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, req)
		})
	})
	r.Use(AuditMiddleware(db.DB))
	r.Post("/api/test/boom", func(_ http.ResponseWriter, _ *http.Request) {
		panic("boom")
	})

	req := httptest.NewRequest(http.MethodPost, "/api/test/boom", nil)
	req.Header.Set("Authorization", "Bearer admin-token-abc")
	r.ServeHTTP(httptest.NewRecorder(), req)

	var status int
	var path string
	if err := db.Get(&status, "SELECT status FROM admin_audit_logs LIMIT 1"); err != nil {
		t.Fatalf("panic path must still be audited: %v", err)
	}
	if status != http.StatusInternalServerError {
		t.Fatalf("panic status = %d, want 500", status)
	}
	if err := db.Get(&path, "SELECT path FROM admin_audit_logs LIMIT 1"); err != nil {
		t.Fatalf("read path: %v", err)
	}
	if path != "/api/test/boom" {
		t.Fatalf("path = %q, want /api/test/boom", path)
	}
}
