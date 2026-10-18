package admin

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

func setupAuthSettingsTest(t *testing.T) (*store.DB, chi.Router) {
	t.Helper()
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := store.AutoMigrate(db); err != nil {
		db.Close()
		t.Fatalf("AutoMigrate: %v", err)
	}

	config.SetRuntime(&config.RuntimeSettings{AuthToken: "admin-auth-settings-token"})
	t.Cleanup(func() { config.SetRuntime(nil) })
	r := chi.NewRouter()
	RegisterAuthSettingsRoutes(r, db.DB, nil)
	return db, r
}

func TestAuthSettingsChange_WrongOldTokenForbidden(t *testing.T) {
	_, r := setupAuthSettingsTest(t)

	resp := doPostJSON(t, r, "/api/settings/auth/change", map[string]any{
		"oldToken": "wrong-old-token-value",
		"newToken": "brand-new-token-1",
	})
	if resp.Code != http.StatusForbidden {
		t.Fatalf("status = %d body=%s, want 403", resp.Code, resp.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["error"] != "Old token verification failed" {
		t.Fatalf("error = %v, want Old token verification failed", body["error"])
	}
	if config.Runtime().AuthToken != "admin-auth-settings-token" {
		t.Fatalf("AuthToken mutated to %q", config.Runtime().AuthToken)
	}
}

func TestAuthSettingsChange_WrongOldTokenDifferentLengthForbidden(t *testing.T) {
	_, r := setupAuthSettingsTest(t)

	resp := doPostJSON(t, r, "/api/settings/auth/change", map[string]any{
		"oldToken": "short",
		"newToken": "brand-new-token-2",
	})
	if resp.Code != http.StatusForbidden {
		t.Fatalf("status = %d body=%s, want 403", resp.Code, resp.Body.String())
	}
	if config.Runtime().AuthToken != "admin-auth-settings-token" {
		t.Fatalf("AuthToken mutated to %q", config.Runtime().AuthToken)
	}
}

func TestAuthSettingsChange_SuccessUpdatesToken(t *testing.T) {
	db, r := setupAuthSettingsTest(t)

	resp := doPostJSON(t, r, "/api/settings/auth/change", map[string]any{
		"oldToken": "admin-auth-settings-token",
		"newToken": "rotated-admin-token",
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", resp.Code, resp.Body.String())
	}
	if config.Runtime().AuthToken != "rotated-admin-token" {
		t.Fatalf("AuthToken = %q, want rotated-admin-token", config.Runtime().AuthToken)
	}

	var stored string
	if err := db.Get(&stored, "SELECT value FROM settings WHERE key = 'auth_token'"); err != nil {
		t.Fatalf("load auth_token setting: %v", err)
	}
	if stored != `"rotated-admin-token"` {
		t.Fatalf("stored auth_token = %q, want JSON-quoted rotated-admin-token", stored)
	}
}

func TestAuthSettingsChange_SuccessClearsMonitorAuthCookies(t *testing.T) {
	_, r := setupAuthSettingsTest(t)

	resp := doPostJSON(t, r, "/api/settings/auth/change", map[string]any{
		"oldToken": "admin-auth-settings-token",
		"newToken": "rotated-admin-token-clear",
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", resp.Code, resp.Body.String())
	}

	// Wire headers must expire both scoped and legacy paths (Max-Age=0).
	headers := resp.Header().Values("Set-Cookie")
	if len(headers) < 2 {
		t.Fatalf("Set-Cookie count = %d, want >= 2 (scoped + legacy); headers=%v", len(headers), headers)
	}
	joined := strings.Join(headers, " | ")
	if !strings.Contains(joined, monitorAuthCookie+"=") {
		t.Fatalf("Set-Cookie missing %s: %s", monitorAuthCookie, joined)
	}
	if !strings.Contains(joined, "Max-Age=0") {
		t.Fatalf("Set-Cookie missing Max-Age=0: %s", joined)
	}

	var scoped *http.Cookie
	var legacyRoot *http.Cookie
	for _, c := range resp.Result().Cookies() {
		if c.Name != monitorAuthCookie {
			continue
		}
		switch c.Path {
		case monitorCookiePath:
			scoped = c
		case "/":
			legacyRoot = c
		}
	}
	if scoped == nil {
		t.Fatalf("missing clear cookie for Path=%q; cookies=%v", monitorCookiePath, resp.Result().Cookies())
	}
	if scoped.MaxAge >= 0 {
		t.Fatalf("scoped clear MaxAge = %d, want < 0 (Max-Age=0 on wire)", scoped.MaxAge)
	}
	if scoped.Value != "" {
		t.Fatalf("scoped clear Value = %q, want empty", scoped.Value)
	}
	if !scoped.HttpOnly {
		t.Fatal("scoped clear cookie must remain HttpOnly")
	}
	if legacyRoot == nil {
		t.Fatal("missing legacy Path=/ clear cookie")
	}
	if legacyRoot.MaxAge >= 0 {
		t.Fatalf("legacy clear MaxAge = %d, want < 0", legacyRoot.MaxAge)
	}
}

func TestAuthSettingsChange_WrongOldTokenDoesNotClearMonitorAuthCookies(t *testing.T) {
	_, r := setupAuthSettingsTest(t)

	resp := doPostJSON(t, r, "/api/settings/auth/change", map[string]any{
		"oldToken": "wrong-old-token-value",
		"newToken": "brand-new-token-no-clear",
	})
	if resp.Code != http.StatusForbidden {
		t.Fatalf("status = %d body=%s, want 403", resp.Code, resp.Body.String())
	}
	if config.Runtime().AuthToken != "admin-auth-settings-token" {
		t.Fatalf("AuthToken mutated to %q", config.Runtime().AuthToken)
	}
	assertNoMonitorAuthClearCookies(t, resp)
}

func TestAuthSettingsChange_ShortNewTokenDoesNotClearMonitorAuthCookies(t *testing.T) {
	_, r := setupAuthSettingsTest(t)

	resp := doPostJSON(t, r, "/api/settings/auth/change", map[string]any{
		"oldToken": "admin-auth-settings-token",
		"newToken": "short",
	})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s, want 400", resp.Code, resp.Body.String())
	}
	if config.Runtime().AuthToken != "admin-auth-settings-token" {
		t.Fatalf("AuthToken mutated to %q", config.Runtime().AuthToken)
	}
	assertNoMonitorAuthClearCookies(t, resp)
}

func assertNoMonitorAuthClearCookies(t *testing.T, resp interface{ Header() http.Header }) {
	t.Helper()
	headers := resp.Header().Values("Set-Cookie")
	for _, h := range headers {
		if strings.Contains(h, monitorAuthCookie) {
			t.Fatalf("failed auth change must not clear %s; Set-Cookie=%v", monitorAuthCookie, headers)
		}
	}
}

func TestAuthSettingsInfo_DoesNotEchoToken(t *testing.T) {
	_, r := setupAuthSettingsTest(t)

	resp := doGet(t, r, "/api/settings/auth/info")
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", resp.Code, resp.Body.String())
	}
	if got := resp.Header().Get("Cache-Control"); got != noStoreCacheControl {
		t.Fatalf("Cache-Control = %q, want %q", got, noStoreCacheControl)
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if _, ok := body["masked"]; ok {
		t.Fatalf("response still carries a masked token: %#v", body)
	}
	if body["customized"] != true {
		t.Fatalf("customized = %v, want true", body["customized"])
	}
	if body["configured"] != true {
		t.Fatalf("configured = %v, want true", body["configured"])
	}
	if body["length"] != float64(len("admin-auth-settings-token")) {
		t.Fatalf("length = %v, want %d", body["length"], len("admin-auth-settings-token"))
	}
	raw := resp.Body.String()
	if strings.Contains(raw, "admin-auth-settings-token") || strings.Contains(raw, "dmin") || strings.Contains(raw, "oken") {
		t.Fatalf("response leaked token fragments: %s", raw)
	}
}

func TestAuthSettingsInfo_ReportsDefaultToken(t *testing.T) {
	_, r := setupAuthSettingsTest(t)
	config.SetRuntime(&config.RuntimeSettings{AuthToken: config.DefaultAuthToken})

	resp := doGet(t, r, "/api/settings/auth/info")
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["customized"] != false {
		t.Fatalf("customized = %v, want false", body["customized"])
	}
	if body["configured"] != true {
		t.Fatalf("configured = %v, want true", body["configured"])
	}
	if strings.Contains(resp.Body.String(), config.DefaultAuthToken) {
		t.Fatalf("response leaked the default token: %s", resp.Body.String())
	}
}

func TestConstantTimeTokenEqual(t *testing.T) {
	if !constantTimeTokenEqual("same-token", "same-token") {
		t.Fatal("expected equal tokens to match")
	}
	if constantTimeTokenEqual("same-token", "diff-token") {
		t.Fatal("expected same-length mismatch to fail")
	}
	if constantTimeTokenEqual("short", "much-longer-token") {
		t.Fatal("expected length mismatch to fail")
	}
}

// Rotation without a caller-supplied secret must generate a 32-byte token,
// publish it, persist it, and return it exactly once.
func TestAuthSettingsChange_GeneratesStrongTokenWhenOmitted(t *testing.T) {
	db, r := setupAuthSettingsTest(t)

	resp := doPostJSON(t, r, "/api/settings/auth/change", map[string]any{
		"oldToken": "admin-auth-settings-token",
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	generated, _ := body["newToken"].(string)
	if len(generated) != 64 {
		t.Fatalf("generated token length = %d, want 64 hex characters", len(generated))
	}
	for _, ch := range generated {
		if !strings.ContainsRune("0123456789abcdef", ch) {
			t.Fatalf("generated token contains non-hex rune %q", ch)
		}
	}
	if config.Runtime().AuthToken != generated {
		t.Fatal("runtime token was not updated to the generated value")
	}
	var stored string
	if err := db.Get(&stored, "SELECT value FROM settings WHERE key = 'auth_token'"); err != nil {
		t.Fatalf("read persisted token: %v", err)
	}
	if stored != `"`+generated+`"` {
		t.Fatalf("persisted token = %q, want the JSON-encoded generated value", stored)
	}

	// The previous master token no longer verifies.
	resp = doPostJSON(t, r, "/api/settings/auth/change", map[string]any{
		"oldToken": "admin-auth-settings-token",
	})
	if resp.Code != http.StatusForbidden {
		t.Fatalf("old token replay = %d, want 403", resp.Code)
	}
}

// An explicit replacement secret is still accepted for automation, but only
// above the strength floor the startup check enforces.
func TestAuthSettingsChange_RejectsShortCustomToken(t *testing.T) {
	_, r := setupAuthSettingsTest(t)

	resp := doPostJSON(t, r, "/api/settings/auth/change", map[string]any{
		"oldToken": "admin-auth-settings-token",
		"newToken": "fifteen-bytes!!",
	})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s, want 400", resp.Code, resp.Body.String())
	}
}
