package app

import (
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
)

// TestBuildSchedulersRegistration guards the full background-scheduler wiring.
// Regression: balance-refresh, log-cleanup and backup-webdav were constructed
// but never Register()ed, so they silently never started. This test fails if a
// scheduler is added or removed without keeping the expected set in sync.
//
// Note: the update-center scheduler is gated behind METAPI_ENABLE_UPDATE_CENTER
// (default false) and is therefore absent from the default set — see
// TestBuildSchedulers_UpdateCenterGatedByEnv for the opt-in path.
func TestBuildSchedulersRegistration(t *testing.T) {
	cfg := &config.Config{}
	reg, checkin, balance, logCleanup, webdav := buildSchedulers(cfg)

	got := reg.List()
	want := []string{
		"usage-aggregation",
		"checkin",
		"balance-refresh",
		"daily-summary",
		"log-cleanup",
		"backup-webdav",
		"site-announcement",
		"model-probe",
		"channel-recovery",
		"sub2api-refresh",
		"admin-snapshot",
		"proxy-file-retention",
		"proxy-video-task-retention",
		"proxy-log-retention",
		"oauth-refresh",
	}

	if len(got) != len(want) {
		t.Fatalf("registered %d schedulers, want %d: got=%v", len(got), len(want), got)
	}

	seen := map[string]bool{}
	for _, name := range got {
		if seen[name] {
			t.Errorf("duplicate scheduler name %q", name)
		}
		seen[name] = true
	}
	for _, name := range want {
		if !seen[name] {
			t.Errorf("scheduler %q not registered; registered=%v", name, got)
		}
	}
	// Sanity: update-center must NOT be registered when the flag is off.
	if seen["update-center"] {
		t.Errorf("update-center registered without METAPI_ENABLE_UPDATE_CENTER; got=%v", got)
	}

	// The four hot-reloadable schedulers must be returned as non-nil pointers.
	for name, ptr := range map[string]any{
		"checkin":    checkin,
		"balance":    balance,
		"logCleanup": logCleanup,
		"webdav":     webdav,
	} {
		if ptr == nil {
			t.Errorf("%s scheduler pointer is nil", name)
		}
	}
}

// TestBuildSchedulers_UpdateCenterGatedByEnv asserts the update-center
// scheduler is registered only when cfg.UpdateCenterEnabled is true.
func TestBuildSchedulers_UpdateCenterGatedByEnv(t *testing.T) {
	t.Run("disabled by default", func(t *testing.T) {
		cfg := &config.Config{} // UpdateCenterEnabled = false
		reg, _, _, _, _ := buildSchedulers(cfg)
		for _, name := range reg.List() {
			if name == "update-center" {
				t.Fatalf("update-center registered while METAPI_ENABLE_UPDATE_CENTER is off: %v", reg.List())
			}
		}
	})

	t.Run("enabled when flag set", func(t *testing.T) {
		cfg := &config.Config{UpdateCenterEnabled: true}
		reg, _, _, _, _ := buildSchedulers(cfg)
		var found bool
		for _, name := range reg.List() {
			if name == "update-center" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("update-center not registered when UpdateCenterEnabled=true: %v", reg.List())
		}
	})
}
