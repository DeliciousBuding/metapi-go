package settingsmigration

import (
	"encoding/json"
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

func setupMigrationDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	return db
}

func setSetting(t *testing.T, db *store.DB, key, value string) {
	t.Helper()
	if err := store.NewSettingsStore(db).Set(key, value); err != nil {
		t.Fatalf("set %s: %v", key, err)
	}
}

func getSetting(t *testing.T, db *store.DB, key string) string {
	t.Helper()
	v, err := store.NewSettingsStore(db).Get(key)
	if err != nil {
		t.Fatalf("get %s: %v", key, err)
	}
	return v
}

func seedLegacySchedules(t *testing.T, db *store.DB) {
	t.Helper()
	setSetting(t, db, "checkin_schedule_mode", `"cron"`)
	setSetting(t, db, "checkin_cron", `"0 8 * * *"`)
	setSetting(t, db, "checkin_interval_hours", `6`)
	setSetting(t, db, "balance_refresh_cron", `"0 * * * *"`)
	setSetting(t, db, "log_cleanup_cron", `"0 6 * * *"`)
}

func TestBuildPlanEmptyDB(t *testing.T) {
	db := setupMigrationDB(t)
	plan, err := BuildPlan(db.DB)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if plan.CurrentVersion != 0 || plan.Pending != 0 || plan.CustomCount != 0 || !plan.LegacyFieldsPreserved {
		t.Fatalf("empty plan = %+v", plan)
	}
	if plan.TargetVersion != CurrentSchemaVersion {
		t.Fatalf("target = %d, want %d", plan.TargetVersion, CurrentSchemaVersion)
	}
}

func TestBuildPlanAndApply(t *testing.T) {
	db := setupMigrationDB(t)
	seedLegacySchedules(t, db)

	plan, err := BuildPlan(db.DB)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if plan.Pending != 3 {
		t.Fatalf("pending = %d, want 3", plan.Pending)
	}
	if plan.CustomCount != 0 {
		t.Fatalf("customCount = %d, want 0", plan.CustomCount)
	}
	if plan.CurrentVersion != 0 || plan.TargetVersion != CurrentSchemaVersion {
		t.Fatalf("versions = (%d, %d)", plan.CurrentVersion, plan.TargetVersion)
	}

	// Legacy bytes must be untouched before apply.
	before := map[string]string{
		"checkin_cron":         getSetting(t, db, "checkin_cron"),
		"balance_refresh_cron": getSetting(t, db, "balance_refresh_cron"),
		"log_cleanup_cron":     getSetting(t, db, "log_cleanup_cron"),
	}

	result, err := Apply(db.DB)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if result.Applied != 4 {
		t.Fatalf("applied = %d, want 4 (3 schedules + schema version)", result.Applied)
	}
	if result.Pending != 0 {
		t.Fatalf("post-apply pending = %d, want 0", result.Pending)
	}

	// v2 keys written with JSON-encoded ScheduleSpec v1.
	checkinV2 := getSetting(t, db, checkinScheduleV2Key)
	var spec map[string]any
	if err := json.Unmarshal([]byte(checkinV2), &spec); err != nil {
		t.Fatalf("checkin_schedule_v2 not valid JSON: %v", err)
	}
	if spec["version"] != float64(1) || spec["kind"] != "daily" {
		t.Fatalf("checkin_schedule_v2 = %v", spec)
	}
	if spec["cron"] != "0 8 * * *" {
		t.Fatalf("checkin_schedule_v2 cron = %v, want original bytes preserved", spec["cron"])
	}

	if v := getSetting(t, db, SchemaVersionKey); v != "1" {
		t.Fatalf("settings_schema_version = %q, want 1", v)
	}

	// Legacy values unchanged.
	if getSetting(t, db, "checkin_cron") != before["checkin_cron"] {
		t.Fatal("legacy checkin_cron modified by migration")
	}
	if getSetting(t, db, "balance_refresh_cron") != before["balance_refresh_cron"] {
		t.Fatal("legacy balance_refresh_cron modified by migration")
	}
	if getSetting(t, db, "log_cleanup_cron") != before["log_cleanup_cron"] {
		t.Fatal("legacy log_cleanup_cron modified by migration")
	}

	// Second apply is a no-op.
	result2, err := Apply(db.DB)
	if err != nil {
		t.Fatalf("second Apply: %v", err)
	}
	if result2.Applied != 0 || result2.Pending != 0 {
		t.Fatalf("second apply = %+v, want no-op", result2)
	}
	if getSetting(t, db, checkinScheduleV2Key) != checkinV2 {
		t.Fatal("second apply overwrote existing v2 key")
	}
}

