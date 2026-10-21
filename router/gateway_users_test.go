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

func TestGatewayUserManagementAuthenticationAndAudit(t *testing.T) {
	dataDir := t.TempDir()
	cfg := &config.Config{RequestBodyLimit: config.DefaultRequestBodyLimit, DataDir: dataDir}
	const token = "gateway-user-router-fixture"
	config.SetRuntime(&config.RuntimeSettings{AuthToken: token, DbType: store.DialectSQLite, DbUrl: filepath.Join(dataDir, "users.db")})
	t.Cleanup(func() { config.SetRuntime(nil) })
	if err := store.EnsureRuntimeDatabase(cfg, config.RuntimeSafe()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.CloseDatabase() })
	r := New(cfg, web.Dist)
	call := func(method, path, body string, bearer, confirmation bool, cookie *http.Cookie) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if bearer {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if confirmation {
			req.Header.Set("X-Admin-Confirm-Token", token)
		}
		if cookie != nil {
			req.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	const path = "/api/admin/users"
	const body = `{"username":"person","displayName":"Person","enabled":true}`
	if w := call("POST", path, body, false, true, nil); w.Code != 401 {
		t.Fatalf("confirmation bypass: %d", w.Code)
	}
	if w := call("POST", path, body, true, false, nil); w.Code != 403 {
		t.Fatalf("unconfirmed create: %d", w.Code)
	}
	if w := call("POST", path, body, true, true, nil); w.Code != 201 {
		t.Fatalf("default group create: %d %s", w.Code, w.Body.String())
	}
	login := call("POST", "/api/auth/login", `{"token":"`+token+`"}`, false, false, nil)
	if login.Code != 200 {
		t.Fatalf("admin login: %d", login.Code)
	}
	var session *http.Cookie
	for _, cookie := range login.Result().Cookies() {
		if cookie.Name == "metapi_session" {
			session = cookie
		}
	}
	if session == nil {
		t.Fatal("missing admin session")
	}
	if w := call("GET", path, "", false, false, session); w.Code != 200 || !strings.Contains(w.Body.String(), "person") {
		t.Fatalf("admin directory: %d %s", w.Code, w.Body.String())
	}
	if w := call("PUT", "/api/admin/users/1", `{"displayName":"Person","groupId":1,"enabled":false}`, false, false, session); w.Code != 403 {
		t.Fatalf("unconfirmed update: %d", w.Code)
	}
	var successes, rejections int
	if err := store.GetDB().Get(&successes, `SELECT COUNT(*) FROM admin_audit_logs WHERE path = ? AND status = 201`, path); err != nil || successes != 1 {
		t.Fatalf("create audit: %d %v", successes, err)
	}
	if err := store.GetDB().Get(&rejections, `SELECT COUNT(*) FROM admin_audit_logs WHERE path = ? AND status = 403`, path); err != nil || rejections != 1 {
		t.Fatalf("reject audit: %d %v", rejections, err)
	}
	var exposed int
	if err := store.GetDB().Get(&exposed, `SELECT COUNT(*) FROM admin_audit_logs WHERE actor LIKE ? OR path LIKE ?`, "%"+token+"%", "%"+token+"%"); err != nil || exposed != 0 {
		t.Fatalf("credential exposure: %d %v", exposed, err)
	}
}
