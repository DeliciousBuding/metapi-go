package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/store"
)

// Observed failure (v0.19 stability window, Aged run on a real New API
// upstream): the operator deleted one upstream key and created another. The
// documented recovery — re-login the account — synced the new key in but left
// the revoked one as the default, so accounts.api_token kept the dead value and
// relay stayed 401/503 until someone set the default by hand and rebuilt
// routes. Sync answered success:true the whole time.
func TestSyncTokensFromUpstreamSwitchesDefaultWhenStoredDefaultGoneUpstream(t *testing.T) {
	db := openTestDB(t)
	siteID := createTestSite(t, db, "ConvergeDefaultSite", "https://converge-default.example.com", "new-api")
	accountID := createTestAccount(t, db, siteID, strPtr("converge-user"), "dashboard-pat")
	revokedID := createTestAccountToken(t, db, accountID, "metapi-aged", "sk-revoked-key", true)
	if _, err := db.Exec("UPDATE accounts SET api_token = ? WHERE id = ?", "sk-revoked-key", accountID); err != nil {
		t.Fatalf("seed accounts.api_token: %v", err)
	}

	result, err := SyncTokensFromUpstream(db.DB, accountID, testTokenListing([]UpstreamAPIToken{
		{Name: "metapi-aged-2", Key: "sk-live-key", Enabled: true, TokenGroup: "default"},
	}, false))
	if err != nil {
		t.Fatalf("SyncTokensFromUpstream: %v", err)
	}
	if result.Created != 1 {
		t.Fatalf("created = %d, want 1", result.Created)
	}
	if result.DefaultSwitchSkipped != "" {
		t.Fatalf("DefaultSwitchSkipped = %q, want empty", result.DefaultSwitchSkipped)
	}
	sw := result.DefaultSwitch
	if sw == nil {
		t.Fatal("DefaultSwitch = nil, want the default relay credential to move to the key upstream still lists")
	}
	if sw.FromTokenID != revokedID || sw.FromTokenName != "metapi-aged" {
		t.Fatalf("DefaultSwitch.From = (%d, %q), want (%d, %q)", sw.FromTokenID, sw.FromTokenName, revokedID, "metapi-aged")
	}
	if sw.ToTokenName != "metapi-aged-2" {
		t.Fatalf("DefaultSwitch.ToTokenName = %q, want metapi-aged-2", sw.ToTokenName)
	}

	assertTokenDefaultState(t, db, revokedID, false)
	assertTokenDefaultState(t, db, sw.ToTokenID, true)

	var apiToken *string
	if err := db.QueryRow("SELECT api_token FROM accounts WHERE id = ?", accountID).Scan(&apiToken); err != nil {
		t.Fatalf("read accounts.api_token: %v", err)
	}
	if apiToken == nil || *apiToken != "sk-live-key" {
		t.Fatalf("accounts.api_token = %v, want the live upstream key (this is the credential relay and the model refresh use)", apiToken)
	}
	if result.DefaultTokenID == nil || *result.DefaultTokenID != sw.ToTokenID {
		t.Fatalf("DefaultTokenID = %v, want %d", result.DefaultTokenID, sw.ToTokenID)
	}

	// The revoked row is a record, not garbage: it stays readable and enabled
	// state is untouched, so an operator can still see what was replaced.
	var stillThere int
	var enabled bool
	if err := db.QueryRow("SELECT COUNT(*), MAX(enabled) FROM account_tokens WHERE id = ?", revokedID).Scan(&stillThere, &enabled); err != nil {
		t.Fatalf("read revoked row: %v", err)
	}
	if stillThere != 1 {
		t.Fatal("the revoked token row was deleted; sync must converge the default, not destroy history")
	}
}

