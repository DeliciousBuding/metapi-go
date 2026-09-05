package service

import (
	"strconv"
	"testing"

	"github.com/deliciousbuding/metapi-go/platform"
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

	result, err := SyncTokensFromUpstream(db.DB, accountID, []UpstreamAPIToken{
		{Name: "metapi-aged-2", Key: "sk-live-key", Enabled: true, TokenGroup: "default"},
	})
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

	result, err := SyncTokensFromUpstream(db.DB, accountID, []UpstreamAPIToken{
		{Name: "kept", Key: "sk-kept-key", Enabled: true, TokenGroup: "default"},
		{Name: "second", Key: "sk-second-key", Enabled: true, TokenGroup: "default"},
	})
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

// A listing that fills the page may be truncated, and "absent from a truncated
// listing" does not prove revocation. Guessing here would move the default away
// from a perfectly good key.
func TestSyncTokensFromUpstreamSkipsDefaultSwitchOnTruncatedListing(t *testing.T) {
	db := openTestDB(t)
	siteID := createTestSite(t, db, "ConvergeTruncatedSite", "https://converge-truncated.example.com", "new-api")
	accountID := createTestAccount(t, db, siteID, strPtr("truncated-user"), "dashboard-pat")
	defaultID := createTestAccountToken(t, db, accountID, "kept", "sk-kept-key", true)

	upstream := make([]UpstreamAPIToken, 0, platform.UpstreamTokenListPageLimit)
	for i := 0; i < platform.UpstreamTokenListPageLimit; i++ {
		upstream = append(upstream, UpstreamAPIToken{
			Name:       "filler",
			Key:        "sk-filler-" + strconv.Itoa(i),
			Enabled:    true,
			TokenGroup: "default",
		})
	}

	result, err := SyncTokensFromUpstream(db.DB, accountID, upstream)
	if err != nil {
		t.Fatalf("SyncTokensFromUpstream: %v", err)
	}
	if result.DefaultSwitch != nil {
		t.Fatalf("DefaultSwitch = %#v, want nil when the listing may be truncated", result.DefaultSwitch)
	}
	if result.DefaultSwitchSkipped != "upstream_listing_may_be_truncated" {
		t.Fatalf("DefaultSwitchSkipped = %q, want upstream_listing_may_be_truncated", result.DefaultSwitchSkipped)
	}
	assertTokenDefaultState(t, db, defaultID, true)
}

// New API lists masked keys (`sk-****`) unless the batch-keys endpoint hydrates
// them (#1179). A hydrated real key never equals its own mask, so absence in a
// masked listing is an artifact of hydration, not a revocation.
func TestSyncTokensFromUpstreamSkipsDefaultSwitchOnMaskedListing(t *testing.T) {
	db := openTestDB(t)
	siteID := createTestSite(t, db, "ConvergeMaskedSite", "https://converge-masked.example.com", "new-api")
	accountID := createTestAccount(t, db, siteID, strPtr("masked-user"), "dashboard-pat")
	defaultID := createTestAccountToken(t, db, accountID, "hydrated", "sk-real-key-value", true)

	result, err := SyncTokensFromUpstream(db.DB, accountID, []UpstreamAPIToken{
		{Name: "hydrated", Key: "sk-r*********alue", Enabled: true, TokenGroup: "default"},
	})
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

	result, err := SyncTokensFromUpstream(db.DB, accountID, []UpstreamAPIToken{
		{Name: "ops-disabled", Key: "sk-live-key", Enabled: true, TokenGroup: "default"},
	})
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
