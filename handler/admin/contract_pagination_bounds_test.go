package admin

// Wave 4 contract audit — pagination boundary behavior.
//
// Four admin list endpoints accept ?page/&pageSize= with identical clamp
// semantics (page → [1, 1_000_000], pageSize → [1, 200]); two more use the
// ?limit/&offset= style (parseLimitOffset: limit → [1, max], offset → >= 0).
// These tests pin the CURRENT clamp behavior for out-of-range inputs (0,
// negative, oversized) so any future semantic drift is deliberate, not
// accidental. The mixed 0-based-offset vs 1-based-page coexistence is the
// documented status quo and is not changed here.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

// pagedEnvelope is the shared {items,total,page,pageSize} list envelope.
type pagedEnvelope struct {
	Items    []map[string]any `json:"items"`
	Total    int              `json:"total"`
	Page     int              `json:"page"`
	PageSize int              `json:"pageSize"`
}

func decodePagedEnvelope(t *testing.T, resp *httptest.ResponseRecorder) pagedEnvelope {
	t.Helper()
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.Code, resp.Body.String())
	}
	var envelope pagedEnvelope
	if err := json.Unmarshal(resp.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode envelope: %v; body=%s", err, resp.Body.String())
	}
	return envelope
}

// ---- GET /api/routes (page-gated envelope) ----

func TestPaginationBounds_Routes_PageZeroAndNegativeClampToFirstPage(t *testing.T) {
	db, r := setupTokenRoutesTest(t)
	seedRoutes(t, db, 3, "2026-01-01T00:00:00Z")

	baseline := decodePagedEnvelope(t, doGet(t, r, "/api/routes?page=1&pageSize=2"))
	pageZero := decodePagedEnvelope(t, doGet(t, r, "/api/routes?page=0&pageSize=2"))
	pageNegative := decodePagedEnvelope(t, doGet(t, r, "/api/routes?page=-5&pageSize=2"))

	for label, envelope := range map[string]pagedEnvelope{"page=0": pageZero, "page=-5": pageNegative} {
		if envelope.Page != 1 {
			t.Fatalf("%s: response page = %d, want clamped 1", label, envelope.Page)
		}
		if len(envelope.Items) != len(baseline.Items) || envelope.Total != baseline.Total {
			t.Fatalf("%s: items=%d total=%d, want identical to page=1 (%d/%d)",
				label, len(envelope.Items), envelope.Total, len(baseline.Items), baseline.Total)
		}
	}
}

func TestPaginationBounds_Routes_PageSizeZeroNegativeAndOversized(t *testing.T) {
	db, r := setupTokenRoutesTest(t)
	seedRoutes(t, db, 3, "2026-01-01T00:00:00Z")

	sizeZero := decodePagedEnvelope(t, doGet(t, r, "/api/routes?page=1&pageSize=0"))
	if sizeZero.PageSize != 1 || len(sizeZero.Items) != 1 {
		t.Fatalf("pageSize=0: got pageSize=%d items=%d, want clamp to 1", sizeZero.PageSize, len(sizeZero.Items))
	}

	sizeNegative := decodePagedEnvelope(t, doGet(t, r, "/api/routes?page=1&pageSize=-10"))
	if sizeNegative.PageSize != 1 || len(sizeNegative.Items) != 1 {
		t.Fatalf("pageSize=-10: got pageSize=%d items=%d, want clamp to 1", sizeNegative.PageSize, len(sizeNegative.Items))
	}

	sizeOversized := decodePagedEnvelope(t, doGet(t, r, "/api/routes?page=1&pageSize=9999"))
	if sizeOversized.PageSize != 200 {
		t.Fatalf("pageSize=9999: got pageSize=%d, want clamp to 200", sizeOversized.PageSize)
	}
	if len(sizeOversized.Items) != 3 {
		t.Fatalf("pageSize=9999: items=%d, want all 3 rows", len(sizeOversized.Items))
	}
}

func TestPaginationBounds_Routes_PagePastLastPageKeepsTrueTotal(t *testing.T) {
	db, r := setupTokenRoutesTest(t)
	seedRoutes(t, db, 3, "2026-01-01T00:00:00Z")

	envelope := decodePagedEnvelope(t, doGet(t, r, "/api/routes?page=99&pageSize=2"))
	if len(envelope.Items) != 0 {
		t.Fatalf("page past end: items = %d, want 0", len(envelope.Items))
	}
	if envelope.Total != 3 {
		t.Fatalf("page past end: total = %d, want true total 3", envelope.Total)
	}
}

