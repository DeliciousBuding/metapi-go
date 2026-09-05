package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// Observed failure (v0.19 stability window, reproduced against a real New API
// upstream and a real relay): the operator rotated their upstream API key —
// deleted the old one, created a new one. Re-logging the account in is the only
// recovery step the docs teach, and it answered success:true while relay stayed
// dead: the revoked key remained the account's default credential, so
// accounts.api_token kept the dead value, the model refresh failed with
// "API key is invalid", the route rebuild was skipped along with it, and the
// chain only came back after a manual set-default plus a manual route rebuild.
//
// This test drives the same sequence through the login handler: after the
// upstream rotates the key, one more login must leave the account relaying with
// the key the upstream still lists, and must say so in its own response.
func TestAccounts_LoginConvergesDefaultRelayTokenAfterUpstreamRotation(t *testing.T) {
	db, r, _ := setupAccountsTest(t)

	var rotated atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/api/user/login":
			fmt.Fprint(w, `{"success":true,"data":{"access_token":"dashboard-pat"}}`)
		case req.Method == http.MethodGet && strings.HasPrefix(req.URL.Path, "/api/token/"):
			if rotated.Load() {
				fmt.Fprint(w, `{"success":true,"data":{"items":[{"id":2,"name":"metapi-aged-2","key":"sk-live-key","status":1}]}}`)
				return
			}
			fmt.Fprint(w, `{"success":true,"data":{"items":[{"id":1,"name":"metapi-aged","key":"sk-revoked-key","status":1}]}}`)
		case req.Method == http.MethodGet && req.URL.Path == "/v1/models":
			// The relay key is the only credential that can list models here, so a
			// stale default is visible as a failed refresh — exactly what the real
			// upstream answered ("model fetch failed: API key is invalid"). The key
			// the upstream honours is the one it still lists.
			liveKey := "sk-revoked-key"
			if rotated.Load() {
				liveKey = "sk-live-key"
			}
			if req.Header.Get("Authorization") == "Bearer "+liveKey {
				fmt.Fprint(w, `{"object":"list","data":[{"id":"gpt-4o-mini","object":"model"},{"id":"claude-3-5-sonnet-20241022","object":"model"}]}`)
				return
			}
			http.Error(w, `{"error":{"message":"Invalid token","type":"new_api_error"}}`, http.StatusUnauthorized)
		case req.Method == http.MethodGet && req.URL.Path == "/api/user/self":
			fmt.Fprint(w, `{"success":true,"data":{"id":1,"username":"root"}}`)
		default:
			http.NotFound(w, req)
		}
	}))
	t.Cleanup(upstream.Close)

	siteResp := doPostJSON(t, r, "/api/sites", map[string]any{
		"name":     "token rotation",
		"url":      upstream.URL,
		"platform": "new-api",
	})
	if siteResp.Code != http.StatusOK {
		t.Fatalf("create site: %d %s", siteResp.Code, siteResp.Body.String())
	}
	var site map[string]any
	if err := json.Unmarshal(siteResp.Body.Bytes(), &site); err != nil {
		t.Fatalf("decode site: %v", err)
	}
	siteID := int64(site["id"].(float64))

	firstLogin := doPostJSON(t, r, "/api/accounts/login", map[string]any{
		"siteId": siteID, "username": "root", "password": "metapi123",
	})
	if firstLogin.Code != http.StatusOK {
		t.Fatalf("first login: %d %s", firstLogin.Code, firstLogin.Body.String())
	}
	var first map[string]any
	if err := json.Unmarshal(firstLogin.Body.Bytes(), &first); err != nil {
		t.Fatalf("decode first login: %v", err)
	}
	accountID := int64(first["account"].(map[string]any)["id"].(float64))
	if refresh, _ := first["modelRefresh"].(map[string]any); refresh == nil || refresh["success"] != true {
		t.Fatalf("baseline modelRefresh = %#v, want success before the rotation", first["modelRefresh"])
	}

	// The operator rotates the upstream key: the old one stops existing.
	rotated.Store(true)

	secondLogin := doPostJSON(t, r, "/api/accounts/login", map[string]any{
		"siteId": siteID, "username": "root", "password": "metapi123",
	})
	if secondLogin.Code != http.StatusOK {
		t.Fatalf("second login: %d %s", secondLogin.Code, secondLogin.Body.String())
	}
	var second map[string]any
	if err := json.Unmarshal(secondLogin.Body.Bytes(), &second); err != nil {
		t.Fatalf("decode second login: %v", err)
	}

	// 1. The credential the account relays with is the key upstream still lists.
	var apiToken *string
	if err := db.Get(&apiToken, `SELECT api_token FROM accounts WHERE id = ?`, accountID); err != nil {
		t.Fatalf("read accounts.api_token: %v", err)
	}
	if apiToken == nil || *apiToken != "sk-live-key" {
		t.Fatalf("accounts.api_token = %v, want sk-live-key after the documented recovery step", apiToken)
	}

	// 2. The default moved, and the revoked row is still there as a record.
	var defaultName string
	if err := db.Get(&defaultName, `SELECT name FROM account_tokens WHERE account_id = ? AND is_default = TRUE`, accountID); err != nil {
		t.Fatalf("read default token: %v", err)
	}
	if defaultName != "metapi-aged-2" {
		t.Fatalf("default token = %q, want metapi-aged-2", defaultName)
	}
	var revokedRows int
	if err := db.Get(&revokedRows, `SELECT COUNT(*) FROM account_tokens WHERE account_id = ? AND token = 'sk-revoked-key'`, accountID); err != nil {
		t.Fatalf("count revoked token rows: %v", err)
	}
	if revokedRows != 1 {
		t.Fatalf("revoked token rows = %d, want 1 (converged, not deleted)", revokedRows)
	}

	// 3. The loop closes on its own: with a live default the model refresh
	//    succeeds, which is what makes the route rebuild run instead of being
	//    skipped along with the failure.
	refresh, _ := second["modelRefresh"].(map[string]any)
	if refresh == nil || refresh["success"] != true {
		t.Fatalf("modelRefresh after rotation = %#v, want success (the refresh must use the converged credential)", second["modelRefresh"])
	}

	// 4. The response says the credential moved. An operator who rotated a key
	//    upstream has to be able to see that Metapi followed, without reading the
	//    database.
	message, _ := second["tokenSyncMessage"].(string)
	if !strings.Contains(message, "default relay token switched") || !strings.Contains(message, "metapi-aged-2") {
		t.Fatalf("tokenSyncMessage = %q, want it to name the switch and the key now in use", message)
	}
}
