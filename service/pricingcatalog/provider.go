package pricingcatalog

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// maxCatalogPayloadBytes bounds the models.dev payload read (dataset is ~2 MB;
// the bound protects against an unexpectedly large response body).
const maxCatalogPayloadBytes = 32 << 20

// SiteSnapshotSource resolves site metadata used for vendor classification.
type SiteSnapshotSource interface {
	// GetSiteSnapshot returns the site's platform/URL. ok=false means the
	// site does not exist; errors are treated as "classify as relay" by the
	// provider.
	GetSiteSnapshot(ctx context.Context, siteID int64) (SiteSnapshot, bool, error)
}

// Options configures a catalog Provider.
type Options struct {
	// RefreshInterval is the periodic refresh cadence. <= 0 disables
	// periodic refresh after the initial background fetch.
	RefreshInterval time.Duration
	// FetchURL defaults to DefaultCatalogURL (models.dev api.json).
	FetchURL string
	// HTTPClient defaults to a client with a 30s timeout.
	HTTPClient *http.Client
	// SiteSnapshotSource resolves site platform/URL for vendor
	// classification. nil → every site classifies as relay (honest default).
	SiteSnapshotSource SiteSnapshotSource
	// SiteClassCacheTTL controls how long a site classification is cached
	// before re-querying the source. Defaults to 1 minute.
	SiteClassCacheTTL time.Duration
	// Logger defaults to slog.Default().
	Logger *slog.Logger
}

// Provider serves models.dev catalog pricing for cold-start cost routing. The
// published catalog snapshot is swapped atomically on refresh, so route
// selection never blocks on network I/O.
//
// Provenance: official vendor sites are labeled "catalog"; relay sites are
// labeled "catalog_estimate" because the official list price is not evidence
// of what the relay actually charges.
type Provider struct {
	snapshot atomic.Pointer[CatalogSnapshot]

	fetchURL     string
	httpClient   *http.Client
	siteSource   SiteSnapshotSource
	siteClassTTL time.Duration
	logger       *slog.Logger

	classMu    sync.Mutex
	classCache map[int64]cachedSiteClass

	refreshInterval time.Duration
	stopCh          chan struct{}
	stopOnce        sync.Once
	startMu         sync.Mutex
	started         bool
}

type cachedSiteClass struct {
	class     SiteClass
	expiresAt time.Time
}

// NewProvider builds a Provider. The initial snapshot is the compile-time
// preset table; call Refresh (or Start) to load the live models.dev dataset.
func NewProvider(opts Options) *Provider {
	fetchURL := strings.TrimSpace(opts.FetchURL)
	if fetchURL == "" {
		fetchURL = DefaultCatalogURL
	}
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	siteClassTTL := opts.SiteClassCacheTTL
	if siteClassTTL <= 0 {
		siteClassTTL = time.Minute
	}

	provider := &Provider{
		fetchURL:        fetchURL,
		httpClient:      httpClient,
		siteSource:      opts.SiteSnapshotSource,
		siteClassTTL:    siteClassTTL,
		logger:          logger,
		classCache:      make(map[int64]cachedSiteClass),
		refreshInterval: opts.RefreshInterval,
		stopCh:          make(chan struct{}),
	}
	provider.snapshot.Store(NewPresetSnapshot())
	return provider
}

// Start kicks off an immediate background fetch plus the periodic refresh
// loop. Start is non-blocking: the preset snapshot serves queries until the
// first successful fetch. Idempotent — repeated calls are no-ops.
func (p *Provider) Start() {
	p.startMu.Lock()
	if p.started {
		p.startMu.Unlock()
		return
	}
	p.started = true
	p.startMu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		if err := p.Refresh(ctx); err != nil {
			p.logger.Warn("pricingcatalog: initial fetch failed; serving built-in presets", "error", err)
		}
		cancel()
	}()

	if p.refreshInterval <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(p.refreshInterval)
		defer ticker.Stop()
		for {
			select {
			case <-p.stopCh:
				return
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				_ = p.Refresh(ctx)
				cancel()
			}
		}
	}()
}

// Stop halts the periodic refresh loop. Safe to call multiple times; the
// current snapshot stays available for queries.
func (p *Provider) Stop() {
	p.stopOnce.Do(func() { close(p.stopCh) })
}

