package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// #1179: binding an account used to stop after credential/token sync. The
// account therefore reopened with totalCount=0 and any route built immediately
// afterwards had no model-backed channel until the periodic model scheduler ran.
// Login must leave a usable model inventory now, while its verified credential
// is freshest.
func TestAccounts_LoginImmediatelyDiscoversAndPersistsModels(t *testing.T) {
	db, r, _ := setupAccountsTest(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/api/user/login":
			fmt.Fprint(w, `{"success":true,"data":{"access_token":"dashboard-pat"}}`)
		case req.Method == http.MethodGet && strings.HasPrefix(req.URL.Path, "/api/token/"):
			fmt.Fprint(w, `{"success":true,"data":{"items":[]}}`)
		case req.Method == http.MethodGet && req.URL.Path == "/v1/models":
			http.Error(w, `{"error":"dashboard PAT is not a relay key"}`, http.StatusUnauthorized)
		case req.Method == http.MethodGet && req.URL.Path == "/api/user/self":
			if req.Header.Get("Authorization") != "Bearer dashboard-pat" {
				http.Error(w, `{"message":"bad auth"}`, http.StatusUnauthorized)
				return
			}
			fmt.Fprint(w, `{"success":true,"data":{"id":1,"username":"root"}}`)
		case req.Method == http.MethodGet && req.URL.Path == "/api/user/models":
			if req.Header.Get("Authorization") != "Bearer dashboard-pat" {
				http.Error(w, `{"message":"bad auth"}`, http.StatusUnauthorized)
				return
			}
			fmt.Fprint(w, `{"success":true,"data":["gpt-4o-mini","claude-3-5-sonnet-20241022"]}`)
		default:
			http.NotFound(w, req)
		}
	}))
	t.Cleanup(upstream.Close)

	siteResp := doPostJSON(t, r, "/api/sites", map[string]any{
		"name":     "login model discovery",
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

	loginResp := doPostJSON(t, r, "/api/accounts/login", map[string]any{
		"siteId":   int64(site["id"].(float64)),
		"username": "root",
		"password": "metapi123",
	})
	if loginResp.Code != http.StatusOK {
		t.Fatalf("login: %d %s", loginResp.Code, loginResp.Body.String())
	}
	var login map[string]any
	if err := json.Unmarshal(loginResp.Body.Bytes(), &login); err != nil {
		t.Fatalf("decode login: %v", err)
	}
	refresh, _ := login["modelRefresh"].(map[string]any)
	if refresh == nil || refresh["success"] != true {
		t.Fatalf("modelRefresh = %#v, want successful immediate discovery", login["modelRefresh"])
	}
	account, _ := login["account"].(map[string]any)
	accountID := int64(account["id"].(float64))

	modelsResp := doGet(t, r, "/api/accounts/"+itoa(accountID)+"/models")
	if modelsResp.Code != http.StatusOK {
		t.Fatalf("models: %d %s", modelsResp.Code, modelsResp.Body.String())
	}
	var models map[string]any
	if err := json.Unmarshal(modelsResp.Body.Bytes(), &models); err != nil {
		t.Fatalf("decode models: %v", err)
	}
	if models["totalCount"] != float64(2) {
		t.Fatalf("totalCount = %v, want 2 immediately after login", models["totalCount"])
	}

	var persisted int
	if err := db.Get(&persisted, `SELECT COUNT(*) FROM model_availability WHERE account_id = ? AND available = TRUE`, accountID); err != nil {
		t.Fatalf("count persisted models: %v", err)
	}
	if persisted != 2 {
		t.Fatalf("persisted model rows = %d, want 2", persisted)
	}
}
