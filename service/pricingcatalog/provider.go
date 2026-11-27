package pricingcatalog

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

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
	// FetchURL is the legacy single-source dataset URL; used when Sources
	// is empty. Defaults to DefaultCatalogURL (models.dev api.json).
	FetchURL string
	// Sources is the ordered multi-source registry (first wins on merge).
	// When non-empty it takes precedence over FetchURL.
	Sources []SourceSpec
	// HTTPClient defaults to a client with a 30s timeout.
	HTTPClient *http.Client
	// AllowPrivateSources permits operator-hosted catalog mirrors on
	// loopback/RFC1918. Imports cannot set it: it comes from deployment
	// configuration, not from the source row.
	AllowPrivateSources bool
	// SiteSnapshotSource resolves site platform/URL for vendor
	// classification. nil → every site classifies as relay (honest default).
	SiteSnapshotSource SiteSnapshotSource
	// SiteClassCacheTTL controls how long a site classification is cached
	// before re-querying the source. Defaults to 1 minute.
	SiteClassCacheTTL time.Duration
	// Logger defaults to slog.Default().
	Logger *slog.Logger
}

// Provider serves the merged catalog snapshot (llm-metadata primary +
// models.dev fallback by default) for cold-start cost routing and
// marketplace hydration. The published snapshot is swapped atomically on
// refresh, so route selection never blocks on network I/O.
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

	sourcesMu sync.RWMutex
	sources   []sourceRuntime

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
// preset table; call Refresh (or Start) to load the live dataset(s).
func NewProvider(opts Options) *Provider {
	fetchURL := strings.TrimSpace(opts.FetchURL)
	if fetchURL == "" {
		fetchURL = DefaultCatalogURL
	}
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = newCatalogHTTPClient(opts.AllowPrivateSources)
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
	provider.SetSources(opts.Sources)
	provider.snapshot.Store(NewPresetSnapshot())
	return provider
}

// Snapshot returns the currently published catalog (never nil).
func (p *Provider) Snapshot() *CatalogSnapshot {
	return p.snapshot.Load()
}

// ResolveCatalogPricing implements routing.CatalogPricingResolver structurally
// (this package must not import routing per design/BACKEND.md — see the
// source literal constants at the top of catalog.go).
//
// unitCost is the estimated per-request cost derived for the 1k input + 1k
// output reference sample. Official-vendor sites use the official list
// price; NewAPI-family relays (whose cost basis is ModelRatio × $2 per 1M)
// use the ratio-derived rate when the catalog carries the ratio set — the
// ratio set is what such upstreams actually bill, so it is a better
// cold-start estimate than the official list price. A ratio-derived rate
// also backstops metadata-only entries that list no USD price. source is
// SourceOfficial for vendor-hosted sites and SourceRelayEstimate for
// third-party relays, so the official list price is never presented as a
// real relay payment price. (nil, "") declines the query (unknown model, no
// catalog data).
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

	listCost := entry.ReferenceUnitCost()
	listUsable := isUsableAmount(listCost)
	ratioCost, ratioUsable := entry.ReferenceUnitCostFromRatios()

	source = SourceRelayEstimate
	official := p.classifySiteCached(siteID) == SiteClassOfficial
	if official {
		source = SourceOfficial
	}

	// Official sites: the vendor list price is the honest estimate; the
	// ratio rate only backstops a missing list price. Relay sites: the
	// ratio set (upstream's own billing basis) wins over the list price.
	var unit float64
	switch {
	case official && listUsable:
		unit = listCost
	case !official && ratioUsable:
		unit = ratioCost
	case listUsable:
		unit = listCost
	case ratioUsable:
		unit = ratioCost
	default:
		return nil, ""
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
