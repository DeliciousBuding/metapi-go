package service

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

func seedRouteCreationModel(t *testing.T, db *store.DB, tokenID int64, model string) {
	t.Helper()
	if _, err := db.Exec("INSERT INTO token_model_availability (token_id, model_name, available) VALUES (?, ?, ?)", tokenID, model, true); err != nil {
		t.Fatal(err)
	}
}

func TestAutomaticModelRoutesAreOptInAndIdempotentAtFleetScale(t *testing.T) {
	db := setupRouteRebuildDB(t)
	// The reported workflow has >100 accounts. Shared models must produce one
	// route each, not one route per account or per availability source.
	for i := 0; i < 120; i++ {
		_, _, tokenID := seedSiteAccountToken(t, db, fmt.Sprintf("fleet-%d", i))
		seedRouteCreationModel(t, db, tokenID, "deepseek-v4-flash")
		seedRouteCreationModel(t, db, tokenID, "gpt-4o")
	}
	off, err := RebuildTokenRoutesFromAvailabilityWithOptions(context.Background(), db.DB, RebuildOptions{})
	if err != nil || off.RoutesConsidered != 0 || off.RoutesCreated != 0 {
		t.Fatalf("default unexpectedly creates routes: %+v %v", off, err)
	}
	enabled := RebuildOptions{CreateModelRoutes: true}
	first, err := RebuildTokenRoutesFromAvailabilityWithOptions(context.Background(), db.DB, enabled)
	if err != nil {
		t.Fatal(err)
	}
	if first.RoutesCreated != 2 || first.PatternRoutes != 2 || first.ChannelsInserted != 240 || !first.Changed {
		t.Fatalf("fleet was not materialized: %+v", first)
	}
	var malformed int
	if err := db.Get(&malformed, "SELECT COUNT(*) FROM token_routes WHERE model_pattern <> display_name OR route_mode <> 'pattern' OR routing_strategy <> 'weighted' OR NOT enabled"); err != nil {
		t.Fatal(err)
	}
	if malformed != 0 {
		t.Fatalf("%d malformed generated routes", malformed)
	}
	second, err := RebuildTokenRoutesFromAvailabilityWithOptions(context.Background(), db.DB, enabled)
	if err != nil || second.RoutesCreated != 0 || second.ChannelsInserted != 0 || second.ChannelsRemoved != 0 || second.Changed {
		t.Fatalf("repeat pass churned routing state: %+v %v", second, err)
	}
}

func TestAutomaticModelRoutesRespectOperatorPatternsAndDisabledRoutes(t *testing.T) {
	db := setupRouteRebuildDB(t)
	_, _, tokenID := seedSiteAccountToken(t, db, "manual-policy")
	for _, model := range []string{"gpt-4o", "claude-sonnet", "team-alias", "deepseek-v4-flash"} {
		seedRouteCreationModel(t, db, tokenID, model)
	}
	for _, row := range []struct {
		pattern, mode string
		enabled       bool
	}{
		{"gpt-*", "pattern", true}, {"claude-sonnet", "pattern", false}, {"team-alias", "explicit_group", false},
	} {
		if _, err := db.Exec("INSERT INTO token_routes (model_pattern, display_name, route_mode, enabled) VALUES (?, 'operator name', ?, ?)", row.pattern, row.mode, row.enabled); err != nil {
			t.Fatal(err)
		}
	}
	stats, err := RebuildTokenRoutesFromAvailabilityWithOptions(context.Background(), db.DB, RebuildOptions{CreateModelRoutes: true})
	if err != nil || stats.RoutesCreated != 1 {
		t.Fatalf("operator policy shadowed: %+v %v", stats, err)
	}
	var disabled int
	if err := db.Get(&disabled, "SELECT COUNT(*) FROM token_routes WHERE display_name = 'operator name' AND NOT enabled"); err != nil {
		t.Fatal(err)
	}
	if disabled != 2 {
		t.Fatalf("manual disabled routes changed: %d", disabled)
	}
	var exact int
	if err := db.Get(&exact, "SELECT COUNT(*) FROM token_routes WHERE model_pattern = 'gpt-4o'"); err != nil {
		t.Fatal(err)
	}
	if exact != 0 {
		t.Fatal("generated exact route took precedence over manual wildcard")
	}
}

