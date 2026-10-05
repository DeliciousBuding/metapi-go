package admin

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestAccounts_Login_NewAPIScopedPATIsReused(t *testing.T) {
	db, router, _ := setupAccountsTest(t)
	const pat = "nap_metapi-test-pat"
	const jwt = "short-lived-login-session"
	var created atomic.Int32
	var logins atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "POST /api/user/login":
			logins.Add(1)
			fmt.Fprint(w, `{"success":true,"data":{"access_token":"`+jwt+`","access_expires_at":1893456000,"session":{"sid":"login-session"}}}`)
		case "GET /api/user/access_tokens/catalog":
			fmt.Fprint(w, `{"success":true,"data":{"groups":[{"group":"personal","resources":[]}]}}`)
		case "POST /api/user/access_tokens":
			if r.Header.Get("X-Security-Proof") == "" {
				w.WriteHeader(http.StatusForbidden)
				fmt.Fprint(w, `{"success":false,"code":"SECURITY_PROOF_REQUIRED"}`)
				return
			}
			created.Add(1)
			fmt.Fprint(w, `{"success":true,"data":{"token":"`+pat+`"}}`)
		case "GET /api/verify/methods":
			fmt.Fprint(w, `{"success":true,"data":{"scope":"access_token.generate","methods":[{"method":"password","available":true}],"password_encryption_enabled":false}}`)
		case "POST /api/verify":
			fmt.Fprint(w, `{"success":true,"data":{"proof_token":"proof","method":"password","scope":"access_token.generate","expires_at":1893456000}}`)
		case "GET /api/user/access_tokens":
			ref := sha256.Sum256([]byte(pat))
			fmt.Fprintf(w, `{"success":true,"data":{"items":[{"token_ref":%q,"scopes":["api_key:read","api_key:reveal","api_key:write","profile:read","wallet:read","wallet:write"],"expires_at":0}]}}`, hex.EncodeToString(ref[:]))
		case "POST /api/user/auth/logout":
			fmt.Fprint(w, `{"success":true}`)
		case "GET /api/token/":
			fmt.Fprint(w, `{"success":true,"data":{"items":[],"total":0}}`)
		case "GET /v1/models":
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"error":"not a relay API key"}`)
		case "GET /api/user/self":
			fmt.Fprint(w, `{"success":true,"data":{"id":7,"username":"newapi-user","quota":1000000,"used_quota":0}}`)
		case "GET /api/user/models":
			fmt.Fprint(w, `{"success":true,"data":["gpt-4o-mini"]}`)
		case "GET /api/user/self/groups":
			fmt.Fprint(w, `{"success":true,"data":[]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	siteResponse := doPostJSON(t, router, "/api/sites", map[string]any{"name": "Current New API", "url": server.URL, "platform": "new-api"})
	if siteResponse.Code != http.StatusOK {
		t.Fatalf("create site: %d %s", siteResponse.Code, siteResponse.Body.String())
	}
	var site map[string]any
	if err := json.Unmarshal(siteResponse.Body.Bytes(), &site); err != nil {
		t.Fatal(err)
	}
	siteID := int64(site["id"].(float64))
	login := map[string]any{"siteId": siteID, "username": "newapi-user", "password": "test-password"}
	for attempt := 0; attempt < 2; attempt++ {
		resp := doPostJSON(t, router, "/api/accounts/login", login)
		if resp.Code != http.StatusOK {
			t.Fatalf("login %d: %d %s", attempt+1, resp.Code, resp.Body.String())
		}
		var result map[string]any
		if err := json.Unmarshal(resp.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result["reusedAccount"] != (attempt == 1) {
			t.Fatalf("reusedAccount on attempt %d = %v", attempt+1, result["reusedAccount"])
		}
	}
	if created.Load() != 1 || logins.Load() != 2 {
		t.Fatalf("PAT creates=%d, logins=%d, want 1/2", created.Load(), logins.Load())
	}
	var stored string
	if err := db.QueryRow("SELECT access_token FROM accounts WHERE site_id = ? AND username = ?", siteID, "newapi-user").Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(stored) != pat {
		t.Fatal("the durable PAT was not persisted and reused")
	}
}
