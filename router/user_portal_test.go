package router

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/deliciousbuding/metapi-go/web"
	"github.com/google/uuid"
)

const portalAdminToken = "synthetic-portal-admin-token"
const portalPassword = "synthetic-user-password"

type portalFixture struct {
	t      *testing.T
	router http.Handler
	db     *store.DB
	user   store.GatewayUser
	group  store.GatewayUserGroup
	nextIP int
}

func newPortalFixture(t *testing.T, dialect string) *portalFixture {
	t.Helper()
	dataDir := t.TempDir()
	dsn := filepath.Join(dataDir, "portal.db")
	if dialect == store.DialectPostgres {
		dsn = os.Getenv("PG_TEST_DSN")
		if dsn == "" {
			t.Skip("PG_TEST_DSN not set")
		}
		control, err := store.Open(dialect, dsn, false)
		if err != nil {
			t.Fatal(err)
		}
		schema := "user_portal_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		if _, err := control.Exec("CREATE SCHEMA " + schema); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if _, err := control.Exec("DROP SCHEMA " + schema + " CASCADE"); err != nil {
				t.Errorf("drop own schema: %v", err)
			}
			_ = control.Close()
		})
		parsed, err := url.Parse(dsn)
		if err != nil {
			t.Fatal(err)
		}
		q := parsed.Query()
		q.Set("search_path", schema)
		parsed.RawQuery = q.Encode()
		dsn = parsed.String()
	}
	cfg := &config.Config{RequestBodyLimit: config.DefaultRequestBodyLimit, DataDir: dataDir, AdminRateLimitRPS: 1000, AdminRateLimitBurst: 1000, AuthRateLimitRPS: 1000, AuthRateLimitBurst: 1000}
	config.SetRuntime(&config.RuntimeSettings{AuthToken: portalAdminToken, DbType: dialect, DbUrl: dsn})
	t.Cleanup(func() { config.SetRuntime(nil) })
	if err := store.EnsureRuntimeDatabase(cfg, config.RuntimeSafe()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.CloseDatabase() })
	f := &portalFixture{t: t, db: store.GetDB()}
	f.router = New(cfg, web.Dist)
	users := store.NewGatewayUserStore(f.db)
	var err error
	f.group, err = users.CreateUserGroup(context.Background(), "portal", "Portal", true)
	if err != nil {
		t.Fatal(err)
	}
	f.user, err = users.CreateUser(context.Background(), "alice", "Alice", f.group.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	return f
}
func (f *portalFixture) call(method, path, body string, cookie *http.Cookie, headers map[string]string) *httptest.ResponseRecorder {
	f.t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	f.nextIP++
	req.RemoteAddr = fmt.Sprintf("192.0.2.%d:1234", f.nextIP%250+1)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, req)
	return w
}
func portalJSON(t *testing.T, w *httptest.ResponseRecorder, status int) map[string]any {
	t.Helper()
	if w.Code != status {
		t.Fatalf("HTTP %d, want %d: %s", w.Code, status, w.Body.String())
	}
	var data map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
		t.Fatal(err)
	}
	return data
}
func (f *portalFixture) issue() string {
	f.t.Helper()
	w := f.call("POST", fmt.Sprintf("/api/admin/users/%d/activation", f.user.ID), "{}", nil, map[string]string{"Authorization": "Bearer " + portalAdminToken, "X-Admin-Confirm-Token": portalAdminToken})
	data := portalJSON(f.t, w, 200)
	if !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
		f.t.Fatal("activation cacheable")
	}
	if _, err := time.Parse(time.RFC3339Nano, data["expiresAt"].(string)); err != nil {
		f.t.Fatal(err)
	}
	return data["code"].(string)
}
func (f *portalFixture) activate(code string) *http.Cookie {
	f.t.Helper()
	w := f.call("POST", "/api/user/auth/activate", `{"username":"alice","code":"`+code+`","password":"`+portalPassword+`"}`, nil, nil)
	portalJSON(f.t, w, 200)
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == auth.UserSessionCookieName {
			if !cookie.HttpOnly || cookie.Path != "/api/user" || cookie.SameSite != http.SameSiteStrictMode {
				f.t.Fatal("unsafe user cookie")
			}
			return cookie
		}
	}
	f.t.Fatal("missing user cookie")
	return nil
}

