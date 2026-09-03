package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/deliciousbuding/metapi-go/store"
)

// openModelRefreshTestDB opens an in-memory SQLite DB with the full schema
// migrated, ready for model_availability / token_model_availability tests.
func openModelRefreshTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db.DB
}

// seedAccountAndToken inserts a site, account, and an account_tokens row
// whose token matches the account access_token, returning the IDs.
func seedAccountAndToken(t *testing.T, db *sqlx.DB, tokenValue string) (accountID, tokenID int64) {
	t.Helper()
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	res, err := db.Exec(
		`INSERT INTO sites (name, url, platform, status, use_system_proxy, sort_order, global_weight, post_refresh_probe_enabled, created_at, updated_at)
		 VALUES (?, ?, 'openai', 'active', FALSE, 0, 0, FALSE, ?, ?)`,
		"ModelRefreshSite-"+tokenValue, "https://api.example.com", now, now,
	)
	if err != nil {
		t.Fatalf("insert site: %v", err)
	}
	siteID, _ := res.LastInsertId()
	return seedAccountOnSite(t, db, siteID, tokenValue, tokenValue, "active")
}

// seedAccountOnSite inserts an account (and a matching account_tokens row when
// tokenValue is non-empty) on an existing site. status may be any accounts
// status value, including "" to exercise the empty-status candidate path.
func seedAccountOnSite(t *testing.T, db *sqlx.DB, siteID int64, username, tokenValue, status string) (accountID, tokenID int64) {
	t.Helper()
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	res, err := db.Exec(
		`INSERT INTO accounts (site_id, username, access_token, status, checkin_enabled, sort_order, created_at, updated_at)
		 VALUES (?, ?, ?, ?, TRUE, 0, ?, ?)`,
		siteID, username, tokenValue, status, now, now,
	)
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}
	accountID, _ = res.LastInsertId()
	if tokenValue == "" {
		return accountID, 0
	}
	tokRes, err := db.Exec(
		`INSERT INTO account_tokens (account_id, name, token, value_status, source, enabled, is_default, created_at, updated_at)
		 VALUES (?, 'default', ?, 'ready', 'manual', TRUE, TRUE, ?, ?)`,
		accountID, tokenValue, now, now,
	)
	if err != nil {
		t.Fatalf("insert account_token: %v", err)
	}
	tokenID, _ = tokRes.LastInsertId()
	return accountID, tokenID
}

