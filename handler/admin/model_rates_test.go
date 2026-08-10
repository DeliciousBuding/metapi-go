package admin

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/deliciousbuding/metapi-go/store"
)

// Read-only rate overview

func TestModelRates_AggregatesAllSurfaces(t *testing.T) {
	db, _ := setupStatsSQLiteTest(t)
	now := time.Now().UTC().Format(time.RFC3339)

	res, err := db.Exec(`INSERT INTO sites (name, url, platform, status, global_weight, created_at, updated_at)
		VALUES (?, ?, ?, 'active', ?, ?, ?)`, "RateSite", "https://rate.example.test", "openai", 2.5, now, now)
	if err != nil {
		t.Fatalf("insert site: %v", err)
	}
	siteID, _ := res.LastInsertId()

	res, err = db.Exec(`INSERT INTO accounts (site_id, username, access_token, api_token, status, unit_cost, checkin_enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'active', 0.0042, 0, ?, ?)`, siteID, "rate-user", "sess", "sk-rate", now, now)
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}
	accountID, _ := res.LastInsertId()

	res, err = db.Exec(`INSERT INTO token_routes (model_pattern, display_name, route_mode, routing_strategy, enabled, created_at, updated_at)
		VALUES (?, ?, 'standard', 'weighted', 1, ?, ?)`, "gpt-4o", "Rate Route", now, now)
	if err != nil {
		t.Fatalf("insert route: %v", err)
	}
	routeID, _ := res.LastInsertId()

	if _, err := db.Exec(`INSERT INTO route_channels (route_id, account_id, source_model, priority, weight, enabled)
		VALUES (?, ?, ?, 10, 30, TRUE)`, routeID, accountID, "gpt-4o"); err != nil {
		t.Fatalf("insert channel: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO downstream_api_keys (name, key, key_weight)
		VALUES (?, ?, ?)`, "rate-key", "sk-dsk", 1.7); err != nil {
		t.Fatalf("insert key: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO model_day_usage (local_day, site_id, model, total_calls, total_tokens, total_spend)
		VALUES (?, ?, ?, 100, 5000, 0.21)`, now[:10], siteID, "gpt-4o"); err != nil {
		t.Fatalf("insert usage: %v", err)
	}

	r := chi.NewRouter()
	RegisterModelRatesRoutes(r, db.DB)

	resp := doGet(t, r, "/api/models/rates")
	if resp.Code != 200 {
		t.Fatalf("rates returned %d: %s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	summary := body["summary"].(map[string]any)
	if summary["accountsWithUnitCost"].(float64) != 1 || summary["channelsTotal"].(float64) != 1 {
		t.Fatalf("summary = %#v, want 1 account w/ unit cost + 1 channel", summary)
	}

	accounts := body["accounts"].([]any)
	if len(accounts) != 1 {
		t.Fatalf("accounts = %#v, want 1", accounts)
	}
	acc := accounts[0].(map[string]any)
	if acc["unitCost"].(float64) != 0.0042 || acc["totalWeight"].(float64) != 30 {
		t.Fatalf("account = %#v, want unitCost 0.0042 + totalWeight 30", acc)
	}

	channels := body["channels"].([]any)
	if len(channels) != 1 || channels[0].(map[string]any)["weight"].(float64) != 30 {
		t.Fatalf("channels = %#v, want weight 30", channels)
	}

	sites := body["sites"].([]any)
	if len(sites) != 1 || sites[0].(map[string]any)["globalWeight"].(float64) != 2.5 {
		t.Fatalf("sites = %#v, want globalWeight 2.5", sites)
	}

	keys := body["keys"].([]any)
	if len(keys) != 1 || keys[0].(map[string]any)["keyWeight"].(float64) != 1.7 {
		t.Fatalf("keys = %#v, want keyWeight 1.7", keys)
	}

	models := body["models"].([]any)
	if len(models) != 1 {
		t.Fatalf("models = %#v, want 1", models)
	}
	m := models[0].(map[string]any)
	if m["model"] != "gpt-4o" || m["spend"].(float64) != 0.21 {
		t.Fatalf("model = %#v, want gpt-4o + spend 0.21", m)
	}
}

// Batch rate editing (unit_cost + weight)

// seedRateFixture inserts one site + account + route + channel and returns IDs.
func seedRateFixture(t *testing.T, db *store.DB) (siteID, accountID, channelID int64) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := db.Exec(`INSERT INTO sites (name, url, platform, status, global_weight, created_at, updated_at)
		VALUES (?, ?, ?, 'active', ?, ?, ?)`, "RateEditSite", "https://rate-edit.example.test", "openai", 2.5, now, now)
	if err != nil {
		t.Fatalf("insert site: %v", err)
	}
	if err := db.Get(&siteID, "SELECT id FROM sites WHERE name = ?", "RateEditSite"); err != nil {
		t.Fatalf("site id: %v", err)
	}

	_, err = db.Exec(`INSERT INTO accounts (site_id, username, access_token, api_token, status, unit_cost, checkin_enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'active', 0.0042, FALSE, ?, ?)`, siteID, "rate-edit-user", "sess", "sk-edit", now, now)
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}
	if err := db.Get(&accountID, "SELECT id FROM accounts WHERE username = ?", "rate-edit-user"); err != nil {
		t.Fatalf("account id: %v", err)
	}

	_, err = db.Exec(`INSERT INTO token_routes (model_pattern, display_name, route_mode, routing_strategy, enabled, created_at, updated_at)
		VALUES (?, ?, 'standard', 'weighted', TRUE, ?, ?)`, "gpt-4o", "Rate Edit Route", now, now)
	if err != nil {
		t.Fatalf("insert route: %v", err)
	}
	var routeID int64
	if err := db.Get(&routeID, "SELECT id FROM token_routes WHERE model_pattern = ?", "gpt-4o"); err != nil {
		t.Fatalf("route id: %v", err)
	}

	_, err = db.Exec(`INSERT INTO route_channels (route_id, account_id, source_model, priority, weight, enabled)
		VALUES (?, ?, ?, 10, 30, TRUE)`, routeID, accountID, "gpt-4o")
	if err != nil {
		t.Fatalf("insert channel: %v", err)
	}
	if err := db.Get(&channelID, "SELECT id FROM route_channels WHERE route_id = ?", routeID); err != nil {
		t.Fatalf("channel id: %v", err)
	}
	return siteID, accountID, channelID
}