func TestSyncTokensFromUpstreamLeavesDefaultAloneWhenStillListedUpstream(t *testing.T) {
	db := openTestDB(t)
	siteID := createTestSite(t, db, "ConvergeNoopSite", "https://converge-noop.example.com", "new-api")
	accountID := createTestAccount(t, db, siteID, strPtr("noop-user"), "dashboard-pat")
	defaultID := createTestAccountToken(t, db, accountID, "kept", "sk-kept-key", true)

	result, err := SyncTokensFromUpstream(db.DB, accountID, testTokenListing([]UpstreamAPIToken{
		{Name: "kept", Key: "sk-kept-key", Enabled: true, TokenGroup: "default"},
		{Name: "second", Key: "sk-second-key", Enabled: true, TokenGroup: "default"},
	}, false))
	if err != nil {
		t.Fatalf("SyncTokensFromUpstream: %v", err)
	}
	if result.DefaultSwitch != nil {
		t.Fatalf("DefaultSwitch = %#v, want nil while the default key is still listed upstream", result.DefaultSwitch)
	}
	if result.DefaultSwitchSkipped != "" {
		t.Fatalf("DefaultSwitchSkipped = %q, want empty (no absence, no skip decision)", result.DefaultSwitchSkipped)
	}
	assertTokenDefaultState(t, db, defaultID, true)
}

func convergenceAPITokens(count int) []platform.ApiTokenInfo {
	tokens := make([]platform.ApiTokenInfo, 0, count)
	for i := 0; i < count-1; i++ {
		tokens = append(tokens, platform.ApiTokenInfo{
			Name:       fmt.Sprintf("filler-%d", i),
			Key:        fmt.Sprintf("sk-filler-%d", i),
			Enabled:    true,
			TokenGroup: "default",
		})
	}
	tokens = append(tokens, platform.ApiTokenInfo{
		Name:       "live-replacement",
		Key:        "sk-live-key",
		Enabled:    true,
		TokenGroup: "default",
	})
	return tokens
}

func seedConvergenceDefault(t *testing.T, name, baseURL string) (*store.DB, int64, int64, int64) {
	t.Helper()
	db := openTestDB(t)
	siteID := createTestSite(t, db, name, baseURL, "new-api")
	accountID := createTestAccount(t, db, siteID, strPtr(strings.ToLower(name)), "dashboard-pat")
	defaultID := createTestAccountToken(t, db, accountID, "revoked-default", "sk-revoked-key", true)
	replacementID := createTestAccountToken(t, db, accountID, "live-replacement", "sk-live-key", true)
	if _, err := db.Exec("UPDATE accounts SET api_token = ? WHERE id = ?", "sk-revoked-key", accountID); err != nil {
		t.Fatalf("seed accounts.api_token: %v", err)
	}
	return db, accountID, defaultID, replacementID
}

func assertDefaultConverged(t *testing.T, db *store.DB, defaultID, replacementID int64, result *TokenSyncResult) {
	t.Helper()
	if result.DefaultSwitch == nil {
		t.Fatalf("DefaultSwitch = nil, want convergence to the listed replacement")
	}
	if result.DefaultSwitchSkipped != "" {
		t.Fatalf("DefaultSwitchSkipped = %q, want empty for a complete listing", result.DefaultSwitchSkipped)
	}
	if result.DefaultSwitch.FromTokenID != defaultID || result.DefaultSwitch.ToTokenID != replacementID {
		t.Fatalf("DefaultSwitch = %+v, want %d -> %d", result.DefaultSwitch, defaultID, replacementID)
	}
	assertTokenDefaultState(t, db, defaultID, false)
	assertTokenDefaultState(t, db, replacementID, true)
}

