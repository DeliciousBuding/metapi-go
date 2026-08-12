package settingsmigration

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/scheduler"
)

// Settings schema version bookkeeping. The settings table keeps the legacy
// *_cron / mode keys as the single runtime source of truth; v2 schedule keys
// are append-only semantic mirrors produced by this migration.
const (
	// SchemaVersionKey records the migrated settings schema version as a
	// JSON-encoded number in the settings table.
	SchemaVersionKey = "settings_schema_version"
	// CurrentSchemaVersion is the target version written by Apply.
	CurrentSchemaVersion = 1

	checkinScheduleV2Key        = "checkin_schedule_v2"
	balanceRefreshScheduleV2Key = "balance_refresh_schedule_v2"
	logCleanupScheduleV2Key     = "log_cleanup_schedule_v2"
)

// MigrationItem describes one legacy -> v2 schedule conversion.
type MigrationItem struct {
	Task        string                 `json:"task"`
	LegacyKey   string                 `json:"legacyKey"`
	LegacyValue string                 `json:"legacyValue"`
	V2Key       string                 `json:"v2Key"`
	Schedule    scheduler.ScheduleSpec `json:"schedule"`
}

// MigrationPlan is the top-level flat preview of a pending migration.
type MigrationPlan struct {
	CurrentVersion        int             `json:"currentVersion"`
	TargetVersion         int             `json:"targetVersion"`
	Pending               int             `json:"pending"`
	CustomCount           int             `json:"customCount"`
	LegacyFieldsPreserved bool            `json:"legacyFieldsPreserved"`
	Items                 []MigrationItem `json:"items"`
}

// MigrationResult extends MigrationPlan with the number of rows actually
// written by Apply.
type MigrationResult struct {
	MigrationPlan
	Applied int `json:"applied"`
}

// failAfterWrites is a test-only hook: when >= 0, Apply fails after that many
// successful writes so callers can prove the whole transaction rolls back.
var failAfterWrites = -1

// BuildPlan computes the append-only migration plan from the current settings
// table. It never writes and never modifies legacy keys.
func BuildPlan(db *sqlx.DB) (MigrationPlan, error) {
	settings, err := readSettings(db)
	if err != nil {
		return MigrationPlan{}, err
	}
	current := currentVersion(settings)
	items := buildItems(settings)
	custom := 0
	for _, it := range items {
		if it.Schedule.Type == "custom" {
			custom++
		}
	}
	return MigrationPlan{
		CurrentVersion:        current,
		TargetVersion:         CurrentSchemaVersion,
		Pending:               len(items),
		CustomCount:           custom,
		LegacyFieldsPreserved: true,
		Items:                 items,
	}, nil
}

// Apply writes the pending v2 schedule keys and the schema version in a single
// transaction. It only appends (never deletes or modifies legacy keys), skips
// already-present v2 keys, and is a no-op when everything is already migrated.
func Apply(db *sqlx.DB) (MigrationResult, error) {
	settings, err := readSettings(db)
	if err != nil {
		return MigrationResult{}, err
	}
	current := currentVersion(settings)
	items := buildItems(settings)

	tx, err := db.Beginx()
	if err != nil {
		return MigrationResult{}, fmt.Errorf("settings migration: begin: %w", err)
	}
	defer tx.Rollback()

	writes := 0
	applied := 0
	for _, it := range items {
		if failAfterWrites >= 0 && writes >= failAfterWrites {
			return MigrationResult{}, fmt.Errorf("settings migration: injected failure after %d writes", writes)
		}
		if err := upsertTx(db, tx, it.V2Key, it.Schedule); err != nil {
			return MigrationResult{}, err
		}
		writes++
		applied++
	}
	if current < CurrentSchemaVersion {
		if failAfterWrites >= 0 && writes >= failAfterWrites {
			return MigrationResult{}, fmt.Errorf("settings migration: injected failure after %d writes", writes)
		}
		if err := upsertTx(db, tx, SchemaVersionKey, CurrentSchemaVersion); err != nil {
			return MigrationResult{}, err
		}
		applied++
	}
	if err := tx.Commit(); err != nil {
		return MigrationResult{}, fmt.Errorf("settings migration: commit: %w", err)
	}

	plan, err := BuildPlan(db)
	if err != nil {
		return MigrationResult{}, err
	}
	return MigrationResult{MigrationPlan: plan, Applied: applied}, nil
}

