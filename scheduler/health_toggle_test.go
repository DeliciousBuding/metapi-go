package scheduler

import (
	"context"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
)

// Focused tests for the issue #1027 global kill switches on the
// upstream-touching account schedulers (check-in, balance refresh, model
// probe). Zero-value config means "enabled" everywhere (historical
// behavior); the switches opt out.

// TestCheckinScheduler_KillSwitchCronMode covers Start gating, the
// no-re-arm guarantee on schedule updates while disabled, and hot toggling.
func TestCheckinScheduler_KillSwitchCronMode(t *testing.T) {
	store.OverrideDB(nil)
	t.Cleanup(func() { store.OverrideDB(nil) })

	config.SetRuntime(&config.RuntimeSettings{
		CheckinCron:         "0 8 * * *",
		CheckinScheduleMode: "cron",
		CheckinDisabled:     true,
	})
	t.Cleanup(func() { config.SetRuntime(nil) })
	s := NewCheckinScheduler(&config.Config{})

	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if s.cronRunner != nil || s.intervalTimer != nil {
		t.Fatalf("disabled Start must not arm any runner")
	}

	// A schedule update while disabled persists the schedule but must not
	// silently re-arm the runner.
	if err := s.UpdateCheckinSchedule("cron", "0 9 * * *", 6, "00:00", "23:59"); err != nil {
		t.Fatalf("UpdateCheckinSchedule: %v", err)
	}
	if config.Runtime().CheckinCron != "0 9 * * *" {
		t.Fatalf("cron = %q, want the updated value persisted", config.Runtime().CheckinCron)
	}
	if s.cronRunner != nil {
		t.Fatalf("schedule update must not re-arm a disabled scheduler")
	}

	// Enabling arms the cron runner with the persisted schedule.
	s.SetEnabled(true)
	if config.Runtime().CheckinDisabled {
		t.Fatalf("SetEnabled(true) must clear CheckinDisabled")
	}
	if s.cronRunner == nil {
		t.Fatalf("SetEnabled(true) must arm the cron runner")
	}

	// Disabling stops it again.
	s.SetEnabled(false)
	if !config.Runtime().CheckinDisabled {
		t.Fatalf("SetEnabled(false) must set CheckinDisabled")
	}
	if s.cronRunner != nil {
		t.Fatalf("SetEnabled(false) must stop the cron runner")
	}
}

// TestCheckinScheduler_KillSwitchIntervalMode covers the interval-mode arm
// path for the same switch.
func TestCheckinScheduler_KillSwitchIntervalMode(t *testing.T) {
	store.OverrideDB(nil)
	t.Cleanup(func() { store.OverrideDB(nil) })

	config.SetRuntime(&config.RuntimeSettings{
		CheckinScheduleMode:  "interval",
		CheckinIntervalHours: 6,
		CheckinDisabled:      true,
	})
	t.Cleanup(func() { config.SetRuntime(nil) })
	s := NewCheckinScheduler(&config.Config{})

	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if s.intervalStop != nil {
		t.Fatalf("disabled Start must not arm the interval loop")
	}

	s.SetEnabled(true)
	if s.intervalStop == nil {
		t.Fatalf("SetEnabled(true) must arm the interval loop")
	}

	s.SetEnabled(false)
	select {
	case <-s.intervalStop:
		// closed — the interval loop is stopped
	default:
		t.Fatalf("SetEnabled(false) must close the interval stop channel")
	}
}

// TestCheckinScheduler_KillSwitchFromDBSetting verifies the checkin_enabled
// DB setting wins over the config default at Start (dual-dialect safe: the
// settings key/value table is identical on SQLite and PostgreSQL).
func TestCheckinScheduler_KillSwitchFromDBSetting(t *testing.T) {
	db := openSettingsTestDB(t)
	store.OverrideDB(db)
	t.Cleanup(func() { store.OverrideDB(nil) })

	insertSetting(t, db, "checkin_enabled", "false")

	config.SetRuntime(&config.RuntimeSettings{CheckinCron: "0 8 * * *", CheckinScheduleMode: "cron"})
	t.Cleanup(func() { config.SetRuntime(nil) })
	s := NewCheckinScheduler(&config.Config{})
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !config.Runtime().CheckinDisabled {
		t.Fatalf("DB setting checkin_enabled=false must disable the scheduler")
	}
	if s.cronRunner != nil {
		t.Fatalf("disabled-by-setting Start must not arm the cron runner")
	}
}

