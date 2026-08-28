package scheduler

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

// Error-surfacing tests.
//
// Observed defect: channel_recovery.loadActiveCandidates returned nil on a
// query error without any log (its sibling loadCoolingCandidates logs), so a
// broken candidate query was invisible in production. The fix must surface
// the error via structured logging consistent with the package style.

// captureHandler is a minimal slog.Handler that records record messages so
// tests can assert structured log emission without parsing text output.
type captureHandler struct {
	mu   sync.Mutex
	msgs []string
}

func (h *captureHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.msgs = append(h.msgs, r.Message)
	return nil
}

func (h *captureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(string) slog.Handler      { return h }

func (h *captureHandler) contains(substr string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, m := range h.msgs {
		if strings.Contains(m, substr) {
			return true
		}
	}
	return false
}

func (h *captureHandler) all() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]string, len(h.msgs))
	copy(out, h.msgs)
	return out
}

// installCaptureLog swaps the default slog logger for a capturing handler and
// restores the previous logger on test cleanup.
func installCaptureLog(t *testing.T) *captureHandler {
	t.Helper()
	h := &captureHandler{}
	prev := slog.Default()
	slog.SetDefault(slog.New(h))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return h
}

// TestChannelRecoveryLoadActiveCandidatesLogsQueryError forces the fallback
// SQL query to fail (closed DB) and asserts the error is surfaced via
// structured logging. Before the fix the error was swallowed silently.
func TestChannelRecoveryLoadActiveCandidatesLogsQueryError(t *testing.T) {
	// Force the SQL fallback path (no coordinator provider registered).
	SetActiveChannelIDsProvider(nil)
	t.Cleanup(func() { SetActiveChannelIDsProvider(nil) })

	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// Close immediately: every subsequent Query fails with
	// "sql: database is closed", which is exactly the error shape a broken
	// runtime DB produces in production.
	if err := db.Close(); err != nil {
		t.Fatalf("close sqlite: %v", err)
	}

	h := installCaptureLog(t)

	s := NewChannelRecoveryScheduler(testConfig())
	if got := s.loadActiveCandidates(db); got != nil {
		t.Fatalf("expected nil candidates from a closed DB, got %v", got)
	}

	if !h.contains("failed to load active candidates") {
		t.Fatalf("query error was swallowed: no structured log emitted; captured messages: %v", h.all())
	}
}

// TestChannelRecoveryLoadCoolingCandidatesLogsQueryError pins the existing
// (correct) behavior of the sibling loader so the fix stays consistent with
// it: query errors are logged, not returned silently.
func TestChannelRecoveryLoadCoolingCandidatesLogsQueryError(t *testing.T) {
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close sqlite: %v", err)
	}

	h := installCaptureLog(t)

	s := NewChannelRecoveryScheduler(testConfig())
	if got := s.loadCoolingCandidates(db); got != nil {
		t.Fatalf("expected nil candidates from a closed DB, got %v", got)
	}

	if !h.contains("failed to load cooling candidates") {
		t.Fatalf("cooling-candidate query error not logged; captured messages: %v", h.all())
	}
}

// TestBackupWebdavCorruptConfigLogsWarning forces a corrupt stored config and
// asserts the fallback-to-disabled path is surfaced. Before the fix the
// unmarshal error was swallowed, so WebDAV backup silently stopped working
// with zero log trail.
func TestBackupWebdavCorruptConfigLogsWarning(t *testing.T) {
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("auto-migrate: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ss := store.NewSettingsStore(db)
	if err := ss.Set(backupWebdavConfigSettingKey, "{not-json"); err != nil {
		t.Fatalf("seed corrupt config: %v", err)
	}

	h := installCaptureLog(t)

	cfg, err := loadBackupWebdavConfig(ss)
	if err != nil {
		t.Fatalf("expected nil error fallback, got %v", err)
	}
	if cfg.Enabled {
		t.Fatalf("corrupt config must fall back to disabled")
	}
	if !h.contains("stored config is corrupt") {
		t.Fatalf("corrupt config was swallowed: no structured log emitted; captured messages: %v", h.all())
	}
}

// TestBackupWebdavUpdateStateLogsPersistFailure forces the state write to
// fail (closed DB) and asserts the failure is logged. Before the fix
// store.Set's error was discarded.
func TestBackupWebdavUpdateStateLogsPersistFailure(t *testing.T) {
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	ss := store.NewSettingsStore(db)
	if err := db.Close(); err != nil {
		t.Fatalf("close sqlite: %v", err)
	}

	h := installCaptureLog(t)

	s := NewBackupWebdavScheduler(testConfig())
	s.updateState(ss, nil)

	if !h.contains("failed to persist state") {
		t.Fatalf("state persist error was swallowed; captured messages: %v", h.all())
	}
}

// TestModelProbePersistResultLogsWriteFailure forces the history insert to
// fail (closed DB) and asserts the write failure is logged at debug level.
// Before the fix the Exec error was discarded.
func TestModelProbePersistResultLogsWriteFailure(t *testing.T) {
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close sqlite: %v", err)
	}

	h := installCaptureLog(t)

	s := NewModelProbeScheduler(testConfig())
	s.persistProbeResult(db, ProbeTarget{ChannelID: 1, ModelName: "test-model"}, "success", ProbeOutcome{Status: "success"})

	if !h.contains("failed to persist probe result") {
		t.Fatalf("probe persist error was swallowed; captured messages: %v", h.all())
	}
}
