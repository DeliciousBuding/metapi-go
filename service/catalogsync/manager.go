package catalogsync

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/deliciousbuding/metapi-go/service/pricingcatalog"
)

// Manager owns the runtime catalog: the DB registry (Store) plus the
// pricingcatalog Provider that fetches/merges the sources and publishes the
// snapshot consumed by routing (cold-start cost) and the marketplace.
// Implements routing.CatalogPricingResolver structurally (unitCost, source).
type Manager struct {
	*Store

	provider *pricingcatalog.Provider

	// interval is the auto-sync period (0 disables the periodic loop after
	// the initial fetch).
	interval time.Duration

	mu          sync.Mutex // guards autoEnabled
	autoEnabled bool

	syncMu sync.Mutex // serializes sync runs (auto + manual)

	stopCh   chan struct{}
	stopOnce sync.Once
	started  bool

	logger *slog.Logger
}

// Options configures a Manager.
type Options struct {
	// Interval is the auto-sync cadence. <= 0 disables the periodic loop
	// (the initial background fetch still runs when auto sync is enabled).
	Interval time.Duration
	// LegacyURL is the PRICING_CATALOG_URL env value; when it differs from
	// both presets it is seeded as the top-priority custom source.
	LegacyURL string
	// SiteSnapshotSource resolves site platform/URL for vendor
	// classification in ResolveCatalogPricing.
	SiteSnapshotSource pricingcatalog.SiteSnapshotSource
	// Logger defaults to slog.Default().
	Logger *slog.Logger
}

// NewManager builds a Manager, seeds the default registry when empty, and
// wires the provider to the persisted sources. Call Start to begin syncing.
func NewManager(db *sqlx.DB, opts Options) (*Manager, error) {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	store := NewStore(db)
	ctx := context.Background()
	if err := store.EnsureDefaults(ctx, opts.LegacyURL); err != nil {
		return nil, err
	}
	sources, err := store.ListSources(ctx)
	if err != nil {
		return nil, err
	}

	specs := make([]pricingcatalog.SourceSpec, 0, len(sources))
	byID := map[int64]Source{}
	for _, row := range sources {
		if !row.Enabled {
			continue
		}
		specs = append(specs, pricingcatalog.SourceSpec{
			ID:   row.ID,
			Name: row.Name,
			URL:  row.URL,
			Kind: pricingcatalog.SourceKindAuto,
		})
		byID[row.ID] = row
	}

	provider := pricingcatalog.NewProvider(pricingcatalog.Options{
		Sources:            specs,
		SiteSnapshotSource: opts.SiteSnapshotSource,
		Logger:             logger,
	})
	// Restore persisted last-known status so the panel shows history before
	// the first sync of this process.
	for _, row := range sources {
		provider.SeedSourceStatus(pricingcatalog.SourceReport{
			ID:          row.ID,
			Name:        row.Name,
			URL:         row.URL,
			ModelCount:  row.LastCount,
			LastSuccess: row.LastSuccessAt,
			LastError:   derefStr(row.LastError),
			AttemptedAt: orZero(row.LastAttemptAt),
		})
	}

	autoEnabled, err := store.AutoSyncEnabled(ctx)
	if err != nil {
		return nil, err
	}

	return &Manager{
		Store:       store,
		provider:    provider,
		interval:    opts.Interval,
		autoEnabled: autoEnabled,
		stopCh:      make(chan struct{}),
		logger:      logger,
	}, nil
}

func orZero(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return *value
}

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
	return m.Status(ctx), nil
}

// ErrSyncBusy reports an overlapping sync run.
var ErrSyncBusy = errors.New("catalogsync: sync already running")

// SyncStatus is the status payload served to the panel.
type SyncStatus struct {
	AutoSync    bool           `json:"autoSync"`
	IntervalMin int            `json:"intervalMin"`
	Snapshot    SnapshotStatus `json:"snapshot"`
	Sources     []Source       `json:"sources"`
}

// SnapshotStatus describes the published merged snapshot.
type SnapshotStatus struct {
	Source    string     `json:"source"`
	FetchedAt *time.Time `json:"fetchedAt"`
	Models    int        `json:"models"`
}

