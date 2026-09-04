package balance

import (
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
)

// TestRefreshBalance_UpstreamFailureIsNotStoredAsZeroBalance covers the two facts
// an operator depends on when a balance refresh cannot reach the upstream:
//
//  1. the stored balance is left alone. A failed fetch used to come back as a zero
//     BalanceInfo with a nil error, which was then written to accounts.balance /
//     balance_used / quota — so one unreachable upstream silently reported the
//     account as having no funds left.
//  2. the failure is visible. Every recovery path in this package (runtime health,
//     the token-expired alert, the auto-relogin retry) is driven by err != nil, so
//     a swallowed fetch error skipped all three and left the account marked healthy.
//
// veloera and done-hub are the two adapters that returned the zero value; both are
// registered platforms reachable from platform.GetAdapter.
func TestRefreshBalance_UpstreamFailureIsNotStoredAsZeroBalance(t *testing.T) {
	for _, platformName := range []string{"veloera", "done-hub"} {
		t.Run(platformName, func(t *testing.T) {
			db, err := store.Open(store.DialectSQLite, ":memory:", false)
			if err != nil {
				t.Fatalf("open sqlite: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })
			if err := store.AutoMigrate(db); err != nil {
				t.Fatalf("migrate: %v", err)
			}

			now := time.Now().UTC().Format(time.RFC3339)
			siteRes, err := db.Exec(
				"INSERT INTO sites (name, url, platform, status, created_at, updated_at) VALUES (?, ?, ?, 'active', ?, ?)",
				platformName+" unreachable", "http://127.0.0.1:1", platformName, now, now,
			)
			if err != nil {
				t.Fatalf("insert site: %v", err)
			}
			siteID, err := siteRes.LastInsertId()
			if err != nil {
				t.Fatalf("site LastInsertId: %v", err)
			}

			// A real stored balance, so "left alone" is distinguishable from "zeroed".
			accountRes, err := db.Exec(
				`INSERT INTO accounts (site_id, username, access_token, status, balance, balance_used, quota, checkin_enabled, created_at, updated_at)
				 VALUES (?, ?, ?, 'active', 5, 1, 10, ?, ?, ?)`,
				siteID, "balance-owner", "session-token", false, now, now,
			)
			if err != nil {
				t.Fatalf("insert account: %v", err)
			}
			accountID, err := accountRes.LastInsertId()
			if err != nil {
				t.Fatalf("account LastInsertId: %v", err)
			}

			if _, err := RefreshBalance(&config.Config{}, db.DB, accountID); err == nil {
				t.Fatal("RefreshBalance reported success against an unreachable upstream")
			}

			var balance, used, quota float64
			if err := db.QueryRow(
				"SELECT balance, balance_used, quota FROM accounts WHERE id = ?", accountID,
			).Scan(&balance, &used, &quota); err != nil {
				t.Fatalf("read stored balance: %v", err)
			}
			if balance != 5 || used != 1 || quota != 10 {
				t.Errorf("stored balance became (%g, %g, %g), want (5, 1, 10) — a failed fetch must not write a zero balance",
					balance, used, quota)
			}

			var extra *string
			if err := db.QueryRow("SELECT extra_config FROM accounts WHERE id = ?", accountID).Scan(&extra); err != nil {
				t.Fatalf("read extra_config: %v", err)
			}
			if extra == nil || !strings.Contains(*extra, "runtimeHealth") {
				t.Errorf("no runtime health was recorded for a failed refresh (extra_config=%v)", extra)
			}
		})
	}
}
