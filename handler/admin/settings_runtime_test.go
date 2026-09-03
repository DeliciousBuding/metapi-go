package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
)

func TestSettingsRuntimeUpdateCheckinSchedulePersistsSettings(t *testing.T) {
	db, r, _ := setupEdgeTest(t)

	resp := doPutJSON(t, r, "/api/settings/runtime", map[string]any{
		"checkinScheduleMode":  "interval",
		"checkinCron":          "15 9 * * *",
		"checkinIntervalHours": 6,
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("update runtime: %d %s", resp.Code, resp.Body.String())
	}
	if config.Runtime().CheckinScheduleMode != "interval" || config.Runtime().CheckinCron != "15 9 * * *" || config.Runtime().CheckinIntervalHours != 6 {
		t.Fatalf("cfg schedule = (%q, %q, %d), want (interval, 15 9 * * *, 6)",
			config.Runtime().CheckinScheduleMode, config.Runtime().CheckinCron, config.Runtime().CheckinIntervalHours)
	}

	settings := map[string]string{}
	rows, err := db.Query("SELECT key, value FROM settings WHERE key IN (?, ?, ?)",
		"checkin_schedule_mode", "checkin_cron", "checkin_interval_hours")
	if err != nil {
		t.Fatalf("query settings: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			t.Fatalf("scan setting: %v", err)
		}
		settings[key] = value
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("settings rows: %v", err)
	}
	if settings["checkin_schedule_mode"] != `"interval"` {
		t.Fatalf("stored mode = %q, want JSON string interval", settings["checkin_schedule_mode"])
	}
	if settings["checkin_cron"] != `"15 9 * * *"` {
		t.Fatalf("stored cron = %q, want JSON string cron", settings["checkin_cron"])
	}
	if settings["checkin_interval_hours"] != `6` {
		t.Fatalf("stored interval = %q, want JSON number 6", settings["checkin_interval_hours"])
	}
}

func TestSettingsRuntimeUpdateCheckinScheduleRejectsInvalidCron(t *testing.T) {
	_, r, _ := setupEdgeTest(t)

	resp := doPutJSON(t, r, "/api/settings/runtime", map[string]any{
		"checkinCron": "bad cron",
	})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s, want 400", resp.Code, resp.Body.String())
	}
}

func TestSettingsRuntimeUpdateCheckinScheduleRejectsFractionalInterval(t *testing.T) {
	_, r, _ := setupEdgeTest(t)

	resp := doPutJSON(t, r, "/api/settings/runtime", map[string]any{
		"checkinIntervalHours": 1.5,
	})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s, want 400", resp.Code, resp.Body.String())
	}
}