// ---- GET /api/accounts (always-paginated path) ----

func TestPaginationBounds_Accounts_OutOfRangeParamsClamped(t *testing.T) {
	_, r := setupAccountsPaginationTest(t)

	pageZero := decodePagedEnvelope(t, doGet(t, r, "/api/accounts?page=0&pageSize=-3"))
	if pageZero.Page != 1 {
		t.Fatalf("page=0: response page = %d, want clamped 1", pageZero.Page)
	}
	if pageZero.PageSize != 1 {
		t.Fatalf("pageSize=-3: response pageSize = %d, want clamped 1", pageZero.PageSize)
	}
	if len(pageZero.Items) != 1 {
		t.Fatalf("clamped page: items = %d, want 1", len(pageZero.Items))
	}
	if pageZero.Total != 3 {
		t.Fatalf("clamped page: total = %d, want 3", pageZero.Total)
	}

	sizeOversized := decodePagedEnvelope(t, doGet(t, r, "/api/accounts?page=1&pageSize=5000"))
	if sizeOversized.PageSize != 200 {
		t.Fatalf("pageSize=5000: response pageSize = %d, want clamp to 200", sizeOversized.PageSize)
	}
	if len(sizeOversized.Items) != 3 {
		t.Fatalf("pageSize=5000: items = %d, want all 3 accounts", len(sizeOversized.Items))
	}
}

// ---- GET /api/downstream-keys (page-gated envelope) ----

func TestPaginationBounds_DownstreamKeys_OutOfRangeParamsClamped(t *testing.T) {
	db, r := setupDownstreamKeysTest(t)
	for _, name := range []string{"bound-key-a", "bound-key-b", "bound-key-c"} {
		seedDownstreamKeyFixture(t, db, name, true)
	}

	pageNegative := decodePagedEnvelope(t, doGet(t, r, "/api/downstream-keys?page=-1&pageSize=0"))
	if pageNegative.Page != 1 || pageNegative.PageSize != 1 {
		t.Fatalf("page=-1&pageSize=0: got page=%d pageSize=%d, want clamped 1/1",
			pageNegative.Page, pageNegative.PageSize)
	}
	if len(pageNegative.Items) != 1 || pageNegative.Total != 3 {
		t.Fatalf("clamped page: items=%d total=%d, want 1/3", len(pageNegative.Items), pageNegative.Total)
	}

	sizeOversized := decodePagedEnvelope(t, doGet(t, r, "/api/downstream-keys?page=1&pageSize=100000"))
	if sizeOversized.PageSize != 200 {
		t.Fatalf("pageSize=100000: got pageSize=%d, want clamp to 200", sizeOversized.PageSize)
	}
}

// ---- GET /api/channels (explicit paging mode) ----

func TestPaginationBounds_Channels_ExplicitPagingClampsAndCounts(t *testing.T) {
	db, r := setupTokenRoutesTest(t)
	routeID, accountID, tokenID := seedRouteChannelRefs(t, db)
	for i := 0; i < 5; i++ {
		if _, err := db.Exec(
			`INSERT INTO route_channels
				(route_id, account_id, token_id, source_model, priority, weight, enabled, manual_override)
			 VALUES (?, ?, ?, ?, 0, 10, TRUE, FALSE)`,
			routeID, accountID, tokenID, "gpt-bound-"+itoa(int64(i)),
		); err != nil {
			t.Fatalf("insert channel %d: %v", i, err)
		}
	}

	pageZero := decodePagedEnvelope(t, doGet(t, r, "/api/channels?page=0&pageSize=0"))
	if pageZero.Page != 1 || pageZero.PageSize != 1 {
		t.Fatalf("page=0&pageSize=0: got page=%d pageSize=%d, want clamped 1/1",
			pageZero.Page, pageZero.PageSize)
	}
	if len(pageZero.Items) != 1 {
		t.Fatalf("clamped channels page: items = %d, want 1", len(pageZero.Items))
	}
	// Explicit paging reports the true fleet total via COUNT(*) OVER () or
	// the empty-page COUNT(*) fallback.
	if pageZero.Total != 5 {
		t.Fatalf("clamped channels page: total = %d, want 5", pageZero.Total)
	}

	sizeOversized := decodePagedEnvelope(t, doGet(t, r, "/api/channels?page=1&pageSize=999999"))
	if sizeOversized.PageSize != 200 {
		t.Fatalf("pageSize=999999: got pageSize=%d, want clamp to 200", sizeOversized.PageSize)
	}

	pastEnd := decodePagedEnvelope(t, doGet(t, r, "/api/channels?page=50&pageSize=50"))
	if len(pastEnd.Items) != 0 {
		t.Fatalf("page past end: items = %d, want 0", len(pastEnd.Items))
	}
	if pastEnd.Total != 5 {
		t.Fatalf("page past end: total = %d, want true total 5 (COUNT fallback)", pastEnd.Total)
	}
}