func TestModelRates_UpdateBatchPersists(t *testing.T) {
	db, _ := setupStatsSQLiteTest(t)
	_, accountID, channelID := seedRateFixture(t, db)

	r := chi.NewRouter()
	RegisterModelRatesRoutes(r, db.DB)

	resp := doPutJSON(t, r, "/api/models/rates", map[string]any{
		"accounts": []map[string]any{{"id": accountID, "unitCost": 0.009}},
		"channels": []map[string]any{{"id": channelID, "weight": 12}},
	})
	if resp.Code != 200 {
		t.Fatalf("update rates returned %d: %s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["success"] != true {
		t.Fatalf("success = %v, want true", body["success"])
	}
	if int(body["updatedAccounts"].(float64)) != 1 || int(body["updatedChannels"].(float64)) != 1 {
		t.Fatalf("counts = %#v, want 1 account + 1 channel", body)
	}

	// Persisted to DB.
	var unitCost float64
	if err := db.Get(&unitCost, "SELECT unit_cost FROM accounts WHERE id = ?", accountID); err != nil {
		t.Fatalf("read unit_cost: %v", err)
	}
	if unitCost != 0.009 {
		t.Fatalf("unit_cost = %v, want 0.009", unitCost)
	}
	var weight float64
	if err := db.Get(&weight, "SELECT weight FROM route_channels WHERE id = ?", channelID); err != nil {
		t.Fatalf("read weight: %v", err)
	}
	if weight != 12 {
		t.Fatalf("weight = %v, want 12", weight)
	}

	// GET reflects the new values.
	resp = doGet(t, r, "/api/models/rates")
	if resp.Code != 200 {
		t.Fatalf("rates returned %d: %s", resp.Code, resp.Body.String())
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	acc := body["accounts"].([]any)[0].(map[string]any)
	if acc["unitCost"].(float64) != 0.009 || acc["totalWeight"].(float64) != 12 {
		t.Fatalf("account = %#v, want unitCost 0.009 + totalWeight 12", acc)
	}
	ch := body["channels"].([]any)[0].(map[string]any)
	if ch["weight"].(float64) != 12 {
		t.Fatalf("channel = %#v, want weight 12", ch)
	}
}

func TestModelRates_UpdateValidation(t *testing.T) {
	db, _ := setupStatsSQLiteTest(t)
	_, accountID, channelID := seedRateFixture(t, db)

	r := chi.NewRouter()
	RegisterModelRatesRoutes(r, db.DB)

	// Negative unitCost → 400.
	resp := doPutJSON(t, r, "/api/models/rates", map[string]any{
		"accounts": []map[string]any{{"id": accountID, "unitCost": -1}},
	})
	if resp.Code != 400 {
		t.Fatalf("negative unitCost status = %d, want 400", resp.Code)
	}

	// Negative weight → 400.
	resp = doPutJSON(t, r, "/api/models/rates", map[string]any{
		"channels": []map[string]any{{"id": channelID, "weight": -3}},
	})
	if resp.Code != 400 {
		t.Fatalf("negative weight status = %d, want 400", resp.Code)
	}

	// Empty arrays → 400.
	resp = doPutJSON(t, r, "/api/models/rates", map[string]any{
		"accounts": []map[string]any{},
		"channels": []map[string]any{},
	})
	if resp.Code != 400 {
		t.Fatalf("empty update status = %d, want 400", resp.Code)
	}

	// Zero and nil values still work: unitCost 0 clears the field, weight 0 disables weighted preference.
	resp = doPutJSON(t, r, "/api/models/rates", map[string]any{
		"accounts": []map[string]any{{"id": accountID, "unitCost": 0}},
		"channels": []map[string]any{{"id": channelID, "weight": 0}},
	})
	if resp.Code != 200 {
		t.Fatalf("zero-value update returned %d: %s", resp.Code, resp.Body.String())
	}
	var unitCost float64
	if err := db.Get(&unitCost, "SELECT unit_cost FROM accounts WHERE id = ?", accountID); err != nil {
		t.Fatalf("read unit_cost: %v", err)
	}
	if unitCost != 0 {
		t.Fatalf("unit_cost = %v, want 0", unitCost)
	}
}

// PostgreSQL dialect parity for the batch update (skipped without PG_TEST_DSN).
func TestModelRates_UpdateBatchPostgres(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PG_TEST_DSN"))
	if dsn == "" {
		t.Skip("PG_TEST_DSN not set; skipping PostgreSQL integration test")
	}
	db, _ := setupStatsPostgresTest(t)
	_, accountID, channelID := seedRateFixture(t, db)

	r := chi.NewRouter()
	RegisterModelRatesRoutes(r, db.DB)

	resp := doPutJSON(t, r, "/api/models/rates", map[string]any{
		"accounts": []map[string]any{{"id": accountID, "unitCost": 0.002}},
		"channels": []map[string]any{{"id": channelID, "weight": 42}},
	})
	if resp.Code != 200 {
		t.Fatalf("update rates returned %d: %s", resp.Code, resp.Body.String())
	}
	var weight float64
	if err := db.Get(&weight, "SELECT weight FROM route_channels WHERE id = ?", channelID); err != nil {
		t.Fatalf("read weight: %v", err)
	}
	if weight != 42 {
		t.Fatalf("weight = %v, want 42", weight)
	}
}
