package admin

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
)

// Focused tests for the issue #1027 global kill switches exposed via
// PUT /api/settings/runtime (checkinEnabled / balanceRefreshEnabled).

func getRuntimeSettingJSON(t *testing.T, db *store.DB, key string) string {
	t.Helper()
	var value string
	if err := db.QueryRow(db.Rebind("SELECT value FROM settings WHERE key = ?"), key).Scan(&value); err != nil {
		t.Fatalf("query setting %s: %v", key, err)
	}
	return value
}

func TestSettingsRuntimeCheckinEnabledKillSwitch(t *testing.T) {
	db, r, _ := setupEdgeTest(t)

	if config.Runtime().CheckinDisabled {
		t.Fatalf("fixture must start with check-in enabled")
	}

	// Disable.
	resp := doPutJSON(t, r, "/api/settings/runtime", map[string]any{
		"checkinEnabled": false,
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("disable: %d %s", resp.Code, resp.Body.String())
	}
	if !config.Runtime().CheckinDisabled {
		t.Fatalf("PUT checkinEnabled=false must set config.Runtime().CheckinDisabled")
	}
	if got := getRuntimeSettingJSON(t, db, "checkin_enabled"); got != "false" {
		t.Fatalf("stored checkin_enabled = %q, want JSON false", got)
	}

	// GET reflects the toggle.
	get := doGet(t, r, "/api/settings/runtime")
	if get.Code != http.StatusOK {
		t.Fatalf("GET runtime: %d", get.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(get.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode GET body: %v", err)
	}
	if body["checkinEnabled"] != false {
		t.Fatalf("GET checkinEnabled = %v, want false", body["checkinEnabled"])
	}
	if body["balanceRefreshEnabled"] != true {
		t.Fatalf("GET balanceRefreshEnabled = %v, want true (untouched)", body["balanceRefreshEnabled"])
	}

	// Re-enable.
	resp = doPutJSON(t, r, "/api/settings/runtime", map[string]any{
		"checkinEnabled": true,
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("re-enable: %d %s", resp.Code, resp.Body.String())
	}
	if config.Runtime().CheckinDisabled {
		t.Fatalf("PUT checkinEnabled=true must clear config.Runtime().CheckinDisabled")
	}
	if got := getRuntimeSettingJSON(t, db, "checkin_enabled"); got != "true" {
		t.Fatalf("stored checkin_enabled = %q, want JSON true", got)
	}
}

func TestSettingsRuntimeBalanceRefreshEnabledKillSwitch(t *testing.T) {
	db, r, _ := setupEdgeTest(t)

	resp := doPutJSON(t, r, "/api/settings/runtime", map[string]any{
		"balanceRefreshEnabled": false,
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("disable: %d %s", resp.Code, resp.Body.String())
	}
	if !config.Runtime().BalanceRefreshDisabled {
		t.Fatalf("PUT balanceRefreshEnabled=false must set config.Runtime().BalanceRefreshDisabled")
	}
	if got := getRuntimeSettingJSON(t, db, "balance_refresh_enabled"); got != "false" {
		t.Fatalf("stored balance_refresh_enabled = %q, want JSON false", got)
	}
}

func TestSettingsRuntimeKillSwitchRejectsNonBoolean(t *testing.T) {
	_, r, _ := setupEdgeTest(t)

	for _, key := range []string{"checkinEnabled", "balanceRefreshEnabled"} {
		resp := doPutJSON(t, r, "/api/settings/runtime", map[string]any{
			key: "yes",
		})
		if resp.Code != http.StatusBadRequest {
			t.Fatalf("%s non-boolean: status = %d body=%s, want 400", key, resp.Code, resp.Body.String())
		}
	}
}
