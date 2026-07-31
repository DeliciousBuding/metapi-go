package admin

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

// ---- N9a (New API borrow): read-only rate overview ----

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
		VALUES (?, ?, ?, 10, 30, 1)`, routeID, accountID, "gpt-4o"); err != nil {
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