// TestBalanceScheduler_KillSwitch covers Start gating and hot toggling for
// the balance refresh scheduler.
func TestBalanceScheduler_KillSwitch(t *testing.T) {
	store.OverrideDB(nil)
	t.Cleanup(func() { store.OverrideDB(nil) })

	config.SetRuntime(&config.RuntimeSettings{BalanceRefreshCron: "0 * * * *", BalanceRefreshDisabled: true})
	t.Cleanup(func() { config.SetRuntime(nil) })
	s := NewBalanceScheduler(&config.Config{})

	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if s.cronRunner != nil {
		t.Fatalf("disabled Start must not arm the cron runner")
	}

	// Hot enable arms the runner with the cfg cron (no DB setting present).
	if err := s.SetEnabled(true); err != nil {
		t.Fatalf("SetEnabled(true): %v", err)
	}
	if s.cronRunner == nil {
		t.Fatalf("SetEnabled(true) must arm the cron runner")
	}

	if err := s.SetEnabled(false); err != nil {
		t.Fatalf("SetEnabled(false): %v", err)
	}
	if s.cronRunner != nil {
		t.Fatalf("SetEnabled(false) must stop the cron runner")
	}
}

// TestBalanceScheduler_KillSwitchFromDBSetting verifies the
// balance_refresh_enabled DB setting wins over the config default at Start.
func TestBalanceScheduler_KillSwitchFromDBSetting(t *testing.T) {
	db := openSettingsTestDB(t)
	store.OverrideDB(db)
	t.Cleanup(func() { store.OverrideDB(nil) })

	insertSetting(t, db, "balance_refresh_enabled", "false")

	config.SetRuntime(&config.RuntimeSettings{BalanceRefreshCron: "0 * * * *"})
	t.Cleanup(func() { config.SetRuntime(nil) })
	s := NewBalanceScheduler(&config.Config{})
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !config.Runtime().BalanceRefreshDisabled {
		t.Fatalf("DB setting balance_refresh_enabled=false must disable the scheduler")
	}
	if s.cronRunner != nil {
		t.Fatalf("disabled-by-setting Start must not arm the cron runner")
	}
}

// TestModelProbeScheduler_SetEnabledHotToggle covers the runtime toggle that
// previously only took effect at Start: enabling must start the ticker and
// disabling must stop it without a restart.
func TestModelProbeScheduler_SetEnabledHotToggle(t *testing.T) {
	store.OverrideDB(nil)
	t.Cleanup(func() { store.OverrideDB(nil) })

	cfg := &config.Config{
		ModelAvailabilityProbeIntervalMs:  60_000,
		ModelAvailabilityProbeTimeoutMs:   3000,
		ModelAvailabilityProbeConcurrency: 1,
	}
	config.SetRuntime(&config.RuntimeSettings{ModelAvailabilityProbeEnabled: false})
	t.Cleanup(func() { config.SetRuntime(nil) })
	s := NewModelProbeScheduler(cfg)
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	runnerRunning := func() bool {
		s.runner.mu.Lock()
		defer s.runner.mu.Unlock()
		return s.runner.running
	}

	if runnerRunning() {
		t.Fatalf("probe ticker must not run while disabled")
	}
	if err := s.SetEnabled(true); err != nil {
		t.Fatalf("SetEnabled(true): %v", err)
	}
	if !config.Runtime().ModelAvailabilityProbeEnabled {
		t.Fatalf("SetEnabled(true) must set the enabled flag")
	}
	if !runnerRunning() {
		t.Fatalf("SetEnabled(true) must start the probe ticker")
	}
	if err := s.SetEnabled(false); err != nil {
		t.Fatalf("SetEnabled(false): %v", err)
	}
	if runnerRunning() {
		t.Fatalf("SetEnabled(false) must stop the probe ticker")
	}
	// Idempotent double-disable.
	if err := s.SetEnabled(false); err != nil {
		t.Fatalf("second SetEnabled(false): %v", err)
	}
}