func TestUserPortalRouterAuthenticationIsolation(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			f := newPortalFixture(t, dialect)
			path := fmt.Sprintf("/api/admin/users/%d/activation", f.user.ID)
			portalJSON(t, f.call("POST", path, "{}", nil, map[string]string{"X-Admin-Confirm-Token": portalAdminToken}), 401)
			portalJSON(t, f.call("POST", path, "{}", nil, map[string]string{"Authorization": "Bearer " + portalAdminToken}), 403)
			code := f.issue()
			cookie := f.activate(code)
			session := portalJSON(t, f.call("GET", "/api/user/session", "", cookie, nil), 200)
			if len(session) != 3 || session["user"].(map[string]any)["username"] != "alice" {
				t.Fatal("unexpected session DTO")
			}
			portalJSON(t, f.call("POST", "/api/user/auth/activate", `{"username":"alice","code":"`+code+`","password":"`+portalPassword+`"}`, nil, nil), 401)
			// Neither an ordinary cookie nor a confirmation header is admin auth.
			for _, headers := range []map[string]string{nil, {"X-Admin-Confirm-Token": portalAdminToken}, {"Authorization": "Bearer " + cookie.Value}, {"Authorization": "Bearer sk-user-fix"}} {
				want := 401
				if headers["Authorization"] != "" {
					want = 403
				}
				portalJSON(t, f.call("GET", "/api/admin/users", "", cookie, headers), want)
			}
			portalJSON(t, f.call("GET", "/api/admin/users", "", &http.Cookie{Name: auth.SessionCookieName, Value: cookie.Value}, nil), 401)
			adminLogin := f.call("POST", "/api/auth/login", `{"token":"`+portalAdminToken+`"}`, nil, nil)
			portalJSON(t, adminLogin, 200)
			var adminCookie *http.Cookie
			for _, c := range adminLogin.Result().Cookies() {
				if c.Name == auth.SessionCookieName {
					adminCookie = c
				}
			}
			if adminCookie == nil {
				t.Fatal("missing admin cookie")
			}
			portalJSON(t, f.call("GET", "/api/user/session", "", adminCookie, nil), 401)
			portalJSON(t, f.call("GET", "/api/user/session", "", nil, map[string]string{"Authorization": "Bearer " + portalAdminToken}), 401)
			// The same hostname on another scheme/port is not the same origin.
			for _, origin := range []string{"https://evil.example", "null", "https://example.com", "http://example.com:99"} {
				portalJSON(t, f.call("POST", "/api/user/keys", `{"name":"bad"}`, cookie, map[string]string{"Origin": origin}), 403)
			}
			portalJSON(t, f.call("POST", "/api/user/keys", `{"name":"bad"}`, cookie, map[string]string{"Sec-Fetch-Site": "same-site"}), 403)
			portalJSON(t, f.call("POST", "/api/user/auth/login", `{"username":"alice","password":"`+portalPassword+`"}`, nil, map[string]string{"Origin": "https://evil.example"}), 403)
			portalJSON(t, f.call("POST", "/api/user/auth/password", `{"currentPassword":"wrong","newPassword":"synthetic-new-password"}`, cookie, nil), 401)
			portalJSON(t, f.call("POST", "/api/user/auth/password", `{"currentPassword":"`+portalPassword+`","newPassword":"synthetic-new-password"}`, cookie, nil), 200)
			portalJSON(t, f.call("GET", "/api/user/session", "", cookie, nil), 401)
			login := f.call("POST", "/api/user/auth/login", `{"username":"alice","password":"synthetic-new-password"}`, nil, nil)
			portalJSON(t, login, 200)
			var next *http.Cookie
			for _, c := range login.Result().Cookies() {
				if c.Name == auth.UserSessionCookieName {
					next = c
				}
			}
			if next == nil {
				t.Fatal("missing new cookie")
			}
			reset := f.issue()
			if reset == code {
				t.Fatal("code reused")
			}
			portalJSON(t, f.call("GET", "/api/user/session", "", next, nil), 401)
			portalJSON(t, f.call("POST", "/api/user/auth/login", `{"username":"alice","password":"synthetic-new-password"}`, nil, nil), 401)
			// Credentials and codes never appear in ordinary admin GET or audit.
			got := f.call("GET", "/api/admin/users", "", adminCookie, nil)
			portalJSON(t, got, 200)
			for _, secret := range []string{code, reset, portalPassword, next.Value} {
				if strings.Contains(got.Body.String(), secret) {
					t.Fatal("credential exposed")
				}
			}
			var successes int
			if err := f.db.Get(&successes, `SELECT COUNT(*) FROM admin_audit_logs WHERE path=? AND status=200`, path); err != nil || successes != 2 {
				t.Fatalf("activation audit: %d %v", successes, err)
			}
			var exposed int
			if err := f.db.Get(&exposed, `SELECT COUNT(*) FROM admin_audit_logs WHERE path LIKE ? OR actor LIKE ?`, "%"+reset+"%", "%"+reset+"%"); err != nil || exposed != 0 {
				t.Fatalf("activation secret audit: %d %v", exposed, err)
			}
		})
	}
}

