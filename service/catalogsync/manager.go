package catalogsync

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/deliciousbuding/metapi-go/service/pricingcatalog"
	"github.com/jmoiron/sqlx"
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
	// AllowPrivateSources permits operator-hosted internal catalog mirrors.
	AllowPrivateSources bool
	// Interval is the auto-sync cadence. <= 0 disables the periodic loop
	// (the initial background fetch still runs when auto sync is enabled).
	Interval time.Duration
	// LegacyURL is the PRICING_CATALOG_URL env value; when it differs from
	// both presets it is seeded as the top-priority custom source.
	LegacyURL string
	// SiteSnapshotSource resolves site platform/URL for vendor
	// classification in ResolveCatalogPricing.
	SiteSnapshotSource pricingcatalog.SiteSnapshotSource
	// HTTPClient overrides the source-fetch client. nil uses the default
	// public-only guarded client (see pricingcatalog.NewProvider).
	HTTPClient *http.Client
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
		Sources:             specs,
		SiteSnapshotSource:  opts.SiteSnapshotSource,
		HTTPClient:          opts.HTTPClient,
		AllowPrivateSources: opts.AllowPrivateSources,
		Logger:              logger,
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

// derefStr converts a nil pointer to "" (the inverse of strPtr).
func derefStr(value *string) string {
	if value == nil {
		return ""
	}
	return *value
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