func TestSettingsRuntimeGlobalAllowedModelsPersistsAndNormalizes(t *testing.T) {
	db, r, _ := setupEdgeTest(t)

	resp := doPutJSON(t, r, "/api/settings/runtime", map[string]any{
		"globalAllowedModels": []any{"gpt-4o", " claude-3.7-sonnet ", "gpt-4o", "", "gemini-pro"},
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("update runtime: %d %s", resp.Code, resp.Body.String())
	}

	want := []string{"gpt-4o", "claude-3.7-sonnet", "gemini-pro"}
	if len(config.Runtime().GlobalAllowedModels) != len(want) {
		t.Fatalf("config.Runtime().GlobalAllowedModels = %#v, want %#v", config.Runtime().GlobalAllowedModels, want)
	}
	for i := range want {
		if config.Runtime().GlobalAllowedModels[i] != want[i] {
			t.Fatalf("config.Runtime().GlobalAllowedModels = %#v, want %#v", config.Runtime().GlobalAllowedModels, want)
		}
	}

	var stored string
	if err := db.Get(&stored, "SELECT value FROM settings WHERE key = ?", "global_allowed_models"); err != nil {
		t.Fatalf("query stored whitelist: %v", err)
	}
	if stored != `["gpt-4o","claude-3.7-sonnet","gemini-pro"]` {
		t.Fatalf("stored whitelist = %q, want normalized JSON array", stored)
	}

	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	models, ok := body["globalAllowedModels"].([]any)
	if !ok {
		t.Fatalf("response globalAllowedModels type = %T, want []any", body["globalAllowedModels"])
	}
	if len(models) != len(want) {
		t.Fatalf("response globalAllowedModels = %#v, want %#v", models, want)
	}
}

func TestSettingsRuntimeGlobalAllowedModelsExplicitEmptyClears(t *testing.T) {
	db, r, _ := setupEdgeTest(t)

	if resp := doPutJSON(t, r, "/api/settings/runtime", map[string]any{
		"globalAllowedModels": []any{"keep-me", "also-keep"},
	}); resp.Code != http.StatusOK {
		t.Fatalf("seed whitelist: %d %s", resp.Code, resp.Body.String())
	}

	resp := doPutJSON(t, r, "/api/settings/runtime", map[string]any{
		"globalAllowedModels": []any{},
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("clear whitelist: %d %s", resp.Code, resp.Body.String())
	}
	if len(config.Runtime().GlobalAllowedModels) != 0 {
		t.Fatalf("config.Runtime().GlobalAllowedModels = %#v, want empty after explicit clear", config.Runtime().GlobalAllowedModels)
	}
	var stored string
	if err := db.Get(&stored, "SELECT value FROM settings WHERE key = ?", "global_allowed_models"); err != nil {
		t.Fatalf("query stored whitelist: %v", err)
	}
	if stored != `[]` {
		t.Fatalf("stored whitelist = %q, want []", stored)
	}
}

func TestSettingsRuntimeGlobalAllowedModelsRejectsNullAndNonArray(t *testing.T) {
	db, r, _ := setupEdgeTest(t)
	config.UpdateRuntime(func(r *config.RuntimeSettings) { r.GlobalAllowedModels = []string{"must-survive"} })
	if _, err := db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?)`,
		"global_allowed_models", `["must-survive"]`); err != nil {
		t.Fatalf("seed db: %v", err)
	}

	for _, body := range []map[string]any{
		{"globalAllowedModels": nil},
		{"globalAllowedModels": "gpt-4o"},
		{"globalAllowedModels": map[string]any{"model": "gpt-4o"}},
		{"globalAllowedModels": []any{"ok", 1}},
	} {
		resp := doPutJSON(t, r, "/api/settings/runtime", body)
		if resp.Code != http.StatusBadRequest {
			t.Fatalf("body=%v status=%d, want 400; resp=%s", body, resp.Code, resp.Body.String())
		}
	}

	if len(config.Runtime().GlobalAllowedModels) != 1 || config.Runtime().GlobalAllowedModels[0] != "must-survive" {
		t.Fatalf("config.Runtime().GlobalAllowedModels clobbered to %#v", config.Runtime().GlobalAllowedModels)
	}
	var stored string
	if err := db.Get(&stored, "SELECT value FROM settings WHERE key = ?", "global_allowed_models"); err != nil {
		t.Fatalf("query stored whitelist: %v", err)
	}
	if stored != `["must-survive"]` {
		t.Fatalf("stored whitelist wiped to %q", stored)
	}
}

func TestSettingsRuntimePartialUpdateDoesNotClobberWhitelist(t *testing.T) {
	db, r, _ := setupEdgeTest(t)
	config.UpdateRuntime(func(r *config.RuntimeSettings) { r.GlobalAllowedModels = []string{"alpha", "beta"} })
	if _, err := db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?)`,
		"global_allowed_models", `["alpha","beta"]`); err != nil {
		t.Fatalf("seed db: %v", err)
	}

	resp := doPutJSON(t, r, "/api/settings/runtime", map[string]any{
		"proxyEmptyContentFailEnabled": true,
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("partial update: %d %s", resp.Code, resp.Body.String())
	}
	if len(config.Runtime().GlobalAllowedModels) != 2 || config.Runtime().GlobalAllowedModels[0] != "alpha" || config.Runtime().GlobalAllowedModels[1] != "beta" {
		t.Fatalf("whitelist clobbered by partial update: %#v", config.Runtime().GlobalAllowedModels)
	}
	var stored string
	if err := db.Get(&stored, "SELECT value FROM settings WHERE key = ?", "global_allowed_models"); err != nil {
		t.Fatalf("query stored whitelist: %v", err)
	}
	if stored != `["alpha","beta"]` {
		t.Fatalf("stored whitelist changed by partial update: %q", stored)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/settings/runtime", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET runtime: %d %s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode GET: %v", err)
	}
	models, ok := got["globalAllowedModels"].([]any)
	if !ok || len(models) != 2 {
		t.Fatalf("GET globalAllowedModels = %#v", got["globalAllowedModels"])
	}
}