func TestAutomaticModelRoutesDelistChannelsWithoutDeletingRoutesOrManualBindings(t *testing.T) {
	db := setupRouteRebuildDB(t)
	_, accountID, tokenID := seedSiteAccountToken(t, db, "model-lifecycle")
	seedRouteCreationModel(t, db, tokenID, "gpt-4o")
	opts := RebuildOptions{CreateModelRoutes: true}
	if _, err := RebuildTokenRoutesFromAvailabilityWithOptions(context.Background(), db.DB, opts); err != nil {
		t.Fatal(err)
	}
	var routeID int64
	if err := db.Get(&routeID, "SELECT id FROM token_routes WHERE model_pattern = 'gpt-4o'"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO route_channels (route_id, account_id, source_model, priority, manual_override, enabled) VALUES (?, ?, 'manual-override', 99, ?, ?)", routeID, accountID, true, true); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE token_model_availability SET available = ? WHERE token_id = ?", false, tokenID); err != nil {
		t.Fatal(err)
	}
	removed, err := RebuildTokenRoutesFromAvailabilityWithOptions(context.Background(), db.DB, opts)
	if err != nil || removed.ChannelsRemoved != 1 || removed.RoutesCreated != 0 {
		t.Fatalf("delisted model not detached: %+v %v", removed, err)
	}
	var remaining int
	if err := db.Get(&remaining, "SELECT COUNT(*) FROM route_channels WHERE route_id = ? AND manual_override AND priority = 99", routeID); err != nil {
		t.Fatal(err)
	}
	if remaining != 1 {
		t.Fatal("manual binding lost on delisting")
	}
	if _, err := db.Exec("UPDATE token_model_availability SET available = ? WHERE token_id = ?", true, tokenID); err != nil {
		t.Fatal(err)
	}
	restored, err := RebuildTokenRoutesFromAvailabilityWithOptions(context.Background(), db.DB, opts)
	if err != nil || restored.RoutesCreated != 0 || restored.ChannelsInserted != 1 {
		t.Fatalf("model return churned route identity: %+v %v", restored, err)
	}
	var idAfter int64
	if err := db.Get(&idAfter, "SELECT id FROM token_routes WHERE model_pattern = 'gpt-4o'"); err != nil {
		t.Fatal(err)
	}
	if idAfter != routeID {
		t.Fatal("route id changed, invalidating downstream grants")
	}
}

