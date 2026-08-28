package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
)

const defaultSiteAnnouncementIntervalMs = 15 * 60 * 1000 // 15 minutes

// SiteAnnouncementSyncResult is the honest outcome of a real announcement sync pass.
// Field names mirror admin SyncSiteAnnouncements / TS syncSiteAnnouncements.
type SiteAnnouncementSyncResult struct {
	ScannedSites  int
	Inserted      int
	Updated       int
	Unsupported   int
	Notifications int
	Events        int
	Failed        int
}

// SiteAnnouncementSyncFunc runs a full announcement sync against the given DB.
// Implementations must call real platform adapters (no invented success counts).
// Injected from app/admin composition root so scheduler does not import handler/admin.
type SiteAnnouncementSyncFunc func(db *sqlx.DB) SiteAnnouncementSyncResult

var (
	defaultSiteAnnouncementSyncMu sync.RWMutex
	defaultSiteAnnouncementSync   SiteAnnouncementSyncFunc
)

// SetDefaultSiteAnnouncementSyncFunc installs the process-wide sync implementation
// used by NewSiteAnnouncementScheduler when no per-instance SyncFunc is set.
// Safe to call before StartBackgroundServices (routes register first).
func SetDefaultSiteAnnouncementSyncFunc(fn SiteAnnouncementSyncFunc) {
	defaultSiteAnnouncementSyncMu.Lock()
	defer defaultSiteAnnouncementSyncMu.Unlock()
	defaultSiteAnnouncementSync = fn
}

func getDefaultSiteAnnouncementSyncFunc() SiteAnnouncementSyncFunc {
	defaultSiteAnnouncementSyncMu.RLock()
	defer defaultSiteAnnouncementSyncMu.RUnlock()
	return defaultSiteAnnouncementSync
}

// SiteAnnouncementScheduler polls site announcements periodically.
type SiteAnnouncementScheduler struct {
	cfg      *config.Config
	mu       sync.Mutex
	inFlight bool
	syncFunc SiteAnnouncementSyncFunc
	runner   *intervalRunner
}

// NewSiteAnnouncementScheduler creates a new site announcement poller.
// Picks up any process-wide SyncFunc installed via SetDefaultSiteAnnouncementSyncFunc.
func NewSiteAnnouncementScheduler(cfg *config.Config) *SiteAnnouncementScheduler {
	return &SiteAnnouncementScheduler{
		cfg:      cfg,
		syncFunc: getDefaultSiteAnnouncementSyncFunc(),
		runner:   &intervalRunner{},
	}
}

// SetSyncFunc injects (or clears) the real sync implementation for this instance.
// When fn is nil, the scheduler keeps scan-only behavior.
func (s *SiteAnnouncementScheduler) SetSyncFunc(fn SiteAnnouncementSyncFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.syncFunc = fn
}

func (s *SiteAnnouncementScheduler) Name() string { return "site-announcement" }

func (s *SiteAnnouncementScheduler) Start(ctx context.Context) error {
	interval := time.Duration(config.MaxInt64(defaultSiteAnnouncementIntervalMs, 10_000)) * time.Millisecond
	slog.Info("site-announcement scheduler started", "interval_ms", interval.Milliseconds())
	return s.runner.start(ctx, interval, true, s.runSync)
}

func (s *SiteAnnouncementScheduler) Stop() error {
	return s.runner.stop()
}

func (s *SiteAnnouncementScheduler) runSync() {
	s.mu.Lock()
	if s.inFlight {
		s.mu.Unlock()
		return
	}
	s.inFlight = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.inFlight = false
		s.mu.Unlock()
	}()

	dbw := store.GetDB()
	if dbw == nil {
		return
	}
	jobCtx, cancel := context.WithTimeout(context.Background(), siteAnnouncementJobTimeout)
	defer cancel()
	runWithSchedulerLease(jobCtx, dbw, s.Name(), func() {
		s.runSyncLocked(dbw)
	})
}

func (s *SiteAnnouncementScheduler) runSyncLocked(dbw *store.DB) {
	s.mu.Lock()
	fn := s.syncFunc
	s.mu.Unlock()

	if fn == nil {
		// Known limitation: no SyncFunc injected — scan-only, no platform calls.
		// What runs: enumerate active sites under the scheduler lease and log the
		// count. What does NOT run: platform adapter GetSiteAnnouncements, DB
		// insert/update into site_announcements, notifications, or events.
		// Production wires SyncFunc via admin.SyncSiteAnnouncements.
		s.runResidualScan(dbw)
		return
	}

	slog.Info("site-announcement: running sync")
	result := fn(dbw.DB)
	slog.Info("site-announcement: sync complete",
		"scanned", result.ScannedSites,
		"inserted", result.Inserted,
		"updated", result.Updated,
		"unsupported", result.Unsupported,
		"notifications", result.Notifications,
		"events", result.Events,
		"failed", result.Failed,
	)
}

func (s *SiteAnnouncementScheduler) runResidualScan(dbw *store.DB) {
	slog.Info("site-announcement: residual scan (no platform sync)")

	rows, err := dbw.Query("SELECT id, platform, url FROM sites WHERE status = 'active'")
	if err != nil {
		slog.Error("site-announcement: failed to query sites", "error", err)
		return
	}
	defer rows.Close()

	count := 0
	var scanErr error
	for rows.Next() {
		var id int64
		var platform, url string
		if err := rows.Scan(&id, &platform, &url); err != nil {
			if scanErr == nil {
				scanErr = err
			}
			continue
		}
		_ = id
		_ = platform
		_ = url
		// Known limitation: site row is counted only. No adapter / DB write.
		count++
	}
	if err := rows.Err(); err != nil && scanErr == nil {
		scanErr = err
	}
	if scanErr != nil {
		slog.Warn("site-announcement: residual scan rows degraded", "error", scanErr)
	}

	slog.Info("site-announcement: residual scan complete (no announcements written)", "sites", count)
}