func TestUserPortalRouterOwnerAndRevocation(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			f := newPortalFixture(t, dialect)
			cookie := f.activate(f.issue())
			ctx := context.Background()
			users := store.NewGatewayUserStore(f.db)
			other, err := users.CreateUser(ctx, "bob", "Bob", f.group.ID, true)
			if err != nil {
				t.Fatal(err)
			}
			otherKey, err := users.CreateOwnKey(ctx, other.ID, "Foreign Key", "sk-foreign-fix", []string{"*"}, nil, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, extra := range []string{`"ownerUserId":2`, `"groupId":2`, `"allowedRouteIds":[1]`, `"proxyUrl":"http://internal.example"`, `"Key":"sk-chosen"`, `"supportedModels":[]`, `"maxRequests":-1`, `"expiresAt":"bad"`} {
				portalJSON(t, f.call("POST", "/api/user/keys", `{"name":"test",`+extra+`}`, cookie, nil), 400)
			}
			created := portalJSON(t, f.call("POST", "/api/user/keys", `{"name":"Own Key"}`, cookie, map[string]string{"Origin": "http://example.com"}), 201)
			key := created["key"].(string)
			item := created["item"].(map[string]any)
			keyID := int64(item["id"].(float64))
			if item["key"] != nil || item["keyMasked"] == key || len(item) != 11 {
				t.Fatalf("unsafe key DTO: %+v", item)
			}
			got := f.call("GET", "/api/user/keys", "", cookie, nil)
			listed := portalJSON(t, got, 200)
			if strings.Contains(got.Body.String(), key) || strings.Contains(got.Body.String(), "Foreign Key") || len(listed["items"].([]any)) != 1 {
				t.Fatal("key ownership or plaintext leak")
			}
			portalJSON(t, f.call("GET", "/api/user/keys", "", nil, map[string]string{"Authorization": "Bearer " + key}), 401)
			portalJSON(t, f.call("DELETE", "/api/user/keys/"+strconv.FormatInt(otherKey.ID, 10), "", cookie, nil), 404)
			portalJSON(t, f.call("POST", "/api/user/keys/"+strconv.FormatInt(otherKey.ID, 10)+"/rotate", "{}", cookie, nil), 404)
			portalJSON(t, f.call("POST", "/api/user/keys/bad/rotate", "{}", cookie, nil), 400)
			if _, err := f.db.Exec(`UPDATE downstream_api_keys SET used_requests=3,used_cost=2 WHERE id=?`, keyID); err != nil {
				t.Fatal(err)
			}
			rotated := portalJSON(t, f.call("POST", "/api/user/keys/"+strconv.FormatInt(keyID, 10)+"/rotate", "{}", cookie, map[string]string{"Origin": "http://example.com"}), 200)
			rotatedKey := rotated["key"].(string)
			rotatedItem := rotated["item"].(map[string]any)
			if rotatedKey == key || rotatedItem["id"] != item["id"] || rotatedItem["name"] != item["name"] || rotatedItem["keyMasked"] == item["keyMasked"] || rotatedItem["usedRequests"] != float64(3) || rotatedItem["usedCost"] != float64(2) || rotatedItem["key"] != nil {
				t.Fatalf("unsafe rotation DTO: %+v", rotated)
			}
			if old := auth.AuthorizeDownstreamToken(key, nil); old.OK || old.StatusCode != 401 {
				t.Fatalf("old key remained valid: %+v", old)
			}
			if next := auth.AuthorizeDownstreamToken(rotatedKey, nil); !next.OK || next.Key == nil || next.Key.ID != keyID {
				t.Fatalf("rotated key did not authenticate: %+v", next)
			}
			afterRotate := f.call("GET", "/api/user/keys", "", cookie, nil)
			if strings.Contains(afterRotate.Body.String(), key) || strings.Contains(afterRotate.Body.String(), rotatedKey) || !strings.Contains(afterRotate.Body.String(), rotatedItem["keyMasked"].(string)) {
				t.Fatalf("rotation plaintext or mask leak: %s", afterRotate.Body.String())
			}
			usage := portalJSON(t, f.call("GET", "/api/user/usage", "", cookie, nil), 200)
			if usage["usedRequests"] != float64(3) || usage["keyCount"] != float64(1) {
				t.Fatal("usage ownership")
			}
			now := time.Now().UTC().Format(time.RFC3339Nano)
			for i, owner := range []*int64{&f.user.ID, &other.ID, nil} {
				if err := store.InsertGatewayRequest(ctx, f.db, store.GatewayRequest{ID: fmt.Sprintf("req-%d", i), OwnerUserID: owner, Transport: "http", TerminalStatus: "completed", StartedAt: now, EndedAt: now}); err != nil {
					t.Fatal(err)
				}
			}
			requests := portalJSON(t, f.call("GET", "/api/user/requests?page=2&pageSize=1", "", cookie, nil), 200)
			if requests["total"] != float64(1) || len(requests["items"].([]any)) != 0 {
				t.Fatal("total owner or page leak")
			}
			request := portalJSON(t, f.call("GET", "/api/user/requests/req-0", "", cookie, nil), 200)
			if len(request) != 8 || request["id"] != "req-0" || request["quota"] != nil {
				t.Fatal("request DTO")
			}
			for _, id := range []string{"req-1", "req-2", "missing"} {
				portalJSON(t, f.call("GET", "/api/user/requests/"+id, "", cookie, nil), 404)
			}
			for _, query := range []string{"page=0", "pageSize=101", "page=-1", "page=9999999999999999999"} {
				portalJSON(t, f.call("GET", "/api/user/requests?"+query, "", cookie, nil), 400)
			}
			for _, target := range []string{"gateway_users", "gateway_user_groups"} {
				id := f.user.ID
				if target == "gateway_user_groups" {
					id = f.group.ID
				}
				if _, err := f.db.Exec("UPDATE "+target+" SET enabled=FALSE WHERE id=?", id); err != nil {
					t.Fatal(err)
				}
				portalJSON(t, f.call("GET", "/api/user/session", "", cookie, nil), 401)
				portalJSON(t, f.call("POST", "/api/user/auth/login", `{"username":"alice","password":"`+portalPassword+`"}`, nil, nil), 401)
				portalJSON(t, f.call("POST", fmt.Sprintf("/api/admin/users/%d/activation", f.user.ID), "{}", nil, map[string]string{"Authorization": "Bearer " + portalAdminToken, "X-Admin-Confirm-Token": portalAdminToken}), 404)
				if _, err := f.db.Exec("UPDATE "+target+" SET enabled=TRUE WHERE id=?", id); err != nil {
					t.Fatal(err)
				}
			}
			portalJSON(t, f.call("GET", "/api/user/session", "", cookie, nil), 200)
			portalJSON(t, f.call("DELETE", "/api/user/keys/"+strconv.FormatInt(keyID, 10), "", cookie, nil), 200)
			portalJSON(t, f.call("POST", "/api/user/auth/logout", "{}", cookie, nil), 200)
			portalJSON(t, f.call("GET", "/api/user/session", "", cookie, nil), 401)
		})
	}
}

func TestUserPortalRouterSessionExpiryAndRateLimit(t *testing.T) {
	f := newPortalFixture(t, store.DialectSQLite)
	cookie := f.activate(f.issue())
	for _, column := range []string{"expires_at", "idle_expires_at"} {
		if _, err := f.db.Exec("UPDATE gateway_user_sessions SET "+column+"=?", "2000-01-01T00:00:00.000000000Z"); err != nil {
			t.Fatal(err)
		}
		portalJSON(t, f.call("GET", "/api/user/session", "", cookie, nil), 401)
		if _, err := f.db.Exec("UPDATE gateway_user_sessions SET "+column+"=?", "2999-01-01T00:00:00.000000000Z"); err != nil {
			t.Fatal(err)
		}
	}
	// Same-IP bad bodies consume the pre-auth bucket without bcrypt work.
	for i := 0; i < 6; i++ {
		req := httptest.NewRequest("POST", "/api/user/auth/login", strings.NewReader(`{"ownerUserId":1}`))
		req.RemoteAddr = "198.51.100.1:1234"
		w := httptest.NewRecorder()
		f.router.ServeHTTP(w, req)
		want := 400
		if i == 5 {
			want = 429
		}
		portalJSON(t, w, want)
	}
}