func TestAutomaticModelRoutesSkipNonLiteralAndUnusableSources(t *testing.T) {
	db := setupRouteRebuildDB(t)
	_, _, usable := seedSiteAccountToken(t, db, "usable")
	for _, model := range []string{"gpt-4o", "gpt-*", "re:.*"} {
		seedRouteCreationModel(t, db, usable, model)
	}
	for i, state := range []string{"disabled", "expired", "masked_pending", "no-value", "site-disabled", "account-disabled"} {
		siteID, accountID, tokenID := seedSiteAccountToken(t, db, fmt.Sprintf("unusable-%d", i))
		seedRouteCreationModel(t, db, tokenID, "should-not-create-"+state)
		var err error
		switch state {
		case "disabled":
			_, err = db.Exec("UPDATE account_tokens SET enabled = ? WHERE id = ?", false, tokenID)
		case "no-value":
			_, err = db.Exec("UPDATE account_tokens SET token = '' WHERE id = ?", tokenID)
		case "site-disabled":
			_, err = db.Exec("UPDATE sites SET status = 'disabled' WHERE id = ?", siteID)
		case "account-disabled":
			_, err = db.Exec("UPDATE accounts SET status = 'disabled' WHERE id = ?", accountID)
		default:
			_, err = db.Exec("UPDATE account_tokens SET value_status = ? WHERE id = ?", state, tokenID)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	stats, err := RebuildTokenRoutesFromAvailabilityWithOptions(context.Background(), db.DB, RebuildOptions{CreateModelRoutes: true})
	if err != nil || stats.RoutesCreated != 1 || stats.UnsafeModelsSkipped != 2 {
		t.Fatalf("invalid model route creation: %+v %v", stats, err)
	}
}

func TestAutomaticModelRoutesIncludeAccountRelayAndOAuthModels(t *testing.T) {
	db := setupRouteRebuildDB(t)
	for i, mode := range []string{"api-key", "oauth", "dashboard-only"} {
		_, accountID, _ := seedSiteAccountToken(t, db, fmt.Sprintf("account-model-%d", i))
		if _, err := db.Exec("INSERT INTO model_availability (account_id, model_name, available, is_manual) VALUES (?, ?, ?, ?)", accountID, mode, true, false); err != nil {
			t.Fatal(err)
		}
		if mode == "api-key" {
			if _, err := db.Exec("UPDATE accounts SET api_token = 'sk-relay-test' WHERE id = ?", accountID); err != nil {
				t.Fatal(err)
			}
		}
		if mode == "oauth" {
			if _, err := db.Exec("UPDATE accounts SET oauth_provider = 'codex' WHERE id = ?", accountID); err != nil {
				t.Fatal(err)
			}
		}
	}
	stats, err := RebuildTokenRoutesFromAvailabilityWithOptions(context.Background(), db.DB, RebuildOptions{CreateModelRoutes: true})
	if err != nil || stats.RoutesCreated != 2 || stats.ChannelsInserted != 2 {
		t.Fatalf("credential capabilities not respected: %+v %v", stats, err)
	}
}

func TestAutomaticModelRoutesConcurrentRebuildDoesNotDuplicate(t *testing.T) {
	db := setupRouteRebuildDB(t)
	_, _, tokenID := seedSiteAccountToken(t, db, "concurrent")
	seedRouteCreationModel(t, db, tokenID, "gpt-4o")
	var wg sync.WaitGroup
	errors := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := RebuildTokenRoutesFromAvailabilityWithOptions(context.Background(), db.DB, RebuildOptions{CreateModelRoutes: true})
			errors <- err
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	var routes int
	if err := db.Get(&routes, "SELECT COUNT(*) FROM token_routes"); err != nil {
		t.Fatal(err)
	}
	if routes != 1 {
		t.Fatalf("concurrent duplicate routes: %d", routes)
	}
}

func TestRebuildDoesNotRecycleStaleAutomaticChannelsBetweenExactRoutes(t *testing.T) {
	db := setupRouteRebuildDB(t)
	_, accountID, tokenID := seedSiteAccountToken(t, db, "no-channel-cycle")
	seedRouteCreationModel(t, db, tokenID, "gpt-4o")
	for i := 0; i < 2; i++ {
		if _, err := db.Exec("INSERT INTO token_routes (model_pattern, route_mode, enabled) VALUES ('gpt-4o', 'pattern', ?)", true); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := RebuildTokenRoutesFromAvailabilityWithOptions(context.Background(), db.DB, RebuildOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE token_model_availability SET available = ? WHERE token_id = ?", false, tokenID); err != nil {
		t.Fatal(err)
	}
	if _, err := RebuildTokenRoutesFromAvailabilityWithOptions(context.Background(), db.DB, RebuildOptions{}); err != nil {
		t.Fatal(err)
	}
	var remaining int
	if err := db.Get(&remaining, "SELECT COUNT(*) FROM route_channels"); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatalf("automatic outputs recycled as discovery inputs: %d stale channels", remaining)
	}
	var firstID int64
	if err := db.Get(&firstID, "SELECT MIN(id) FROM token_routes"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO route_channels (route_id, account_id, token_id, source_model, manual_override, enabled) VALUES (?, ?, ?, 'gpt-4o', ?, ?)", firstID, accountID, tokenID, true, true); err != nil {
		t.Fatal(err)
	}
	if _, err := RebuildTokenRoutesFromAvailabilityWithOptions(context.Background(), db.DB, RebuildOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Get(&remaining, "SELECT COUNT(*) FROM route_channels"); err != nil {
		t.Fatal(err)
	}
	if remaining != 2 {
		t.Fatalf("manual source no longer inherited: %d", remaining)
	}
	if _, err := db.Exec("DELETE FROM route_channels WHERE manual_override = ?", true); err != nil {
		t.Fatal(err)
	}
	if _, err := RebuildTokenRoutesFromAvailabilityWithOptions(context.Background(), db.DB, RebuildOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Get(&remaining, "SELECT COUNT(*) FROM route_channels"); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatalf("removed manual source resurrected from an automatic copy: %d", remaining)
	}
}