func newCompleteNewAPIListingServer(t *testing.T, count int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/token/" {
			http.NotFound(w, r)
			return
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("p"))
		start := (page - 1) * platform.UpstreamTokenListPageLimit
		end := start + platform.UpstreamTokenListPageLimit
		if end > count {
			end = count
		}
		items := make([]map[string]interface{}, 0, end-start)
		for i := start; i < end; i++ {
			name := fmt.Sprintf("token-%d", i+1)
			key := fmt.Sprintf("sk-filler-%d", i)
			if i == count-1 {
				name = "live-replacement"
				key = "sk-live-key"
			}
			items = append(items, map[string]interface{}{
				"id":     i + 1,
				"name":   name,
				"key":    key,
				"status": 1,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"data": map[string]interface{}{
				"items":     items,
				"total":     count,
				"page_size": platform.UpstreamTokenListPageLimit,
			},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

// A complete New API listing may end exactly on a page boundary. The explicit
// completeness proof, not the token count or remainder, authorizes convergence.
func TestSyncTokensFromUpstreamCompleteNewAPIListingAllowsDefaultSwitch(t *testing.T) {
	for _, count := range []int{platform.UpstreamTokenListPageLimit, platform.UpstreamTokenListPageLimit * 2} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			srv := newCompleteNewAPIListingServer(t, count)
			db, accountID, defaultID, replacementID := seedConvergenceDefault(t, "CompleteListing", srv.URL)

			adapter := &platform.NewApiAdapter{BaseAdapter: platform.NewBaseAdapter("new-api")}
			listing, err := FetchUpstreamAPITokenListing(context.Background(), adapter, srv.URL, "dashboard-pat", nil, nil)
			if err != nil {
				t.Fatalf("FetchUpstreamAPITokenListing: %v", err)
			}
			if !listing.Complete {
				t.Fatalf("Complete = false for a fully validated %d-token New API listing", count)
			}
			if len(listing.Tokens) != count {
				t.Fatalf("tokens = %d, want %d", len(listing.Tokens), count)
			}

			result, err := SyncTokensFromUpstream(db.DB, accountID, listing)
			if err != nil {
				t.Fatalf("SyncTokensFromUpstream: %v", err)
			}
			assertDefaultConverged(t, db, defaultID, replacementID, result)
		})
	}
}

// Unknown or incomplete sources must keep the legacy page-limit guard. A 101st
// row is not proof that the adapter returned every row.
func TestSyncTokensFromUpstreamIncompleteListingKeepsDefault(t *testing.T) {
	for _, count := range []int{platform.UpstreamTokenListPageLimit + 1, platform.UpstreamTokenListPageLimit * 2} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			db, accountID, defaultID, _ := seedConvergenceDefault(t, "IncompleteListing", "https://incomplete.example.com")
			adapter := &stubTokenAdapter{tokens: convergenceAPITokens(count)}
			listing, err := FetchUpstreamAPITokenListing(context.Background(), adapter, "https://incomplete.example.com", "dashboard-pat", nil, nil)
			if err != nil {
				t.Fatalf("FetchUpstreamAPITokenListing: %v", err)
			}
			if listing.Complete {
				t.Fatalf("Complete = true for unknown adapter with %d tokens", count)
			}

			result, err := SyncTokensFromUpstream(db.DB, accountID, listing)
			if err != nil {
				t.Fatalf("SyncTokensFromUpstream: %v", err)
			}
			if result.DefaultSwitch != nil {
				t.Fatalf("DefaultSwitch = %#v, want nil for an incomplete listing", result.DefaultSwitch)
			}
			if result.DefaultSwitchSkipped != "upstream_listing_may_be_truncated" {
				t.Fatalf("DefaultSwitchSkipped = %q, want upstream_listing_may_be_truncated", result.DefaultSwitchSkipped)
			}
			assertTokenDefaultState(t, db, defaultID, true)
		})
	}
}

