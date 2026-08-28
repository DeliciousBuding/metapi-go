package admin

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

// TestSearch_MixedCaseQueryMatchesMixedCaseRows guards the w18-pg-dialect
// LIKE divergence fix: SQLite LIKE is case-insensitive for ASCII while
// PostgreSQL LIKE is case-sensitive, so the shared search filters normalize
// both sides with LOWER(). On SQLite this test passes before and after the
// fix; the PostgreSQL half of the divergence is proven in
// store/like_case_test.go, and CI test-pg re-runs the store suite against a
// real server.
func TestSearch_MixedCaseQueryMatchesMixedCaseRows(t *testing.T) {
	globalChannelsCache.clear()
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	r := chi.NewRouter()
	RegisterSearchRoutes(r, db.DB)

	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.Exec(
		`INSERT INTO sites (name, url, platform, status, created_at, updated_at)
		 VALUES ('OpenAI SearchCaseProbe', 'https://searchcaseprobe.example.com', 'openai', 'active', ?, ?)`, now, now)
	if err != nil {
		t.Fatalf("insert site: %v", err)
	}
	siteID, _ := res.LastInsertId()

	res, err = db.Exec(
		`INSERT INTO accounts (site_id, username, access_token, status, checkin_enabled, created_at, updated_at)
		 VALUES (?, 'SearchCaseProbeUser', 'sk-searchcaseprobe', 'active', TRUE, ?, ?)`,
		siteID, now, now)
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}
	accountID, _ := res.LastInsertId()

	if _, err := db.Exec(
		`INSERT INTO account_tokens (account_id, name, token, value_status, source, enabled, is_default, created_at, updated_at)
		 VALUES (?, 'SearchCaseProbeToken', 'tok-searchcaseprobe', 'active', 'manual', TRUE, FALSE, ?, ?)`,
		accountID, now, now); err != nil {
		t.Fatalf("insert account token: %v", err)
	}

	// Lowercase query must match mixed-case site name, username and token
	// name on BOTH dialects (the pre-fix PG behavior returned zero hits).
	rec := doPostJSON(t, r, "/api/search", map[string]any{"query": "searchcaseprobe", "limit": 10})
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, key := range []string{"sites", "accounts", "accountTokens"} {
		items, _ := resp[key].([]any)
		if len(items) == 0 {
			t.Fatalf("search %q returned no %s hits; body=%s", "searchcaseprobe", key, rec.Body.String())
		}
	}
}
