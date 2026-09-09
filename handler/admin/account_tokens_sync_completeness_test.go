package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/service"
)

func TestAccountTokenSyncIncompleteNewAPIListingDoesNotPersistOrChangeDefault(t *testing.T) {
	for _, tc := range []struct {
		name       string
		secondPage http.HandlerFunc
	}{
		{
			name: "second_page_failure",
			secondPage: func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, `{"success":false,"message":"temporary failure"}`, http.StatusInternalServerError)
			},
		},
		{
			name: "short_page_before_total",
			secondPage: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"success": true,
					"data": map[string]interface{}{
						"items":     []interface{}{},
						"total":     101,
						"page_size": 100,
					},
				})
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/api/token/" {
					http.NotFound(w, r)
					return
				}
				if r.URL.Query().Get("p") == "2" {
					tc.secondPage(w, r)
					return
				}
				items := make([]map[string]interface{}, 0, 100)
				for i := 0; i < 100; i++ {
					items = append(items, map[string]interface{}{
						"id":     i + 1,
						"name":   fmt.Sprintf("filler-%d", i),
						"key":    fmt.Sprintf("sk-filler-%d", i),
						"status": 1,
					})
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"success": true,
					"data": map[string]interface{}{
						"items":     items,
						"total":     101,
						"page_size": 100,
					},
				})
			}))
			t.Cleanup(upstream.Close)

			db, _, cfg := setupAccountsTest(t)
			now := time.Now().UTC().Format(time.RFC3339)
			siteRes, err := db.Exec(
				`INSERT INTO sites (name, url, platform, status, created_at, updated_at) VALUES (?, ?, 'new-api', 'active', ?, ?)`,
				"Incomplete NewAPI listing", upstream.URL, now, now,
			)
			if err != nil {
				t.Fatalf("insert site: %v", err)
			}
			siteID, err := siteRes.LastInsertId()
			if err != nil {
				t.Fatalf("site LastInsertId: %v", err)
			}
			accountRes, err := db.Exec(
				`INSERT INTO accounts (site_id, username, access_token, api_token, status, checkin_enabled, created_at, updated_at)
				 VALUES (?, 'sync-user', 'dashboard-pat', 'sk-revoked-key', 'active', TRUE, ?, ?)`,
				siteID, now, now,
			)
			if err != nil {
				t.Fatalf("insert account: %v", err)
			}
			accountID, err := accountRes.LastInsertId()
			if err != nil {
				t.Fatalf("account LastInsertId: %v", err)
			}
			if _, err := db.Exec(
				`INSERT INTO account_tokens (account_id, name, token, token_group, value_status, source, enabled, is_default, created_at, updated_at)
				 VALUES (?, 'revoked-default', 'sk-revoked-key', 'default', 'ready', 'manual', TRUE, TRUE, ?, ?)`,
				accountID, now, now,
			); err != nil {
				t.Fatalf("insert default token: %v", err)
			}

			row, err := service.GetAccountWithSiteByID(db.DB, accountID)
			if err != nil {
				t.Fatalf("GetAccountWithSiteByID: %v", err)
			}
			_, syncErr := executeAccountTokenSync(context.Background(), db.DB, cfg, row)
			if syncErr == nil {
				t.Fatal("executeAccountTokenSync succeeded on an incomplete New API listing")
			}
			if !strings.Contains(syncErr.Error(), "page 2") {
				t.Fatalf("sync error = %v, want page 2 failure", syncErr)
			}

			var tokenCount int
			if err := db.QueryRow("SELECT COUNT(*) FROM account_tokens WHERE account_id = ?", accountID).Scan(&tokenCount); err != nil {
				t.Fatalf("count account tokens: %v", err)
			}
			if tokenCount != 1 {
				t.Fatalf("account_tokens = %d, want 1 (no partial listing persisted)", tokenCount)
			}
			var defaultName string
			if err := db.QueryRow("SELECT name FROM account_tokens WHERE account_id = ? AND is_default = TRUE", accountID).Scan(&defaultName); err != nil {
				t.Fatalf("read default token: %v", err)
			}
			if defaultName != "revoked-default" {
				t.Fatalf("default token = %q, want revoked-default", defaultName)
			}
			var apiToken *string
			if err := db.QueryRow("SELECT api_token FROM accounts WHERE id = ?", accountID).Scan(&apiToken); err != nil {
				t.Fatalf("read accounts.api_token: %v", err)
			}
			if apiToken == nil || *apiToken != "sk-revoked-key" {
				t.Fatalf("accounts.api_token = %v, want sk-revoked-key", apiToken)
			}
		})
	}
}
