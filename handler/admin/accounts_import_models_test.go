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

func TestAccounts_SessionImportPersistsModelsAfterRelayTokenSync(t *testing.T) {
	db, r, _ := setupAccountsTest(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch req.URL.Path {
		case "/api/user/self":
			fmt.Fprint(w, `{"success":true,"data":{"id":2,"username":"import-user","quota":10000000}}`)
		case "/api/token/":
			fmt.Fprint(w, `{"success":true,"data":[{"id":1,"name":"relay","key":"fixture-relay-key","status":1}]}`)
		case "/v1/models":
			if req.Header.Get("Authorization") != "Bearer fixture-relay-key" {
				w.WriteHeader(http.StatusUnauthorized)
				fmt.Fprint(w, `{"error":"management credential is not a relay key"}`)
				return
			}
			fmt.Fprint(w, `{"data":[{"id":"first-import-model"}]}`)
		default:
			http.NotFound(w, req)
		}
	}))
	defer upstream.Close()
	siteID := createTokenSyncSite(t, r, "fresh import", upstream.URL)
	resp := doPostJSON(t, r, "/api/accounts", map[string]any{"siteId": siteID, "accessToken": "fixture-management-pat", "credentialMode": "session", "username": "import-user"})
	if resp.Code != http.StatusOK {
		t.Fatalf("create HTTP %d: %s", resp.Code, resp.Body.String())
	}
	var result map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["modelCount"] != float64(1) || result["apiTokenFound"] != true {
		t.Fatalf("initialized state=%s", resp.Body.String())
	}
	refresh, _ := result["modelRefresh"].(map[string]any)
	if refresh["success"] != true {
		t.Fatalf("refresh=%#v", refresh)
	}
	aid := int64(result["id"].(float64))
	var count int
	if err := db.Get(&count, db.Rebind("SELECT COUNT(*) FROM model_availability WHERE account_id = ? AND available = TRUE"), aid); err != nil || count != 1 {
		t.Fatalf("persisted=%d, err=%v", count, err)
	}
	models := doGet(t, r, "/api/accounts/"+itoa(aid)+"/models")
	if !strings.Contains(models.Body.String(), "first-import-model") {
		t.Fatalf("immediate models=%s", models.Body.String())
	}
}

func TestAccounts_BatchAPIKeyImportsPersistEachModelInventory(t *testing.T) {
	db, r, _ := setupAccountsTest(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":[{"id":"batch-import-model"}]}`)
	}))
	defer upstream.Close()
	site := doPostJSON(t, r, "/api/sites", map[string]any{"name": "batch model imports", "url": upstream.URL, "platform": "openai"})
	var siteData map[string]any
	json.Unmarshal(site.Body.Bytes(), &siteData)
	resp := doPostJSON(t, r, "/api/accounts", map[string]any{"siteId": siteData["id"], "credentialMode": "apikey", "accessTokens": []string{"fixture-api-a", "fixture-api-b"}})
	if resp.Code != http.StatusOK {
		t.Fatalf("create: %s", resp.Body.String())
	}
	var result map[string]any
	json.Unmarshal(resp.Body.Bytes(), &result)
	items := result["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("items=%#v", items)
	}
	for _, raw := range items {
		item := raw.(map[string]any)
		if item["modelCount"] != float64(1) {
			t.Fatalf("item=%#v", item)
		}
	}
	var count int
	if err := db.Get(&count, "SELECT COUNT(*) FROM model_availability WHERE available = TRUE"); err != nil || count != 2 {
		t.Fatalf("persisted=%d, err=%v", count, err)
	}
}

func TestAccounts_ExplicitSkipImportDoesNotProbeModels(t *testing.T) {
	_, r, _ := setupAccountsTest(t)
	var hits atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) { hits.Add(1); http.Error(w, "must not probe", 500) }))
	defer upstream.Close()
	site := doPostJSON(t, r, "/api/sites", map[string]any{"name": "explicit no probe", "url": upstream.URL, "platform": "openai"})
	var siteData map[string]any
	json.Unmarshal(site.Body.Bytes(), &siteData)
	resp := doPostJSON(t, r, "/api/accounts", map[string]any{"siteId": siteData["id"], "credentialMode": "apikey", "accessToken": "fixture-api-key", "skipModelFetch": true})
	if resp.Code != http.StatusOK {
		t.Fatalf("create: %s", resp.Body.String())
	}
	var result map[string]any
	json.Unmarshal(resp.Body.Bytes(), &result)
	refresh, _ := result["modelRefresh"].(map[string]any)
	if refresh["skipped"] != true || hits.Load() != 0 {
		t.Fatalf("refresh=%#v, probes=%d", refresh, hits.Load())
	}
}