func TestSettingsRuntimeUpdateBalanceCronRejectsInvalid(t *testing.T) {
	_, r, _ := setupEdgeTest(t)
	resp := doPutJSON(t, r, "/api/settings/runtime", map[string]any{
		"balanceRefreshCron": "bad cron",
	})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s, want 400", resp.Code, resp.Body.String())
	}
}

func TestSettingsRuntimeUpdateBalanceCronDualWrite(t *testing.T) {
	db, r, _ := setupEdgeTest(t)
	resp := doPutJSON(t, r, "/api/settings/runtime", map[string]any{
		"balanceRefreshCron": "0 9 * * *",
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", resp.Code, resp.Body.String())
	}
	if config.Runtime().BalanceRefreshCron != "0 9 * * *" {
		t.Fatalf("config.Runtime().BalanceRefreshCron = %q", config.Runtime().BalanceRefreshCron)
	}
	var legacy, v2 string
	if err := db.Get(&legacy, "SELECT value FROM settings WHERE key = ?", "balance_refresh_cron"); err != nil {
		t.Fatalf("read legacy: %v", err)
	}
	if err := db.Get(&v2, "SELECT value FROM settings WHERE key = ?", "balance_refresh_schedule_v2"); err != nil {
		t.Fatalf("read v2: %v", err)
	}
	if legacy != `"0 9 * * *"` {
		t.Fatalf("legacy = %q", legacy)
	}
	var spec map[string]any
	if err := json.Unmarshal([]byte(v2), &spec); err != nil {
		t.Fatalf("v2 not JSON: %v", err)
	}
	if spec["version"] != float64(1) || spec["kind"] != "daily" || spec["cron"] != "0 9 * * *" {
		t.Fatalf("v2 spec = %v", spec)
	}
}

func TestSettingsRuntimeUpdateModelSyncCronRejectsInvalid(t *testing.T) {
	_, r, _ := setupEdgeTest(t)
	resp := doPutJSON(t, r, "/api/settings/runtime", map[string]any{
		"modelSyncCron": "bad cron",
	})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s, want 400", resp.Code, resp.Body.String())
	}
}

// TestSettingsRuntimeUpdateModelSyncCronPersistsPlain covers the #1005 model
// sync cron: plain persistence (no v2 dual schedule mirror), config update,
// and GET round-trip.
func TestSettingsRuntimeUpdateModelSyncCronPersistsPlain(t *testing.T) {
	db, r, _ := setupEdgeTest(t)
	resp := doPutJSON(t, r, "/api/settings/runtime", map[string]any{
		"modelSyncCron": "0 5 * * 1",
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", resp.Code, resp.Body.String())
	}
	if config.Runtime().ModelSyncCron != "0 5 * * 1" {
		t.Fatalf("config.Runtime().ModelSyncCron = %q", config.Runtime().ModelSyncCron)
	}
	var stored string
	if err := db.Get(&stored, "SELECT value FROM settings WHERE key = ?", "model_sync_cron"); err != nil {
		t.Fatalf("read model_sync_cron: %v", err)
	}
	if stored != `"0 5 * * 1"` {
		t.Fatalf("stored = %q", stored)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/settings/runtime", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET runtime: %d %s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode GET: %v", err)
	}
	if got["modelSyncCron"] != "0 5 * * 1" {
		t.Fatalf("GET modelSyncCron = %v", got["modelSyncCron"])
	}
}

func TestSettingsRuntimeUpdateLogCleanupCronRejectsInvalid(t *testing.T) {
	_, r, _ := setupEdgeTest(t)
	resp := doPutJSON(t, r, "/api/settings/runtime", map[string]any{
		"logCleanupCron": "nope",
	})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s, want 400", resp.Code, resp.Body.String())
	}
}

