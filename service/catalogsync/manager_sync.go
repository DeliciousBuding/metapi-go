package catalogsync

import (
	"context"
	"errors"
	"time"

	"github.com/deliciousbuding/metapi-go/service/pricingcatalog"
)

// Start begins an immediate background sync (when auto sync is enabled)
// plus the periodic loop. Idempotent.
func (m *Manager) Start() {
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return
	}
	m.started = true
	enabled := m.autoEnabled
	m.mu.Unlock()

	if enabled {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			if _, err := m.SyncNow(ctx, 0); err != nil {
				m.logger.Warn("catalogsync: initial sync failed; serving built-in presets", "error", err)
			}
			cancel()
		}()
	}
	if m.interval <= 0 {
		return
	}
	go m.syncLoop()
}

// Stop halts the periodic loop. Safe to call multiple times.
func (m *Manager) Stop() {
	m.stopOnce.Do(func() { close(m.stopCh) })
}

func (m *Manager) syncLoop() {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.mu.Lock()
			enabled := m.autoEnabled
			m.mu.Unlock()
			if !enabled {
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			_, _ = m.SyncNow(ctx, 0)
			cancel()
		}
	}
}

// SyncNow runs a sync pass: all enabled sources (sourceID <= 0) or one
// source. Concurrent runs are declined with ErrSyncBusy. Returns the full
// status payload for the panel.
func (m *Manager) SyncNow(ctx context.Context, sourceID int64) (*SyncStatus, error) {
	if m.provider == nil {
		return nil, errors.New("catalogsync: provider not configured")
	}
	if !m.syncMu.TryLock() {
		return nil, ErrSyncBusy
	}
	defer m.syncMu.Unlock()

	var report *pricingcatalog.RefreshReport
	var err error
	if sourceID > 0 {
		report, err = m.provider.SyncSource(ctx, sourceID)
	} else {
		report, err = m.provider.Refresh(ctx)
	}
	if err != nil {
		return nil, err
	}
	// Persist per-source outcomes (best-effort; sync succeeded regardless).
	for _, sourceReport := range report.Sources {
		if recordErr := m.RecordStatus(ctx, sourceReport); recordErr != nil {
			m.logger.Warn("catalogsync: persist source status failed", "source", sourceReport.Name, "error", recordErr)
		}
	}
	return m.Status(ctx)
}

// ErrSyncBusy reports an overlapping sync run.
var ErrSyncBusy = errors.New("catalogsync: sync already running")

// AutoSyncEnabled reports the current auto-sync toggle.
func (m *Manager) AutoSyncEnabled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.autoEnabled
}

// SetAutoSyncEnabled flips the toggle and persists it to settings.
func (m *Manager) SetAutoSyncEnabled(ctx context.Context, enabled bool) error {
	if err := m.Store.SetAutoSyncEnabled(ctx, enabled); err != nil {
		return err
	}
	m.mu.Lock()
	m.autoEnabled = enabled
	m.mu.Unlock()
	return nil
}

// Interval returns the configured auto-sync period (0 = disabled).
func (m *Manager) Interval() time.Duration {
	return m.interval
}