// Refresh fetches and parses the catalog, then atomically publishes the new
// snapshot. On any error the current snapshot is kept (presets until the
// first success), so a flaky endpoint never erases a working catalog.
func (p *Provider) Refresh(ctx context.Context) error {
	data, err := p.fetchCatalog(ctx)
	if err != nil {
		p.logger.Warn("pricingcatalog: catalog fetch failed; keeping current snapshot", "error", err)
		return err
	}
	snapshot, err := ParseCatalog(data)
	if err != nil {
		p.logger.Warn("pricingcatalog: catalog parse failed; keeping current snapshot", "error", err)
		return err
	}
	snapshot.Source = "models.dev api.json"
	snapshot.FetchedAt = time.Now()
	p.snapshot.Store(snapshot)
	p.logger.Info("pricingcatalog: catalog refreshed", "url", p.fetchURL, "models", snapshot.Len())
	return nil
}

func (p *Provider) fetchCatalog(ctx context.Context) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, p.fetchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("pricingcatalog: build request: %w", err)
	}
	response, err := p.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("pricingcatalog: fetch %s: %w", p.fetchURL, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pricingcatalog: fetch %s: unexpected status %d", p.fetchURL, response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxCatalogPayloadBytes))
	if err != nil {
		return nil, fmt.Errorf("pricingcatalog: read %s: %w", p.fetchURL, err)
	}
	return data, nil
}

// Snapshot returns the currently published catalog (never nil).
func (p *Provider) Snapshot() *CatalogSnapshot {
	return p.snapshot.Load()
}

// ResolveCatalogPricing implements routing.CatalogPricingResolver structurally
// (this package must not import routing per design/BACKEND.md — see the
// source literal constants at the top of catalog.go).
//
// unitCost is the estimated per-request cost derived from the official list
// price (1k input + 1k output reference sample). source is SourceOfficial for
// vendor-hosted sites and SourceRelayEstimate for third-party relays, so the
// official list price is never presented as a real relay payment price.
// (nil, "") declines the query (unknown model, no catalog data).
func (p *Provider) ResolveCatalogPricing(siteID, accountID int64, modelName string) (unitCost *float64, source string) {
	if p == nil {
		return nil, ""
	}
	snapshot := p.snapshot.Load()
	if snapshot == nil {
		return nil, ""
	}
	entry, ok := snapshot.Lookup(modelName)
	if !ok {
		return nil, ""
	}
	unit := entry.ReferenceUnitCost()
	if !(unit > 0) || math.IsNaN(unit) || math.IsInf(unit, 0) {
		return nil, ""
	}

	source = SourceRelayEstimate
	if p.classifySiteCached(siteID) == SiteClassOfficial {
		source = SourceOfficial
	}
	return &unit, source
}

// CatalogUnitCost is the pricingFn-shaped query
// (func(siteID, accountID int64, modelName string) *float64) for consumers
// that only need the number. It deliberately drops provenance; prefer
// ResolveCatalogPricing when the source label matters.
func (p *Provider) CatalogUnitCost(siteID, accountID int64, modelName string) *float64 {
	unitCost, _ := p.ResolveCatalogPricing(siteID, accountID, modelName)
	return unitCost
}

func (p *Provider) classifySiteCached(siteID int64) SiteClass {
	if siteID <= 0 || p.siteSource == nil {
		return SiteClassRelay
	}
	now := time.Now()

	p.classMu.Lock()
	cached, ok := p.classCache[siteID]
	p.classMu.Unlock()
	if ok && now.Before(cached.expiresAt) {
		return cached.class
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	snapshot, found, err := p.siteSource.GetSiteSnapshot(ctx, siteID)

	class := SiteClassRelay
	switch {
	case err != nil:
		p.logger.Warn("pricingcatalog: site classification failed; treating as relay", "siteID", siteID, "error", err)
	case !found:
		p.logger.Debug("pricingcatalog: site not found; treating as relay", "siteID", siteID)
	default:
		class = ClassifySite(snapshot)
	}

	p.classMu.Lock()
	p.classCache[siteID] = cachedSiteClass{class: class, expiresAt: now.Add(p.siteClassTTL)}
	p.classMu.Unlock()
	return class
}