func TestSettingsRuntimeUpdateCheckinScheduleObject(t *testing.T) {
	db, r, _ := setupEdgeTest(t)
	resp := doPutJSON(t, r, "/api/settings/runtime", map[string]any{
		"checkinSchedule": map[string]any{
			"version": 1,
			"kind":    "daily",
			"time":    "07:30",
			"cron":    "30 7 * * *",
		},
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", resp.Code, resp.Body.String())
	}
	if config.Runtime().CheckinScheduleMode != "cron" || config.Runtime().CheckinCron != "30 7 * * *" {
		t.Fatalf("checkin cfg = (%q, %q)", config.Runtime().CheckinScheduleMode, config.Runtime().CheckinCron)
	}
	var v2 string
	if err := db.Get(&v2, "SELECT value FROM settings WHERE key = ?", "checkin_schedule_v2"); err != nil {
		t.Fatalf("read v2: %v", err)
	}
	var spec map[string]any
	if err := json.Unmarshal([]byte(v2), &spec); err != nil {
		t.Fatalf("v2 not JSON: %v", err)
	}
	if spec["kind"] != "daily" || spec["cron"] != "30 7 * * *" {
		t.Fatalf("v2 spec = %v", spec)
	}
}

func TestSettingsRuntimeUpdateCheckinScheduleObjectRejectsBadVersion(t *testing.T) {
	_, r, _ := setupEdgeTest(t)
	resp := doPutJSON(t, r, "/api/settings/runtime", map[string]any{
		"checkinSchedule": map[string]any{
			"version": 99,
			"kind":    "daily",
			"time":    "07:30",
		},
	})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s, want 400", resp.Code, resp.Body.String())
	}
}

func TestSettingsRuntimeUpdateBrandingPersists(t *testing.T) {
	db, r, _ := setupEdgeTest(t)
	resp := doPutJSON(t, r, "/api/settings/runtime", map[string]any{
		"systemName":    "My Gateway",
		"logo":          "https://example.com/logo.png",
		"footer":        "Powered by Metapi",
		"about":         "About copy",
		"serverAddress": "https://gw.example.com",
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", resp.Code, resp.Body.String())
	}
	if config.Runtime().SystemName != "My Gateway" || config.Runtime().Logo != "https://example.com/logo.png" || config.Runtime().ServerAddress != "https://gw.example.com" {
		t.Fatalf("runtime branding = %+v", config.Runtime())
	}
	var stored string
	if err := db.Get(&stored, "SELECT value FROM settings WHERE key = ?", "system_name"); err != nil {
		t.Fatalf("read system_name: %v", err)
	}
	if stored != `"My Gateway"` {
		t.Fatalf("stored system_name = %q", stored)
	}
}

func TestSettingsRuntimeGetIncludesWindowAndScheduleFields(t *testing.T) {
	_, r, _ := setupEdgeTest(t)
	config.UpdateRuntime(func(r *config.RuntimeSettings) { r.CheckinWindowStart = "02:00" })
	config.UpdateRuntime(func(r *config.RuntimeSettings) { r.CheckinWindowEnd = "03:30" })
	req := httptest.NewRequest("GET", "/api/settings/runtime", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode GET: %v", err)
	}
	for _, key := range []string{"checkinWindowStart", "checkinWindowEnd", "checkinSchedule", "balanceRefreshSchedule", "modelSyncCron", "logCleanupSchedule", "systemName", "logo", "footer", "about", "serverAddress"} {
		if _, ok := body[key]; !ok {
			t.Fatalf("GET runtime missing key %s", key)
		}
	}
	if body["checkinWindowStart"] != "02:00" || body["checkinWindowEnd"] != "03:30" {
		t.Fatalf("window fields = (%v, %v)", body["checkinWindowStart"], body["checkinWindowEnd"])
	}
}

func TestSettingsMigrationPreviewAndApply(t *testing.T) {
	db, r, _ := setupEdgeTest(t)
	if _, err := db.Exec("INSERT INTO settings (key, value) VALUES (?, ?), (?, ?), (?, ?)",
		"checkin_cron", `"0 8 * * *"`, "balance_refresh_cron", `"0 * * * *"`, "log_cleanup_cron", `"0 6 * * *"`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	req := httptest.NewRequest("GET", "/api/settings/migration/preview", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview status = %d body=%s", rec.Code, rec.Body.String())
	}
	var prev map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &prev); err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	if prev["pending"] != float64(3) || prev["currentVersion"] != float64(0) || prev["targetVersion"] != float64(1) {
		t.Fatalf("preview = %v", prev)
	}
	if prev["legacyFieldsPreserved"] != true {
		t.Fatalf("legacyFieldsPreserved = %v", prev["legacyFieldsPreserved"])
	}
	req2 := httptest.NewRequest("POST", "/api/settings/migration/apply", nil)
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("apply status = %d body=%s", rec2.Code, rec2.Body.String())
	}
	var res map[string]any
	if err := json.Unmarshal(rec2.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode apply: %v", err)
	}
	if res["applied"] != float64(4) {
		t.Fatalf("applied = %v, want 4", res["applied"])
	}
	var v2 string
	if err := db.Get(&v2, "SELECT value FROM settings WHERE key = ?", "balance_refresh_schedule_v2"); err != nil {
		t.Fatalf("read v2: %v", err)
	}
	if !strings.Contains(v2, `"kind":"interval"`) {
		t.Fatalf("balance v2 = %q", v2)
	}
}

