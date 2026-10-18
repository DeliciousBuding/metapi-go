package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/internal/sqlitetest"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

const proxyURLReauthMasterToken = "proxy-url-reauth-master-token"

// setupProxyURLReauthTest wires the admin surfaces that persist egress proxy
// settings and publishes a runtime snapshot so ReauthConfirmed can compare the
// X-Admin-Confirm-Token header.
func setupProxyURLReauthTest(t *testing.T) (chi.Router, *store.DB) {
	t.Helper()
	db := sqlitetest.Open(t)
	cfg := &config.Config{
		AccountCredentialSecret: "test-secret-proxy-url",
		RequestBodyLimit:        20 * 1024 * 1024,
	}
	config.Set(cfg)
	config.SetRuntime(&config.RuntimeSettings{AuthToken: proxyURLReauthMasterToken})
	t.Cleanup(func() { config.SetRuntime(nil) })

	r := chi.NewRouter()
	RegisterSitesRoutes(r, db.DB)
	RegisterAccountsRoutes(r, db.DB, cfg)
	RegisterDownstreamKeysRoutes(r, db.DB)
	RegisterOauthRoutes(r, db.DB)
	RegisterSettingsRoutes(r, db.DB, cfg)
	return r, db
}

func doProxyURLRequest(t *testing.T, r chi.Router, method, path string, body any, confirm string) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if confirm != "" {
		req.Header.Set(auth.ReauthConfirmHeader, confirm)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func assertReauthRequired(t *testing.T, rec *httptest.ResponseRecorder, label string) {
	t.Helper()
	if rec.Code != http.StatusForbidden {
		t.Fatalf("%s without confirmation = %d, want 403: %s", label, rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"reauthRequired":true`) {
		t.Fatalf("%s rejection missing reauthRequired: %s", label, rec.Body.String())
	}
}

func decodeProxyURLResponse(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response %q: %v", rec.Body.String(), err)
	}
	return out
}

// proxyUrl redirects the credentials Metapi sends upstream, so a session alone
// must not be able to set one on a site, account, downstream key or OAuth
// connection. Clearing the field stays available without re-confirmation.
func TestProxyURLWritesRequireMasterToken(t *testing.T) {
	r, _ := setupProxyURLReauthTest(t)
	const validProxy = "http://proxy.example.com:3128"

	t.Run("site create", func(t *testing.T) {
		body := map[string]any{
			"name":     "Proxy URL Site",
			"url":      "https://proxy-url-site.example.com",
			"platform": "new-api",
			"proxyUrl": validProxy,
		}
		assertReauthRequired(t, doProxyURLRequest(t, r, http.MethodPost, "/api/sites", body, ""), "site create")
		rec := doProxyURLRequest(t, r, http.MethodPost, "/api/sites", body, proxyURLReauthMasterToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("site create with confirmation = %d: %s", rec.Code, rec.Body.String())
		}
		if got := decodeProxyURLResponse(t, rec)["proxyUrl"]; got != validProxy {
			t.Fatalf("site create proxyUrl = %v, want %s", got, validProxy)
		}
	})

	t.Run("site update", func(t *testing.T) {
		body := map[string]any{"proxyUrl": "http://proxy2.example.com:3128"}
		assertReauthRequired(t, doProxyURLRequest(t, r, http.MethodPut, "/api/sites/1", body, ""), "site update")
		rec := doProxyURLRequest(t, r, http.MethodPut, "/api/sites/1", body, proxyURLReauthMasterToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("site update with confirmation = %d: %s", rec.Code, rec.Body.String())
		}
		if got := decodeProxyURLResponse(t, rec)["proxyUrl"]; got != "http://proxy2.example.com:3128" {
			t.Fatalf("site update proxyUrl = %v", got)
		}
		// An ordinary metadata update is not an egress change.
		benign := doProxyURLRequest(t, r, http.MethodPut, "/api/sites/1", map[string]any{"name": "Renamed"}, "")
		if benign.Code != http.StatusOK {
			t.Fatalf("site rename without confirmation = %d: %s", benign.Code, benign.Body.String())
		}
		// Clearing the proxy is safe without confirmation.
		cleared := doProxyURLRequest(t, r, http.MethodPut, "/api/sites/1", map[string]any{"proxyUrl": ""}, "")
		if cleared.Code != http.StatusOK {
			t.Fatalf("site proxy clear without confirmation = %d: %s", cleared.Code, cleared.Body.String())
		}
	})
}

func TestProxyURLAccountWritesRequireMasterToken(t *testing.T) {
	r, _ := setupProxyURLReauthTest(t)
	const validProxy = "socks5h://proxy.example.com:1080"

	// Site + account fixtures: API-key mode with skipModelFetch avoids an
	// upstream call, which is unrelated to the reauth gate under test.
	site := decodeProxyURLResponse(t, doProxyURLRequest(t, r, http.MethodPost, "/api/sites", map[string]any{
		"name": "Account Site", "url": "https://api.openai.com",
	}, ""))
	siteID := int64(site["id"].(float64))

	createBody := map[string]any{
		"siteId":         siteID,
		"username":       "proxy-user",
		"accessToken":    "sk-proxy-fixture-token",
		"credentialMode": "apikey",
		"skipModelFetch": true,
		"proxyUrl":       validProxy,
	}
	assertReauthRequired(t, doProxyURLRequest(t, r, http.MethodPost, "/api/accounts", createBody, ""), "account create")
	created := doProxyURLRequest(t, r, http.MethodPost, "/api/accounts", createBody, proxyURLReauthMasterToken)
	if created.Code != http.StatusOK {
		t.Fatalf("account create with confirmation = %d: %s", created.Code, created.Body.String())
	}
	accountID := int64(decodeProxyURLResponse(t, created)["id"].(float64))

	updateBody := map[string]any{"proxyUrl": "http://account-proxy.example.com:8080"}
	assertReauthRequired(t, doProxyURLRequest(t, r, http.MethodPut, "/api/accounts/"+itoa(accountID), updateBody, ""), "account update")
	updated := doProxyURLRequest(t, r, http.MethodPut, "/api/accounts/"+itoa(accountID), updateBody, proxyURLReauthMasterToken)
	if updated.Code != http.StatusOK {
		t.Fatalf("account update with confirmation = %d: %s", updated.Code, updated.Body.String())
	}
	if got := decodeProxyURLResponse(t, updated)["extraConfig"]; !strings.Contains(toJSON(t, got), "account-proxy.example.com") {
		t.Fatalf("account update did not persist proxyUrl: %v", got)
	}

	// extraConfig is merged verbatim, so the nested spelling must be gated too.
	nested := map[string]any{"extraConfig": map[string]any{"proxyUrl": "http://nested-proxy.example.com:8080"}}
	assertReauthRequired(t, doProxyURLRequest(t, r, http.MethodPut, "/api/accounts/"+itoa(accountID), nested, ""), "account update nested proxyUrl")

	// Ordinary account edits are unaffected.
	benign := doProxyURLRequest(t, r, http.MethodPut, "/api/accounts/"+itoa(accountID), map[string]any{"isPinned": true}, "")
	if benign.Code != http.StatusOK {
		t.Fatalf("account pin without confirmation = %d: %s", benign.Code, benign.Body.String())
	}
}

func TestProxyURLDownstreamKeyWritesRequireMasterToken(t *testing.T) {
	r, _ := setupProxyURLReauthTest(t)
	const validProxy = "https://key-proxy.example.com:8443"

	createBody := map[string]any{
		"name":     "proxied key",
		"key":      "sk-proxy-url-key",
		"proxyUrl": validProxy,
	}
	assertReauthRequired(t, doProxyURLRequest(t, r, http.MethodPost, "/api/downstream-keys", createBody, ""), "key create")
	created := doProxyURLRequest(t, r, http.MethodPost, "/api/downstream-keys", createBody, proxyURLReauthMasterToken)
	if created.Code != http.StatusOK {
		t.Fatalf("key create with confirmation = %d: %s", created.Code, created.Body.String())
	}
	item := decodeProxyURLResponse(t, created)["item"].(map[string]any)
	keyID := int64(item["id"].(float64))

	updateBody := map[string]any{"proxyUrl": "http://key-proxy-2.example.com:3128"}
	assertReauthRequired(t, doProxyURLRequest(t, r, http.MethodPut, "/api/downstream-keys/"+itoa(keyID), updateBody, ""), "key update")
	updated := doProxyURLRequest(t, r, http.MethodPut, "/api/downstream-keys/"+itoa(keyID), updateBody, proxyURLReauthMasterToken)
	if updated.Code != http.StatusOK {
		t.Fatalf("key update with confirmation = %d: %s", updated.Code, updated.Body.String())
	}

	// Metadata-only edits no longer need the route-level blanket gate.
	benign := doProxyURLRequest(t, r, http.MethodPut, "/api/downstream-keys/"+itoa(keyID), map[string]any{"name": "renamed key"}, "")
	if benign.Code != http.StatusOK {
		t.Fatalf("key rename without confirmation = %d: %s", benign.Code, benign.Body.String())
	}
	// Rotating the bearer secret itself still needs re-confirmation.
	assertReauthRequired(t, doProxyURLRequest(t, r, http.MethodPut, "/api/downstream-keys/"+itoa(keyID), map[string]any{"key": "sk-rotated-key-value"}, ""), "key rotation")
	// Clearing the proxy is safe without confirmation.
	cleared := doProxyURLRequest(t, r, http.MethodPut, "/api/downstream-keys/"+itoa(keyID), map[string]any{"proxyUrl": ""}, "")
	if cleared.Code != http.StatusOK {
		t.Fatalf("key proxy clear without confirmation = %d: %s", cleared.Code, cleared.Body.String())
	}
}

func TestProxyURLRejectsInvalidValues(t *testing.T) {
	r, _ := setupProxyURLReauthTest(t)
	invalid := []string{
		"ftp://proxy.example.com:21",
		"socks4://proxy.example.com:1080",
		"http://",
		"proxy.example.com:8080",
		"http://proxy example.com:8080",
		"http://proxy.example.com/\nX: 1",
	}
	for _, value := range invalid {
		t.Run("site create/"+value, func(t *testing.T) {
			body := map[string]any{
				"name": "Invalid Proxy Site", "url": "https://invalid-proxy-site.example.com",
				"platform": "new-api", "proxyUrl": value,
			}
			rec := doProxyURLRequest(t, r, http.MethodPost, "/api/sites", body, proxyURLReauthMasterToken)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("site create proxyUrl %q = %d, want 400: %s", value, rec.Code, rec.Body.String())
			}
		})
		t.Run("system proxy/"+value, func(t *testing.T) {
			rec := doProxyURLRequest(t, r, http.MethodPut, "/api/settings/runtime", map[string]any{"systemProxyUrl": value}, proxyURLReauthMasterToken)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("systemProxyUrl %q = %d, want 400: %s", value, rec.Code, rec.Body.String())
			}
		})
	}

	// The accepted schemes keep working.
	for _, value := range []string{"http://proxy.example.com:3128", "https://proxy.example.com:8443", "socks5://proxy.example.com:1080", "socks5h://proxy.example.com:1080"} {
		rec := doProxyURLRequest(t, r, http.MethodPut, "/api/settings/runtime", map[string]any{"systemProxyUrl": value}, proxyURLReauthMasterToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("systemProxyUrl %q = %d, want 200: %s", value, rec.Code, rec.Body.String())
		}
	}
}

func TestProxyURLOAuthWritesRequireMasterToken(t *testing.T) {
	r, _ := setupProxyURLReauthTest(t)
	body := map[string]any{"proxyUrl": "http://oauth-proxy.example.com:3128"}
	assertReauthRequired(t, doProxyURLRequest(t, r, http.MethodPatch, "/api/oauth/connections/1/proxy", body, ""), "oauth proxy update")
	assertReauthRequired(t, doProxyURLRequest(t, r, http.MethodPost, "/api/oauth/connections/1/rebind", body, ""), "oauth rebind")
	assertReauthRequired(t, doProxyURLRequest(t, r, http.MethodPost, "/api/oauth/providers/claude/start", body, ""), "oauth start")

	// Invalid values are rejected before any provider work, even confirmed.
	bad := map[string]any{"proxyUrl": "ftp://oauth-proxy.example.com:21"}
	rec := doProxyURLRequest(t, r, http.MethodPatch, "/api/oauth/connections/1/proxy", bad, proxyURLReauthMasterToken)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("oauth invalid proxyUrl = %d, want 400: %s", rec.Code, rec.Body.String())
	}
}

func toJSON(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal %v: %v", v, err)
	}
	return string(raw)
}