func TestApplyCustomCronCounted(t *testing.T) {
	db := setupMigrationDB(t)
	setSetting(t, db, "checkin_cron", `"0 0 * * 1-5"`)
	setSetting(t, db, "balance_refresh_cron", `"0 0 * * 1-5"`)

	plan, err := BuildPlan(db.DB)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if plan.CustomCount != 2 {
		t.Fatalf("customCount = %d, want 2", plan.CustomCount)
	}
	for _, it := range plan.Items {
		if it.Schedule.Type != "custom" {
			t.Fatalf("item %s type = %q, want custom", it.Task, it.Schedule.Type)
		}
	}
}

func TestApplyRollbackOnInjectedFailure(t *testing.T) {
	db := setupMigrationDB(t)
	seedLegacySchedules(t, db)

	old := failAfterWrites
	failAfterWrites = 1
	t.Cleanup(func() { failAfterWrites = old })

	if _, err := Apply(db.DB); err == nil {
		t.Fatal("Apply with injected failure returned nil error")
	}

	// Nothing may be persisted after rollback.
	for _, key := range []string{checkinScheduleV2Key, balanceRefreshScheduleV2Key, logCleanupScheduleV2Key, SchemaVersionKey} {
		if v := getSetting(t, db, key); v != "" {
			t.Fatalf("key %s persisted after rollback: %q", key, v)
		}
	}

	// Re-run succeeds and completes all writes (after clearing the hook).
	failAfterWrites = -1
	result, err := Apply(db.DB)
	if err != nil {
		t.Fatalf("Apply after rollback: %v", err)
	}
	if result.Applied != 4 {
		t.Fatalf("applied = %d, want 4", result.Applied)
	}
}

func TestApplyDoesNotOverwriteExistingV2Key(t *testing.T) {
	db := setupMigrationDB(t)
	seedLegacySchedules(t, db)
	setSetting(t, db, checkinScheduleV2Key, `{"version":1,"kind":"custom","cron":"0 0 * * 1-5"}`)

	result, err := Apply(db.DB)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if result.Applied != 3 {
		t.Fatalf("applied = %d, want 3 (checkin v2 already present)", result.Applied)
	}
	v := getSetting(t, db, checkinScheduleV2Key)
	if v != `{"version":1,"kind":"custom","cron":"0 0 * * 1-5"}` {
		t.Fatalf("existing v2 key overwritten: %q", v)
	}
}

func TestBuildPlanWindowCheckin(t *testing.T) {
	db := setupMigrationDB(t)
	setSetting(t, db, "checkin_schedule_mode", `"window"`)
	setSetting(t, db, "checkin_window_start", `"02:00"`)
	setSetting(t, db, "checkin_window_end", `"03:30"`)

	plan, err := BuildPlan(db.DB)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if len(plan.Items) != 1 || plan.Items[0].Schedule.Type != "window" {
		t.Fatalf("items = %+v, want single window item", plan.Items)
	}
	if plan.Items[0].Schedule.WindowStart != "02:00" || plan.Items[0].Schedule.WindowEnd != "03:30" {
		t.Fatalf("window bounds = (%q, %q)", plan.Items[0].Schedule.WindowStart, plan.Items[0].Schedule.WindowEnd)
	}
}