func TestSettingsRuntimeScheduleMirrorUsesFinalCron(t *testing.T) {
	db, r, _ := setupEdgeTest(t)
	resp := doPutJSON(t, r, "/api/settings/runtime", map[string]any{
		"checkinCron": "0 10 * * *",
		"checkinSchedule": map[string]any{
			"version": 1,
			"kind":    "daily",
			"time":    "10:00",
			"cron":    "0 9 * * *",
		},
		"balanceRefreshCron": "0 11 * * *",
		"balanceRefreshSchedule": map[string]any{
			"version": 1,
			"kind":    "daily",
			"time":    "11:00",
			"cron":    "0 8 * * *",
		},
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", resp.Code, resp.Body.String())
	}
	for key, want := range map[string]string{
		"checkin_schedule_v2":         "0 10 * * *",
		"balance_refresh_schedule_v2": "0 11 * * *",
	} {
		var raw string
		if err := db.Get(&raw, "SELECT value FROM settings WHERE key = ?", key); err != nil {
			t.Fatalf("read %s: %v", key, err)
		}
		var spec map[string]any
		if err := json.Unmarshal([]byte(raw), &spec); err != nil {
			t.Fatalf("decode %s: %v", key, err)
		}
		if spec["cron"] != want {
			t.Fatalf("%s cron = %v, want %q", key, spec["cron"], want)
		}
	}
}

func TestSettingsRuntimeNotifyTogglesPersistAsJSONObject(t *testing.T) {
	db, r, _ := setupEdgeTest(t)
	resp := doPutJSON(t, r, "/api/settings/runtime", map[string]any{
		"notifyTaskToggles": map[string]any{
			"token_expired": true,
			"low_balance":   false,
		},
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", resp.Code, resp.Body.String())
	}
	var raw string
	if err := db.Get(&raw, "SELECT value FROM settings WHERE key = ?", "notify_task_toggles"); err != nil {
		t.Fatalf("read notify_task_toggles: %v", err)
	}
	var toggles map[string]bool
	if err := json.Unmarshal([]byte(raw), &toggles); err != nil {
		t.Fatalf("stored value is not a JSON object: %q: %v", raw, err)
	}
	if !toggles["token_expired"] || toggles["low_balance"] {
		t.Fatalf("stored toggles = %#v", toggles)
	}
}

// errorCode contract: the settings-apply validation funnel emits the
// additive machine-readable invalidSettingsValue code on 400-class apply
// failures (the largest single validation family — scout §settings_apply);
// 5xx apply failures intentionally carry no code. The message text stays
// the display fallback.
func TestSettingsRuntimeApplyValidationCarriesCode(t *testing.T) {
	_, r, _ := setupEdgeTest(t)
	resp := doPutJSON(t, r, "/api/settings/runtime", map[string]any{
		"checkinCron": "bad cron",
	})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s, want 400", resp.Code, resp.Body.String())
	}
	var body struct {
		Error     string `json:"error"`
		ErrorCode string `json:"errorCode"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v (body=%s)", err, resp.Body.String())
	}
	if body.Error == "" {
		t.Fatalf("expected human-readable error text, got %q", resp.Body.String())
	}
	if body.ErrorCode != "invalidSettingsValue" {
		t.Fatalf("errorCode = %q, want %q (body=%s)", body.ErrorCode, "invalidSettingsValue", resp.Body.String())
	}
}