// ---- GET /api/checkin/logs (limit/offset style) ----

func TestPaginationBounds_CheckinLogs_LimitOffsetClamped(t *testing.T) {
	db, r, _ := setupCheckinRoutesTest(t)
	upstream, _ := newCheckinAnyRouterServer(t)
	siteID := insertCheckinRouteSite(t, db, upstream.URL)
	accountID := insertCheckinRouteAccount(t, db, siteID, true)
	seedCheckinLogs(t, db, accountID, []struct {
		status  string
		message string
	}{
		{"success", "reward a"},
		{"failed", "boom"},
		{"skipped", "no-op"},
		{"success", "reward b"},
	})

	var envelope struct {
		Items    []map[string]any `json:"items"`
		Total    int              `json:"total"`
		Page     int              `json:"page"`
		PageSize int              `json:"pageSize"`
	}

	resp := doGet(t, r, "/api/checkin/logs?limit=0&offset=-10")
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.Code, resp.Body.String())
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode: %v; body=%s", err, resp.Body.String())
	}
	if envelope.PageSize != 1 {
		t.Fatalf("limit=0: pageSize = %d, want clamp to 1", envelope.PageSize)
	}
	if len(envelope.Items) != 1 {
		t.Fatalf("limit=0: items = %d, want 1", len(envelope.Items))
	}
	if envelope.Total != 4 {
		t.Fatalf("limit=0: total = %d, want 4", envelope.Total)
	}

	resp = doGet(t, r, "/api/checkin/logs?limit=99999")
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.Code, resp.Body.String())
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode: %v; body=%s", err, resp.Body.String())
	}
	if envelope.PageSize != 200 {
		t.Fatalf("limit=99999: pageSize = %d, want clamp to 200", envelope.PageSize)
	}
	if len(envelope.Items) != 4 {
		t.Fatalf("limit=99999: items = %d, want all 4 logs", len(envelope.Items))
	}
}

// ---- GET /api/admin/audit-logs (limit/offset style) ----

func TestPaginationBounds_AuditLogs_LimitOffsetClamped(t *testing.T) {
	db, _ := setupStatsSQLiteTest(t)
	now := "2026-01-01T00:00:00Z"
	for _, method := range []string{"POST", "PUT", "DELETE"} {
		if _, err := db.Exec(`INSERT INTO admin_audit_logs (actor, method, path, status, request_id, remote_ip, created_at)
			VALUES ('aabbccdd', ?, '/api/bounds', 200, 'req-x', '1.2.3.4', ?)`, method, now); err != nil {
			t.Fatalf("seed audit row: %v", err)
		}
	}
	r := chi.NewRouter()
	RegisterAuditLogsRoutes(r, db.DB)

	var envelope struct {
		Items  []map[string]any `json:"items"`
		Total  int              `json:"total"`
		Limit  int              `json:"limit"`
		Offset int              `json:"offset"`
	}

	resp := doGet(t, r, "/api/admin/audit-logs?limit=0&offset=-7")
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.Code, resp.Body.String())
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode: %v; body=%s", err, resp.Body.String())
	}
	if envelope.Limit != 1 {
		t.Fatalf("limit=0: limit = %d, want clamp to 1", envelope.Limit)
	}
	if envelope.Offset != 0 {
		t.Fatalf("offset=-7: offset = %d, want clamp to 0", envelope.Offset)
	}
	if len(envelope.Items) != 1 || envelope.Total != 3 {
		t.Fatalf("clamped audit page: items=%d total=%d, want 1/3", len(envelope.Items), envelope.Total)
	}

	resp = doGet(t, r, "/api/admin/audit-logs?limit=50000")
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.Code, resp.Body.String())
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode: %v; body=%s", err, resp.Body.String())
	}
	if envelope.Limit != 200 {
		t.Fatalf("limit=50000: limit = %d, want clamp to 200", envelope.Limit)
	}
}