// Sub2API exposes no explicit completeness contract and its legacy listing
// path can combine or truncate endpoint results. 101 rows must therefore remain
// incomplete even though the count is not a page-limit multiple.
func TestSyncTokensFromUpstreamSub2API101PartialListingKeepsDefault(t *testing.T) {
	db, accountID, defaultID, _ := seedConvergenceDefault(t, "Sub2Partial", "https://sub2-partial.example.com")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/keys" {
			http.NotFound(w, r)
			return
		}
		items := make([]map[string]interface{}, 0, platform.UpstreamTokenListPageLimit+1)
		for _, token := range convergenceAPITokens(platform.UpstreamTokenListPageLimit + 1) {
			items = append(items, map[string]interface{}{
				"id":     len(items) + 1,
				"name":   token.Name,
				"key":    token.Key,
				"status": 1,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"items": items},
		})
	}))
	t.Cleanup(srv.Close)

	adapter := &platform.Sub2ApiAdapter{BaseAdapter: platform.NewBaseAdapter("sub2api")}
	listing, err := FetchUpstreamAPITokenListing(context.Background(), adapter, srv.URL, "session-jwt", nil, nil)
	if err != nil {
		t.Fatalf("FetchUpstreamAPITokenListing: %v", err)
	}
	if listing.Complete {
		t.Fatal("Complete = true for a Sub2API partial 101-row listing")
	}
	if len(listing.Tokens) != platform.UpstreamTokenListPageLimit+1 {
		t.Fatalf("tokens = %d, want %d", len(listing.Tokens), platform.UpstreamTokenListPageLimit+1)
	}

	result, err := SyncTokensFromUpstream(db.DB, accountID, listing)
	if err != nil {
		t.Fatalf("SyncTokensFromUpstream: %v", err)
	}
	if result.DefaultSwitch != nil {
		t.Fatalf("DefaultSwitch = %#v, want nil for Sub2API partial 101-row listing", result.DefaultSwitch)
	}
	if result.DefaultSwitchSkipped != "upstream_listing_may_be_truncated" {
		t.Fatalf("DefaultSwitchSkipped = %q, want upstream_listing_may_be_truncated", result.DefaultSwitchSkipped)
	}
	assertTokenDefaultState(t, db, defaultID, true)
}

// A legacy flat response to the current New API query has no page marker. It
// must be discarded, retried through p=0&size=100, and still must not authorize
// a default switch because the legacy response cannot prove completeness.
func TestSyncTokensFromUpstreamLegacyFlatWrongCurrentPageKeepsDefault(t *testing.T) {
	var currentCalls atomic.Int32
	var legacyCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/token/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Query().Get("p") == "1" && r.URL.Query().Get("page_size") == strconv.Itoa(platform.UpstreamTokenListPageLimit):
			currentCalls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"data": []interface{}{
					map[string]interface{}{"id": 3, "name": "wrong-current-page", "key": "sk-test-wrong", "status": 1},
				},
			})
		case r.URL.Query().Get("p") == "0" && r.URL.Query().Get("size") == strconv.Itoa(platform.UpstreamTokenListPageLimit):
			legacyCalls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"data": []interface{}{
					map[string]interface{}{"id": 2, "name": "live-replacement", "key": "sk-live-key", "status": 1},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	db, accountID, defaultID, replacementID := seedConvergenceDefault(t, "LegacyFlatCurrentPage", srv.URL)
	adapter := &platform.NewApiAdapter{BaseAdapter: platform.NewBaseAdapter("new-api")}
	listing, err := FetchUpstreamAPITokenListing(context.Background(), adapter, srv.URL, "dashboard-pat", nil, nil)
	if err != nil {
		t.Fatalf("FetchUpstreamAPITokenListing: %v", err)
	}
	if listing.Complete {
		t.Fatal("Complete = true for a legacy flat response without total")
	}
	if !listing.DefaultSwitchUnsafe {
		t.Fatal("DefaultSwitchUnsafe = false for a legacy flat response without total")
	}
	if len(listing.Tokens) != 1 || listing.Tokens[0].Key != "sk-live-key" {
		t.Fatalf("tokens = %+v, want the legacy p=0 homepage, not the current-query page", listing.Tokens)
	}
	if currentCalls.Load() != 1 || legacyCalls.Load() != 1 {
		t.Fatalf("current/legacy calls = %d/%d, want 1/1", currentCalls.Load(), legacyCalls.Load())
	}

	result, err := SyncTokensFromUpstream(db.DB, accountID, listing)
	if err != nil {
		t.Fatalf("SyncTokensFromUpstream: %v", err)
	}
	if result.DefaultSwitch != nil {
		t.Fatalf("DefaultSwitch = %#v, want nil for an unproven legacy flat listing", result.DefaultSwitch)
	}
	if result.DefaultSwitchSkipped != "upstream_listing_completeness_unproven" {
		t.Fatalf("DefaultSwitchSkipped = %q, want upstream_listing_completeness_unproven", result.DefaultSwitchSkipped)
	}
	assertTokenDefaultState(t, db, defaultID, true)
	assertTokenDefaultState(t, db, replacementID, false)
}

