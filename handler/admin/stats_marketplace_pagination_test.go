package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

// seedMarketplacePaginationRows creates one site/account and three
// model_availability rows whose model names share the supplied prefix.
// It is used by the SQLite and PostgreSQL marketplace pagination tests.
func seedMarketplacePaginationRows(t *testing.T, db *store.DB, prefix string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	cleanup := func() {
		for i := 0; i < 3; i++ {
			_, _ = db.Exec(db.Rebind(`DELETE FROM model_availability WHERE model_name = ?`),
				prefix+"-"+strconv.Itoa(i))
		}
		_, _ = db.Exec(db.Rebind(`DELETE FROM accounts WHERE username = ?`), prefix+"-account")
		_, _ = db.Exec(db.Rebind(`DELETE FROM sites WHERE name = ?`), prefix+"-site")
	}
	cleanup()
	t.Cleanup(cleanup)

	siteID, err := execInsertID(db.DB,
		`INSERT INTO sites (name, url, platform, status, created_at, updated_at)
		 VALUES (?, 'https://marketplace.example.test', 'openai', 'active', ?, ?)`,
		prefix+"-site", now, now)
	if err != nil {
		t.Fatalf("insert marketplace site: %v", err)
	}
	accountID, err := execInsertID(db.DB,
		`INSERT INTO accounts (site_id, username, access_token, status, checkin_enabled, created_at, updated_at)
		 VALUES (?, ?, 'token', 'active', true, ?, ?)`,
		siteID, prefix+"-account", now, now)
	if err != nil {
		t.Fatalf("insert marketplace account: %v", err)
	}
	for i := 0; i < 3; i++ {
		if _, err := db.Exec(db.Rebind(
			`INSERT INTO model_availability (account_id, model_name, available, is_manual, latency_ms, checked_at)
			 VALUES (?, ?, true, false, ?, ?)`),
			accountID, prefix+"-"+strconv.Itoa(i), 10+i, now); err != nil {
			t.Fatalf("insert model availability %d: %v", i, err)
		}
	}
}

type marketplacePagedEnvelope struct {
	Items    []map[string]any `json:"items"`
	Total    int              `json:"total"`
	Page     int              `json:"page"`
	PageSize int              `json:"pageSize"`
	Meta     json.RawMessage  `json:"meta"`
}

func decodeMarketplaceEnvelope(t *testing.T, resp *httptest.ResponseRecorder) marketplacePagedEnvelope {
	t.Helper()
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.Code, resp.Body.String())
	}
	var envelope marketplacePagedEnvelope
	if err := json.Unmarshal(resp.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode paginated envelope: %v; body=%s", err, resp.Body.String())
	}
	return envelope
}

// TestModelsMarketplace_NoPaginationReturnsLegacyShape pins backward
// compatibility: without ?page the endpoint still returns the old
// {models, meta} surface, so pre-existing callers do not see a new envelope.
func TestModelsMarketplace_NoPaginationReturnsLegacyShape(t *testing.T) {
	db, r := setupStatsSQLiteTest(t)
	seedMarketplacePaginationRows(t, db, "legacy-marketplace")

	resp := doGet(t, r, "/api/models/marketplace")
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode legacy body: %v; body=%s", err, resp.Body.String())
	}
	models, ok := body["models"].([]any)
	if !ok || len(models) != 3 {
		t.Fatalf("legacy models = %#v, want array of 3", body["models"])
	}
	if _, hasItems := body["items"]; hasItems {
		t.Fatalf("legacy response must not include items: %#v", body["items"])
	}
	if _, hasTotal := body["total"]; hasTotal {
		t.Fatalf("legacy response must not include total: %#v", body["total"])
	}
}

// TestModelsMarketplace_PaginationReturnsSubsetAndTotal verifies that
// ?page=&pageSize= slices the aggregated directory and returns the shared
// {items,total,page,pageSize,meta} envelope with the real total.
func TestModelsMarketplace_PaginationReturnsSubsetAndTotal(t *testing.T) {
	db, r := setupStatsSQLiteTest(t)
	seedMarketplacePaginationRows(t, db, "paged-marketplace")

	pageOne := decodeMarketplaceEnvelope(t, doGet(t, r, "/api/models/marketplace?page=1&pageSize=2"))
	if pageOne.Total != 3 {
		t.Fatalf("page 1 total = %d, want 3", pageOne.Total)
	}
	if len(pageOne.Items) != 2 {
		t.Fatalf("page 1 items = %d, want 2", len(pageOne.Items))
	}
	if pageOne.Page != 1 || pageOne.PageSize != 2 {
		t.Fatalf("page 1 = %d/%d, want 1/2", pageOne.Page, pageOne.PageSize)
	}
	if len(pageOne.Meta) == 0 {
		t.Fatalf("page 1 meta is empty; the refresh/catalog metadata must survive paging")
	}

	pageTwo := decodeMarketplaceEnvelope(t, doGet(t, r, "/api/models/marketplace?page=2&pageSize=2"))
	if pageTwo.Total != 3 {
		t.Fatalf("page 2 total = %d, want 3", pageTwo.Total)
	}
	if len(pageTwo.Items) != 1 {
		t.Fatalf("page 2 items = %d, want 1 remainder", len(pageTwo.Items))
	}
	if pageTwo.Page != 2 {
		t.Fatalf("page 2 page = %d, want 2", pageTwo.Page)
	}

	pagePastEnd := decodeMarketplaceEnvelope(t, doGet(t, r, "/api/models/marketplace?page=99&pageSize=2"))
	if pagePastEnd.Total != 3 {
		t.Fatalf("page past end total = %d, want 3", pagePastEnd.Total)
	}
	if len(pagePastEnd.Items) != 0 {
		t.Fatalf("page past end items = %d, want 0", len(pagePastEnd.Items))
	}
}

// TestModelsMarketplace_PaginationPostgres exercises the same page-gated
// response against PostgreSQL when PG_TEST_DSN is available. The aggregator
// queries use the dual-dialect sqlx Rebind path, so this catches dialect
// regressions in the paginated slice without changing SQL semantics.
func TestModelsMarketplace_PaginationPostgres(t *testing.T) {
	db, r := setupStatsPostgresTest(t)
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	seedMarketplacePaginationRows(t, db, "pg-marketplace-"+suffix)

	pageOne := decodeMarketplaceEnvelope(t, doGet(t, r, "/api/models/marketplace?page=1&pageSize=2"))
	if pageOne.Total != 3 || len(pageOne.Items) != 2 {
		t.Fatalf("postgres page 1 = total %d items %d, want 3/2", pageOne.Total, len(pageOne.Items))
	}
	pageTwo := decodeMarketplaceEnvelope(t, doGet(t, r, "/api/models/marketplace?page=2&pageSize=2"))
	if pageTwo.Total != 3 || len(pageTwo.Items) != 1 {
		t.Fatalf("postgres page 2 = total %d items %d, want 3/1", pageTwo.Total, len(pageTwo.Items))
	}
}
