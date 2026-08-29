package admin

import (
	"strconv"
	"testing"
	"time"
)

// TestListAccounts_PaginationPostgres guards the page-gated accounts endpoint
// against PostgreSQL dialect regressions. The seed uses execInsertID instead
// of LastInsertId (pgx does not support it) and boolean literals accepted by
// both SQLite and PG, matching the existing accounts PG fixtures.
func TestListAccounts_PaginationPostgres(t *testing.T) {
	db, r, _ := setupAccountsPostgresTest(t)
	globalAccountsCache.clear()
	now := time.Now().UTC().Format(time.RFC3339)
	prefix := "pg-acct-page-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	t.Cleanup(func() {
		_, _ = db.Exec(db.Rebind(`DELETE FROM accounts WHERE username = ?`), prefix+"-0")
		_, _ = db.Exec(db.Rebind(`DELETE FROM accounts WHERE username = ?`), prefix+"-1")
		_, _ = db.Exec(db.Rebind(`DELETE FROM accounts WHERE username = ?`), prefix+"-2")
		_, _ = db.Exec(db.Rebind(`DELETE FROM sites WHERE name = ?`), prefix+"-site")
	})

	siteID, err := execInsertID(db.DB,
		`INSERT INTO sites (name, url, platform, status, created_at, updated_at)
		 VALUES (?, 'https://acct-pg.example.test', 'openai', 'active', ?, ?)`,
		prefix+"-site", now, now)
	if err != nil {
		t.Fatalf("insert pg site: %v", err)
	}
	for i := 0; i < 3; i++ {
		if _, err := db.Exec(db.Rebind(
			`INSERT INTO accounts (site_id, username, access_token, status, checkin_enabled, created_at, updated_at)
			 VALUES (?, ?, ?, 'active', true, ?, ?)`),
			siteID, prefix+"-"+strconv.Itoa(i), "token-"+strconv.Itoa(i), now, now); err != nil {
			t.Fatalf("insert pg account: %v", err)
		}
	}

	pageOne := decodePagedEnvelope(t, doGet(t, r, "/api/accounts?page=1&pageSize=2"))
	if pageOne.Total < 3 || len(pageOne.Items) != 2 || pageOne.Page != 1 {
		t.Fatalf("pg accounts page 1 = total %d items %d page %d, want >=3/2/1",
			pageOne.Total, len(pageOne.Items), pageOne.Page)
	}
	pageTwo := decodePagedEnvelope(t, doGet(t, r, "/api/accounts?page=2&pageSize=2"))
	if pageTwo.Total < 3 || len(pageTwo.Items) < 1 || pageTwo.Page != 2 {
		t.Fatalf("pg accounts page 2 = total %d items %d page %d, want >=3/>=1/2",
			pageTwo.Total, len(pageTwo.Items), pageTwo.Page)
	}
}

// TestListChannels_PaginationPostgres guards the page-gated channels endpoint
// on PostgreSQL. It seeds one route channel and confirms the envelope still
// reports the real total and a page-sized subset.
func TestListChannels_PaginationPostgres(t *testing.T) {
	db, r := setupTokenRoutesPostgresTest(t)
	globalChannelsCache.clear()
	routeID, accountID, tokenID := seedRouteChannelRefs(t, db)
	prefix := "pg-channel-page-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	channelID, err := execInsertID(db.DB,
		`INSERT INTO route_channels (route_id, account_id, token_id, source_model, priority, weight, enabled, manual_override)
		 VALUES (?, ?, ?, ?, 1, 10, true, false)`,
		routeID, accountID, tokenID, prefix)
	if err != nil {
		t.Fatalf("insert pg route channel: %v", err)
	}
	var siteID int64
	_ = db.Get(&siteID, db.Rebind(`SELECT site_id FROM accounts WHERE id = ?`), accountID)
	t.Cleanup(func() {
		_, _ = db.Exec(db.Rebind(`DELETE FROM route_channels WHERE id = ? OR route_id = ? OR account_id = ? OR token_id = ?`), channelID, routeID, accountID, tokenID)
		_, _ = db.Exec(db.Rebind(`DELETE FROM account_tokens WHERE account_id = ?`), accountID)
		_, _ = db.Exec(db.Rebind(`DELETE FROM token_routes WHERE id = ?`), routeID)
		_, _ = db.Exec(db.Rebind(`DELETE FROM accounts WHERE id = ?`), accountID)
		_, _ = db.Exec(db.Rebind(`DELETE FROM sites WHERE id = ?`), siteID)
	})

	pageOne := decodePagedEnvelope(t, doGet(t, r, "/api/channels?page=1&pageSize=1"))
	if pageOne.Total < 1 || len(pageOne.Items) != 1 {
		t.Fatalf("pg channels page = total %d items %d, want >=1/1",
			pageOne.Total, len(pageOne.Items))
	}
}