func readSettings(db *sqlx.DB) (map[string]string, error) {
	rows, err := db.Query("SELECT key, value FROM settings")
	if err != nil {
		return nil, fmt.Errorf("settings migration: read settings: %w", err)
	}
	defer rows.Close()

	out := make(map[string]string)
	for rows.Next() {
		var k, v sql.NullString
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("settings migration: scan setting: %w", err)
		}
		if !k.Valid {
			continue
		}
		out[k.String] = ""
		if v.Valid {
			out[k.String] = v.String
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("settings migration: iterate settings: %w", err)
	}
	return out, nil
}

func currentVersion(settings map[string]string) int {
	raw, ok := settings[SchemaVersionKey]
	if !ok || strings.TrimSpace(raw) == "" {
		return 0
	}
	var v int
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return 0
	}
	return v
}

func buildItems(settings map[string]string) []MigrationItem {
	var items []MigrationItem
	if it, ok := buildCheckinItem(settings); ok {
		items = append(items, it)
	}
	if it, ok := buildCronItem("balance", balanceRefreshScheduleV2Key, "balance_refresh_cron", settings); ok {
		items = append(items, it)
	}
	if it, ok := buildCronItem("log-cleanup", logCleanupScheduleV2Key, "log_cleanup_cron", settings); ok {
		items = append(items, it)
	}
	return items
}

func buildCheckinItem(settings map[string]string) (MigrationItem, bool) {
	if _, ok := settings[checkinScheduleV2Key]; ok {
		return MigrationItem{}, false
	}
	hasLegacy := false
	for _, k := range []string{"checkin_schedule_mode", "checkin_cron", "checkin_interval_hours", "checkin_window_start", "checkin_window_end"} {
		if _, ok := settings[k]; ok {
			hasLegacy = true
			break
		}
	}
	if !hasLegacy {
		return MigrationItem{}, false
	}

	mode := jsonString(settings["checkin_schedule_mode"], "cron")
	cron := jsonString(settings["checkin_cron"], config.DefaultCheckinCron)
	intervalHours := jsonInt(settings["checkin_interval_hours"], config.DefaultCheckinIntervalHours)
	if intervalHours < 1 || intervalHours > 24 {
		intervalHours = config.DefaultCheckinIntervalHours
	}
	windowStart := jsonString(settings["checkin_window_start"], "00:00")
	windowEnd := jsonString(settings["checkin_window_end"], "23:59")

	var spec scheduler.ScheduleSpec
	switch strings.ToLower(mode) {
	case "window":
		spec = scheduler.ScheduleSpec{
			Version:     scheduler.ScheduleSpecVersion,
			Type:        "window",
			WindowStart: windowStart,
			WindowEnd:   windowEnd,
			Cron:        cron,
		}
		if err := spec.Validate(); err != nil {
			spec.WindowStart = "00:00"
			spec.WindowEnd = "23:59"
		}
	case "interval":
		spec = scheduler.ScheduleSpec{
			Version:    scheduler.ScheduleSpecVersion,
			Type:       "interval",
			EveryHours: intervalHours,
			Cron:       cron,
		}
	default:
		spec = scheduler.CronToSchedule(cron)
	}
	return MigrationItem{
		Task:        "checkin",
		LegacyKey:   "checkin_schedule_mode",
		LegacyValue: mode,
		V2Key:       checkinScheduleV2Key,
		Schedule:    spec,
	}, true
}

func buildCronItem(task, v2Key, legacyKey string, settings map[string]string) (MigrationItem, bool) {
	if _, ok := settings[v2Key]; ok {
		return MigrationItem{}, false
	}
	cron := jsonString(settings[legacyKey], "")
	if cron == "" {
		return MigrationItem{}, false
	}
	return MigrationItem{
		Task:        task,
		LegacyKey:   legacyKey,
		LegacyValue: cron,
		V2Key:       v2Key,
		Schedule:    scheduler.CronToSchedule(cron),
	}, true
}

func upsertTx(db *sqlx.DB, tx *sqlx.Tx, key string, value any) error {
	jsonValue, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("settings migration: marshal %q: %w", key, err)
	}
	query := db.Rebind(`INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`)
	if _, err := tx.Exec(query, key, string(jsonValue)); err != nil {
		return fmt.Errorf("settings migration: upsert %q: %w", key, err)
	}
	return nil
}

func jsonString(raw, fallback string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	var s string
	if err := json.Unmarshal([]byte(raw), &s); err == nil {
		if strings.TrimSpace(s) == "" {
			return fallback
		}
		return s
	}
	return raw
}

func jsonInt(raw string, fallback int) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	var v int
	if err := json.Unmarshal([]byte(raw), &v); err == nil {
		return v
	}
	return fallback
}