// New API lists masked keys (`sk-****`) unless the batch-keys endpoint hydrates
// them (#1179). A hydrated real key never equals its own mask, so absence in a
// masked listing is an artifact of hydration, not a revocation.
func TestSyncTokensFromUpstreamSkipsDefaultSwitchOnMaskedListing(t *testing.T) {
	db := openTestDB(t)
	siteID := createTestSite(t, db, "ConvergeMaskedSite", "https://converge-masked.example.com", "new-api")
	accountID := createTestAccount(t, db, siteID, strPtr("masked-user"), "dashboard-pat")
	defaultID := createTestAccountToken(t, db, accountID, "hydrated", "sk-real-key-value", true)

	result, err := SyncTokensFromUpstream(db.DB, accountID, testTokenListing([]UpstreamAPIToken{
		{Name: "hydrated", Key: "sk-r*********alue", Enabled: true, TokenGroup: "default"},
	}, false))
	if err != nil {
		t.Fatalf("SyncTokensFromUpstream: %v", err)
	}
	if result.DefaultSwitch != nil {
		t.Fatalf("DefaultSwitch = %#v, want nil when the listing only carries masked display values", result.DefaultSwitch)
	}
	if result.DefaultSwitchSkipped != "upstream_listing_is_masked" {
		t.Fatalf("DefaultSwitchSkipped = %q, want upstream_listing_is_masked", result.DefaultSwitchSkipped)
	}
	assertTokenDefaultState(t, db, defaultID, true)
}

// Operator intent outranks convergence: a key the operator disabled is not an
// eligible replacement, and sync must not re-enable it (existing contract).
func TestSyncTokensFromUpstreamSkipsDefaultSwitchWhenReplacementIsOperatorDisabled(t *testing.T) {
	db := openTestDB(t)
	siteID := createTestSite(t, db, "ConvergeDisabledSite", "https://converge-disabled.example.com", "new-api")
	accountID := createTestAccount(t, db, siteID, strPtr("disabled-user"), "dashboard-pat")
	defaultID := createTestAccountToken(t, db, accountID, "revoked-default", "sk-revoked-key", true)
	disabledID := createTestAccountTokenWithEnabled(t, db, accountID, "ops-disabled", "sk-live-key", false, false)

	result, err := SyncTokensFromUpstream(db.DB, accountID, testTokenListing([]UpstreamAPIToken{
		{Name: "ops-disabled", Key: "sk-live-key", Enabled: true, TokenGroup: "default"},
	}, false))
	if err != nil {
		t.Fatalf("SyncTokensFromUpstream: %v", err)
	}
	if result.DefaultSwitch != nil {
		t.Fatalf("DefaultSwitch = %#v, want nil: the only listed key is operator-disabled", result.DefaultSwitch)
	}
	if result.DefaultSwitchSkipped != "no_enabled_token_listed_upstream" {
		t.Fatalf("DefaultSwitchSkipped = %q, want no_enabled_token_listed_upstream", result.DefaultSwitchSkipped)
	}
	assertTokenDefaultState(t, db, defaultID, true)
	assertTokenDefaultState(t, db, disabledID, false)

	var enabled bool
	if err := db.QueryRow("SELECT enabled FROM account_tokens WHERE id = ?", disabledID).Scan(&enabled); err != nil {
		t.Fatalf("read disabled token: %v", err)
	}
	if enabled {
		t.Fatal("sync re-enabled an operator-disabled token while looking for a replacement")
	}
}