func (m *Manager) Status(ctx context.Context) *SyncStatus {
	m.mu.Lock()
	autoSync := m.autoEnabled
	m.mu.Unlock()

	intervalMin := 0
	if m.interval > 0 {
		intervalMin = int(m.interval / time.Minute)
	}

	sources, err := m.ListSources(ctx)
	if err != nil {
		sources = []Source{}
	}
	// Live provider statuses win over DB rows for the just-run attempt.
	live := m.provider.SourceStatuses()
	liveByID := map[int64]pricingcatalog.SourceReport{}
	liveByName := map[string]pricingcatalog.SourceReport{}
	for _, report := range live {
		liveByID[report.ID] = report
		liveByName[report.Name] = report
	}
	merged := make([]Source, 0, len(sources))
	for _, row := range sources {
		if liveReport, ok := liveByID[row.ID]; ok {
			row.LastSuccessAt = liveReport.LastSuccess
			row.LastError = strPtr(liveReport.LastError)
			row.LastCount = liveReport.ModelCount
			if !liveReport.AttemptedAt.IsZero() {
				row.LastAttemptAt = &liveReport.AttemptedAt
			}
		} else if liveReport, ok := liveByName[row.Name]; ok {
			row.LastSuccessAt = liveReport.LastSuccess
			row.LastError = strPtr(liveReport.LastError)
			row.LastCount = liveReport.ModelCount
			if !liveReport.AttemptedAt.IsZero() {
				row.LastAttemptAt = &liveReport.AttemptedAt
			}
		}
		merged = append(merged, row)
	}

	snapshot := m.provider.Snapshot()
	var fetchedAt *time.Time
	if snapshot != nil && !snapshot.FetchedAt.IsZero() {
		t := snapshot.FetchedAt
		fetchedAt = &t
	}
	snapSource := ""
	if snapshot != nil {
		snapSource = snapshot.Source
	}

	return &SyncStatus{
		AutoSync:    autoSync,
		IntervalMin: intervalMin,
		Snapshot: SnapshotStatus{
			Source:    snapSource,
			FetchedAt: fetchedAt,
			Models:    snapshotLen(snapshot),
		},
		Sources: merged,
	}
}

func snapshotLen(snapshot *pricingcatalog.CatalogSnapshot) int {
	if snapshot == nil {
		return 0
	}
	return snapshot.Len()
}

// strPtr converts an empty string to nil so NULL columns round-trip.
func strPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

// derefStr converts a nil pointer to "" (the inverse of strPtr).
func derefStr(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

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

// Snapshot returns the published merged catalog snapshot (never nil).
func (m *Manager) Snapshot() *pricingcatalog.CatalogSnapshot {
	return m.provider.Snapshot()
}

// ReloadSources re-reads the registry into the provider after CRUD changes.
func (m *Manager) ReloadSources(ctx context.Context) error {
	sources, err := m.ListSources(ctx)
	if err != nil {
		return err
	}
	specs := make([]pricingcatalog.SourceSpec, 0, len(sources))
	for _, row := range sources {
		if !row.Enabled {
			continue
		}
		specs = append(specs, pricingcatalog.SourceSpec{
			ID:   row.ID,
			Name: row.Name,
			URL:  row.URL,
			Kind: pricingcatalog.SourceKindAuto,
		})
	}
	m.provider.SetSources(specs)
	return nil
}

// CreateSource persists a new source and re-wires the provider.
func (m *Manager) CreateSource(ctx context.Context, in SourceInput) (Source, error) {
	source, err := m.Store.CreateSource(ctx, in)
	if err != nil {
		return Source{}, err
	}
	if err := m.ReloadSources(ctx); err != nil {
		return Source{}, err
	}
	return source, nil
}

// UpdateSource patches a source and re-wires the provider.
func (m *Manager) UpdateSource(ctx context.Context, id int64, in SourceInput) (Source, error) {
	source, err := m.Store.UpdateSource(ctx, id, in)
	if err != nil {
		return Source{}, err
	}
	if err := m.ReloadSources(ctx); err != nil {
		return Source{}, err
	}
	return source, nil
}

// DeleteSource removes a source and re-wires the provider.
func (m *Manager) DeleteSource(ctx context.Context, id int64) error {
	if err := m.Store.DeleteSource(ctx, id); err != nil {
		return err
	}
	return m.ReloadSources(ctx)
}

// ResolveCatalogPricing implements routing.CatalogPricingResolver
// structurally: it delegates to the underlying provider (site-classified
// official vs relay provenance).
func (m *Manager) ResolveCatalogPricing(siteID, accountID int64, modelName string) (unitCost *float64, source string) {
	if m == nil || m.provider == nil {
		return nil, ""
	}
	return m.provider.ResolveCatalogPricing(siteID, accountID, modelName)
}
