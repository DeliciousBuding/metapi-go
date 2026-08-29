package scheduler

import (
	"encoding/json"
	"math"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
)

// ---- Setting resolution utilities ----

// resolveCronSetting reads a cron expression from the DB settings table,
// validates it, and returns the fallback if invalid or missing.
// Mirrors TS resolveCronSetting().
func resolveCronSetting(settingKey string, fallback string) string {
	db := store.GetDB()
	if db == nil {
		return fallback
	}

	settingsStore := store.NewSettingsStore(db)
	raw, err := settingsStore.Get(settingKey)
	if err != nil || raw == "" {
		return fallback
	}

	var value string
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return fallback
	}

	if !ValidateCronExpr(value) {
		return fallback
	}

	return value
}

// resolveBooleanSetting reads a boolean from the DB settings table.
// Falls back to the provided default if missing or invalid.
func resolveBooleanSetting(settingKey string, fallback bool) bool {
	db := store.GetDB()
	if db == nil {
		return fallback
	}

	settingsStore := store.NewSettingsStore(db)
	raw, err := settingsStore.Get(settingKey)
	if err != nil || raw == "" {
		return fallback
	}

	var value bool
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return fallback
	}

	return value
}

// resolveStringSetting reads a string value from DB settings.
// E1: window bounds hydration (checkin_window_start/end).
func resolveStringSetting(settingKey string, fallback string) string {
	db := store.GetDB()
	if db == nil {
		return fallback
	}

	settingsStore := store.NewSettingsStore(db)
	raw, err := settingsStore.Get(settingKey)
	if err != nil || raw == "" {
		return fallback
	}

	var value string
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return fallback
	}
	if value == "" {
		return fallback
	}
	return value
}

// resolvePositiveIntegerSetting reads a positive integer (>=1) from DB settings.
func resolvePositiveIntegerSetting(settingKey string, fallback int) int {
	db := store.GetDB()
	if db == nil {
		return fallback
	}

	settingsStore := store.NewSettingsStore(db)
	raw, err := settingsStore.Get(settingKey)
	if err != nil || raw == "" {
		return fallback
	}

	var value float64
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return fallback
	}

	if math.IsInf(value, 0) || math.IsNaN(value) || value < 1 {
		return fallback
	}

	return int(math.Trunc(value))
}

// ---- Common helpers ----

// toISOTime returns the current time as an ISO 8601 string.
func toISOTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

// resolveCheckinScheduleMode reads the checkin schedule mode from config and DB.
// Returns "cron", "interval", or "window" (E1).
func resolveCheckinScheduleMode(rt *config.RuntimeSettings) string {
	db := store.GetDB()
	if db == nil {
		return rt.CheckinScheduleMode
	}

	settingsStore := store.NewSettingsStore(db)
	raw, err := settingsStore.Get("checkin_schedule_mode")
	if err != nil || raw == "" {
		return rt.CheckinScheduleMode
	}

	var value string
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return rt.CheckinScheduleMode
	}

	value = strings.TrimSpace(strings.ToLower(value))
	switch value {
	case "interval":
		return "interval"
	case "window":
		return "window"
	}
	return "cron"
}
