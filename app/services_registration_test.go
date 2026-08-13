package app

import (
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
)

// TestBuildSchedulersRegistration guards the full background-scheduler wiring.
// Regression: balance-refresh, log-cleanup and backup-webdav were constructed
// but never Register()ed, so they silently never started. This test fails if a
// scheduler is added or removed without keeping the expected set in sync.
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
		"update-center",
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