// seedOpenAISite inserts an openai-platform site pointing at url.
func seedOpenAISite(t *testing.T, db *sqlx.DB, name, url string) int64 {
	t.Helper()
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	res, err := db.Exec(
		`INSERT INTO sites (name, url, platform, status, use_system_proxy, sort_order, global_weight, post_refresh_probe_enabled, created_at, updated_at)
		 VALUES (?, ?, 'openai', 'active', FALSE, 0, 0, FALSE, ?, ?)`,
		name, url, now, now,
	)
	if err != nil {
		t.Fatalf("insert site: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// startModelsUpstream serves the OpenAI-compatible /v1/models contract the
// openai adapter fetches during refresh.
func startModelsUpstream(t *testing.T, status int, models ...string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		data := make([]map[string]string, 0, len(models))
		for _, m := range models {
			data = append(data, map[string]string{"id": m})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"object": "list", "data": data})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// withModelRefreshCounters swaps the rebuild/invalidate seams for counting
// fakes and restores the production owners on cleanup.
func withModelRefreshCounters(t *testing.T) (rebuilds, invalidates *int) {
	t.Helper()
	var rebuildCount, invalidateCount int
	restore := SetModelRefreshSideEffectsForTest(
		func(ctx context.Context, db *sqlx.DB) (RouteRebuildStats, error) {
			rebuildCount++
			return RouteRebuildStats{}, nil
		},
		func() { invalidateCount++ },
	)
	t.Cleanup(restore)
	return &rebuildCount, &invalidateCount
}

func TestPersistTokenModelAvailability_UpsertsAndMarksUnavailable(t *testing.T) {
	db := openModelRefreshTestDB(t)
	_, tokenID := seedAccountAndToken(t, db, "sk-backfill")
	now := time.Now().UTC().Format(time.RFC3339)

	// First refresh: 2 models available.
	if err := persistTokenModelAvailability(db, tokenID, []string{"gpt-4o", "claude-3"}, now); err != nil {
		t.Fatalf("first persist: %v", err)
	}

	var availCount int
	if err := db.Get(&availCount,
		`SELECT COUNT(*) FROM token_model_availability WHERE token_id = ? AND available = TRUE`,
		tokenID,
	); err != nil {
		t.Fatalf("count available: %v", err)
	}
	if availCount != 2 {
		t.Fatalf("expected 2 available, got %d", availCount)
	}

	// Second refresh: only 1 model — the other should be marked unavailable.
	if err := persistTokenModelAvailability(db, tokenID, []string{"gpt-4o"}, now); err != nil {
		t.Fatalf("second persist: %v", err)
	}
	if err := db.Get(&availCount,
		`SELECT COUNT(*) FROM token_model_availability WHERE token_id = ? AND available = TRUE`,
		tokenID,
	); err != nil {
		t.Fatalf("count after second: %v", err)
	}
	if availCount != 1 {
		t.Fatalf("expected 1 available after shrink, got %d", availCount)
	}
	var unavailCount int
	if err := db.Get(&unavailCount,
		`SELECT COUNT(*) FROM token_model_availability WHERE token_id = ? AND available = FALSE`,
		tokenID,
	); err != nil {
		t.Fatalf("count unavailable: %v", err)
	}
	if unavailCount != 1 {
		t.Fatalf("expected 1 unavailable (claude-3), got %d", unavailCount)
	}
}

func TestPersistTokenModelAvailability_ReAddsPreviouslyUnavailable(t *testing.T) {
	db := openModelRefreshTestDB(t)
	_, tokenID := seedAccountAndToken(t, db, "sk-cycle")
	now := time.Now().UTC().Format(time.RFC3339)

	// Refresh 1: gpt-4o only.
	if err := persistTokenModelAvailability(db, tokenID, []string{"gpt-4o"}, now); err != nil {
		t.Fatalf("persist 1: %v", err)
	}
	// Refresh 2: claude-3 only (gpt-4o becomes unavailable).
	if err := persistTokenModelAvailability(db, tokenID, []string{"claude-3"}, now); err != nil {
		t.Fatalf("persist 2: %v", err)
	}
	// Refresh 3: gpt-4o comes back — should flip available=1, not insert a dupe.
	if err := persistTokenModelAvailability(db, tokenID, []string{"gpt-4o", "claude-3"}, now); err != nil {
		t.Fatalf("persist 3: %v", err)
	}

	var totalCount int
	if err := db.Get(&totalCount,
		`SELECT COUNT(*) FROM token_model_availability WHERE token_id = ?`,
		tokenID,
	); err != nil {
		t.Fatalf("count total: %v", err)
	}
	if totalCount != 2 {
		t.Fatalf("expected 2 total rows (no dupes), got %d", totalCount)
	}
	var availCount int
	if err := db.Get(&availCount,
		`SELECT COUNT(*) FROM token_model_availability WHERE token_id = ? AND available = TRUE`,
		tokenID,
	); err != nil {
		t.Fatalf("count available: %v", err)
	}
	if availCount != 2 {
		t.Fatalf("expected 2 available after re-add, got %d", availCount)
	}
}

func TestResolveAccountTokenID_MatchAndMiss(t *testing.T) {
	db := openModelRefreshTestDB(t)
	accountID, tokenID := seedAccountAndToken(t, db, "sk-match")

	gotID, ok := resolveAccountTokenID(db, accountID, "sk-match")
	if !ok {
		t.Fatal("expected match for sk-match")
	}
	if gotID != tokenID {
		t.Fatalf("token id = %d, want %d", gotID, tokenID)
	}

	// Bearer prefix should be stripped.
	gotID2, ok2 := resolveAccountTokenID(db, accountID, "Bearer sk-match")
	if !ok2 || gotID2 != tokenID {
		t.Fatalf("bearer prefix: ok=%v id=%d, want %d", ok2, gotID2, tokenID)
	}

	// Non-matching token returns false.
	_, ok3 := resolveAccountTokenID(db, accountID, "sk-nope")
	if ok3 {
		t.Fatal("expected false for non-matching token")
	}

	// Empty token returns false.
	_, ok4 := resolveAccountTokenID(db, accountID, "")
	if ok4 {
		t.Fatal("expected false for empty token")
	}
}

func TestResolveAccountTokenID_NoRows(t *testing.T) {
	db := openModelRefreshTestDB(t)
	accountID, _ := seedAccountAndToken(t, db, "sk-lone")
	_, ok := resolveAccountTokenID(db, accountID, "sk-other")
	if ok {
		t.Fatal("expected false when no account_tokens row matches")
	}
}

// TestSyncAllAccountModels_BatchSemantics is the Wave 15 #1005 acceptance
// proof: sequential per-account refreshes skip their own rebuild, and the
// whole pass triggers exactly one route rebuild + one routing-cache
// invalidation when at least one account succeeded. Candidate filtering
// (disabled / non-active-non-empty status / missing credential / unknown
// adapter) keeps ineligible accounts out of the totals entirely.
func TestSyncAllAccountModels_BatchSemantics(t *testing.T) {
	db := openModelRefreshTestDB(t)
	rebuilds, invalidates := withModelRefreshCounters(t)

	goodUpstream := startModelsUpstream(t, http.StatusOK, "gpt-4o", "gpt-4o-mini")
	badUpstream := startModelsUpstream(t, http.StatusInternalServerError)

	goodSite := seedOpenAISite(t, db, "batch-good", goodUpstream.URL)
	badSite := seedOpenAISite(t, db, "batch-bad", badUpstream.URL)

	// Candidates: one success + one honest upstream failure.
	okAccount, _ := seedAccountOnSite(t, db, goodSite, "ok-user", "sk-ok", "active")
	_ = okAccount
	failAccount, _ := seedAccountOnSite(t, db, badSite, "fail-user", "sk-fail", "active")
	_ = failAccount
	// Empty status counts as active/eligible (non-disabled semantics).
	emptyStatusAccount, _ := seedAccountOnSite(t, db, goodSite, "empty-status", "sk-empty-status", "")
	_ = emptyStatusAccount

	// Non-candidates: none of these may appear in the totals.
	seedAccountOnSite(t, db, goodSite, "disabled-user", "sk-disabled", "disabled")
	seedAccountOnSite(t, db, goodSite, "expired-user", "sk-expired", "expired")
	seedAccountOnSite(t, db, goodSite, "no-credential", "", "active")
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	if res, err := db.Exec(
		`INSERT INTO sites (name, url, platform, status, use_system_proxy, sort_order, global_weight, post_refresh_probe_enabled, created_at, updated_at)
		 VALUES ('batch-no-adapter', 'https://nope.example.com', 'no-such-platform', 'active', FALSE, 0, 0, FALSE, ?, ?)`,
		now, now,
	); err != nil {
		t.Fatalf("insert no-adapter site: %v", err)
	} else {
		noAdapterSite, _ := res.LastInsertId()
		seedAccountOnSite(t, db, noAdapterSite, "no-adapter-user", "sk-no-adapter", "active")
	}

	summary := SyncAllAccountModels(context.Background(), db)

	if summary.Total != 3 {
		t.Fatalf("total = %d, want 3 (ok + empty-status + failing upstream); non-candidates must be excluded", summary.Total)
	}
	if summary.Success != 2 {
		t.Fatalf("success = %d, want 2", summary.Success)
	}
	if summary.Failed != 1 {
		t.Fatalf("failed = %d, want 1", summary.Failed)
	}

	// Exactly one rebuild + one invalidation for the whole batch.
	if *rebuilds != 1 {
		t.Errorf("rebuild calls = %d, want exactly 1 for the batch", *rebuilds)
	}
	if *invalidates != 1 {
		t.Errorf("invalidate calls = %d, want exactly 1 for the batch", *invalidates)
	}

	// Availability for the successful accounts really landed in the store.
	var availCount int
	if err := db.Get(&availCount,
		`SELECT COUNT(*) FROM model_availability WHERE account_id = ? AND available = TRUE`,
		okAccount,
	); err != nil {
		t.Fatalf("count availability: %v", err)
	}
	if availCount != 2 {
		t.Fatalf("ok-account available rows = %d, want 2", availCount)
	}
}

// TestSyncAllAccountModels_ZeroSuccessStillRebuilds pins the #1174 invariant: a
// pass whose every upstream refresh failed still ends with exactly one rebuild.
// Routes are composed from the availability already in the store, so skipping
// the local step because the *upstream* step failed left operators with routes
// that no pass would ever recompose — and the rebuild short-circuits without a
// write when nothing changed, so this costs a read pass, not churn.
func TestSyncAllAccountModels_ZeroSuccessStillRebuilds(t *testing.T) {
	db := openModelRefreshTestDB(t)
	rebuilds, invalidates := withModelRefreshCounters(t)

	badUpstream := startModelsUpstream(t, http.StatusInternalServerError)
	badSite := seedOpenAISite(t, db, "all-fail", badUpstream.URL)
	seedAccountOnSite(t, db, badSite, "fail-1", "sk-fail-1", "active")
	seedAccountOnSite(t, db, badSite, "fail-2", "sk-fail-2", "active")

	summary := SyncAllAccountModels(context.Background(), db)

	if summary.Total != 2 || summary.Success != 0 || summary.Failed != 2 {
		t.Fatalf("summary = %+v, want total=2 success=0 failed=2", summary)
	}
	if *rebuilds != 1 || *invalidates != 1 {
		t.Errorf("side effects with zero success: rebuilds=%d invalidates=%d, want 1/1", *rebuilds, *invalidates)
	}
}

// TestSyncAllAccountModels_RebuildSurvivesACanceledPassContext is the other half
// of #1174: the pass ran on its caller's context, so a caller that went away —
// a browser that timed out on POST /api/routes/rebuild, a proxy read timeout —
// canceled the model-sync loop *and* the local rebuild behind it, and the
// handler answered "route rebuild failed: context canceled". The rebuild reads
// and writes only the local database, so it must not inherit that cancellation.
func TestSyncAllAccountModels_RebuildSurvivesACanceledPassContext(t *testing.T) {
	db := openModelRefreshTestDB(t)
	rebuilds, _ := withModelRefreshCounters(t)

	upstream := startModelsUpstream(t, http.StatusOK, "gpt-4o")
	siteID := seedOpenAISite(t, db, "canceled-pass", upstream.URL)
	seedAccountOnSite(t, db, siteID, "canceled-user", "sk-canceled", "active")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	summary := SyncAllAccountModels(ctx, db)

	if summary.Total != 1 || summary.Success != 0 {
		t.Fatalf("summary = %+v, want total=1 success=0 (the pass must stop at once)", summary)
	}
	if *rebuilds != 1 {
		t.Fatalf("rebuild calls = %d, want 1: the local recomposition must survive the canceled pass context", *rebuilds)
	}
	if summary.RebuildErr != nil {
		t.Fatalf("rebuild error = %v, want nil", summary.RebuildErr)
	}
}

// TestRefreshAccountModels_SkipRebuildPersistsWithoutSideEffects verifies the
// batch-mode single-account path: availability persists, but rebuild and
// cache invalidation are deferred to the caller.
func TestRefreshAccountModels_SkipRebuildPersistsWithoutSideEffects(t *testing.T) {
	db := openModelRefreshTestDB(t)
	rebuilds, invalidates := withModelRefreshCounters(t)

	upstream := startModelsUpstream(t, http.StatusOK, "gpt-4o")
	siteID := seedOpenAISite(t, db, "skip-rebuild", upstream.URL)
	accountID, _ := seedAccountOnSite(t, db, siteID, "skip-user", "sk-skip", "active")

	result := RefreshAccountModels(context.Background(), db, accountID, false, true)
	if !result.Success {
		t.Fatalf("refresh failed: %s / %s", result.ErrorCode, result.ErrorMessage)
	}
	if result.RebuildRan {
		t.Error("RebuildRan = true with skipRebuild, want false")
	}
	if *rebuilds != 0 || *invalidates != 0 {
		t.Errorf("skipRebuild side effects: rebuilds=%d invalidates=%d, want 0/0", *rebuilds, *invalidates)
	}

	var availCount int
	if err := db.Get(&availCount,
		`SELECT COUNT(*) FROM model_availability WHERE account_id = ? AND available = TRUE`,
		accountID,
	); err != nil {
		t.Fatalf("count availability: %v", err)
	}
	if availCount != 1 {
		t.Fatalf("available rows = %d, want 1", availCount)
	}
}

// TestRefreshAccountModels_InlineRebuildRunsOnce pins the single-account
// admin path (skipRebuild=false): exactly one rebuild + one invalidation.
func TestRefreshAccountModels_InlineRebuildRunsOnce(t *testing.T) {
	db := openModelRefreshTestDB(t)
	rebuilds, invalidates := withModelRefreshCounters(t)

	upstream := startModelsUpstream(t, http.StatusOK, "gpt-4o")
	siteID := seedOpenAISite(t, db, "inline-rebuild", upstream.URL)
	accountID, _ := seedAccountOnSite(t, db, siteID, "inline-user", "sk-inline", "active")

	result := RefreshAccountModels(context.Background(), db, accountID, false, false)
	if !result.Success {
		t.Fatalf("refresh failed: %s / %s", result.ErrorCode, result.ErrorMessage)
	}
	if !result.RebuildRan {
		t.Error("RebuildRan = false, want true for the inline path")
	}
	if *rebuilds != 1 || *invalidates != 1 {
		t.Errorf("inline side effects: rebuilds=%d invalidates=%d, want 1/1", *rebuilds, *invalidates)
	}
}

// TestRefreshAccountModels_StatusGuards covers disabled/inactive gating and
// the allowInactive recovery path without touching the network.
func TestRefreshAccountModels_StatusGuards(t *testing.T) {
	db := openModelRefreshTestDB(t)
	siteID := seedOpenAISite(t, db, "guards", "https://guards.example.com")

	disabledID, _ := seedAccountOnSite(t, db, siteID, "disabled", "sk-disabled", "disabled")
	if res := RefreshAccountModels(context.Background(), db, disabledID, true, true); res.ErrorCode != "disabled" {
		t.Fatalf("disabled account: ErrorCode = %q, want disabled", res.ErrorCode)
	}

	expiredID, _ := seedAccountOnSite(t, db, siteID, "expired", "sk-expired", "expired")
	if res := RefreshAccountModels(context.Background(), db, expiredID, false, true); res.ErrorCode != "inactive" {
		t.Fatalf("expired account with allowInactive=false: ErrorCode = %q, want inactive", res.ErrorCode)
	}

	noCredID, _ := seedAccountOnSite(t, db, siteID, "no-cred", "", "active")
	if res := RefreshAccountModels(context.Background(), db, noCredID, false, true); res.ErrorCode != "missing_credential" {
		t.Fatalf("credential-less account: ErrorCode = %q, want missing_credential", res.ErrorCode)
	}
}

func TestCleanModelNames(t *testing.T) {
	got := cleanModelNames([]string{" gpt-4o ", "", "GPT-4o", "gpt-4o-mini", "\tgpt-4o-mini\t"})
	want := []string{"gpt-4o", "gpt-4o-mini"}
	if len(got) != len(want) {
		t.Fatalf("cleanModelNames = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("cleanModelNames[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if got := cleanModelNames(nil); len(got) != 0 {
		t.Fatalf("cleanModelNames(nil) = %#v, want empty", got)
	}
}
