package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

func TestRefreshAccountPricingManagementContextAndFailure(t *testing.T) {
	db := openModelRefreshTestDB(t)
	var fail atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pricing" || r.Header.Get("New-API-User") != "42" || r.Header.Get("Authorization") != "Bearer synthetic-management" {
			t.Errorf("missing account context: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		if fail.Load() {
			w.WriteHeader(500)
			_, _ = w.Write([]byte(`{"error":"synthetic-management"}`))
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"data":[{"model_name":"synthetic","quota_type":0,"model_ratio":1,"completion_ratio":2,"enable_groups":["all"]}],"group_ratio":{"default":0.5},"usable_group":{"default":"Standard"}}`))
	}))
	defer server.Close()
	siteID := seedSiteOnPlatform(t, db, "pricing-synthetic", server.URL, "new-api")
	accountID, _ := seedNewAPIAccountCredentials(t, db, siteID, "synthetic-relay", "synthetic-management")
	if err := RefreshAccountPricing(context.Background(), db, accountID); err != nil {
		t.Fatal(err)
	}
	var initial store.AccountPricingSnapshot
	if err := db.Get(&initial, db.Rebind(`SELECT * FROM account_pricing WHERE account_id=?`), accountID); err != nil {
		t.Fatal(err)
	}
	fail.Store(true)
	if err := RefreshAccountPricing(context.Background(), db, accountID); err == nil || strings.Contains(err.Error(), "synthetic-management") {
		t.Fatalf("error=%v", err)
	}
	var got store.AccountPricingSnapshot
	if err := db.Get(&got, db.Rebind(`SELECT * FROM account_pricing WHERE account_id=?`), accountID); err != nil {
		t.Fatal(err)
	}
	if got.Payload != initial.Payload || got.FetchedAt != initial.FetchedAt || got.LastError != "upstream_error" || strings.Contains(got.Payload, "synthetic-management") {
		t.Fatalf("saved snapshot: %+v", got)
	}
}
