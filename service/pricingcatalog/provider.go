package pricingcatalog

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// maxCatalogPayloadBytes bounds the catalog payload read (the datasets are
// ~2 MB; the bound protects against an unexpectedly large response body).
const maxCatalogPayloadBytes = 32 << 20

// SourceKind selects the wire-shape parser for a catalog source.
type SourceKind string

const (
	// SourceKindAuto tries the llm-metadata parser first (superset shape)
	// and falls back to the models.dev parser, so both official datasets
	// and custom mirrors of either shape work without URL sniffing.
	SourceKindAuto SourceKind = "auto"
	// SourceKindLLMMetadata forces the llm-metadata all.json parser.
	SourceKindLLMMetadata SourceKind = "llm-metadata"
	// SourceKindModelsDev forces the models.dev api.json parser.
	SourceKindModelsDev SourceKind = "models.dev"
)

// SourceSpec describes one catalog source to fetch. Sources are fetched in
// list order and merged first-wins (earlier sources override later ones).
type SourceSpec struct {
	// ID is the DB registry id; 0 for non-persisted sources (tests).
	ID int64
	// Name is the operator-facing source name (e.g. "llm-metadata").
	Name string
	// URL is the dataset endpoint.
	URL string
	// Kind selects the parser; SourceKindAuto (zero value) auto-detects.
	Kind SourceKind
}

// SourceReport is the last-known sync outcome for one source.
type SourceReport struct {
	ID   int64
	Name string
	URL  string
	// ModelCount is the entry count of the source's last successful parse
	// (kept from the previous success when the last attempt failed).
	ModelCount int
	// LastSuccess is the last successful fetch+parse time (nil = never).
	LastSuccess *time.Time
	// LastError is the last fetch/parse error ("" = last attempt OK).
	LastError string
	// AttemptedAt is the last attempt time (success or failure).
	AttemptedAt time.Time
}

// RefreshReport summarizes one Refresh / SyncSource run.
type RefreshReport struct {
	// Sources carries the per-source outcome in fetch order.
	Sources []SourceReport
	// Models is the published snapshot entry count.
	Models int
	// Source is the published snapshot source label.
	Source string
	// FetchedAt is the published snapshot timestamp.
	FetchedAt time.Time
}

type sourceRuntime struct {
	spec        SourceSpec
	lastGood    *CatalogSnapshot
	lastSuccess *time.Time
	lastError   string
	attemptedAt time.Time
	// seededCount is the persisted last-known entry count from DB state; it
	// backs ModelCount until the first live parse in this process.
	seededCount int
}

func (s *sourceRuntime) report() SourceReport {
	count := s.seededCount
	if s.lastGood != nil {
		count = s.lastGood.Len()
	}
	return SourceReport{
		ID:          s.spec.ID,
		Name:        s.spec.Name,
		URL:         s.spec.URL,
		ModelCount:  count,
		LastSuccess: s.lastSuccess,
		LastError:   s.lastError,
		AttemptedAt: s.attemptedAt,
	}
}

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
	provider.SetSources(opts.Sources)
	provider.snapshot.Store(NewPresetSnapshot())
	return provider
}

// SetSources replaces the fetch source list. Sources are fetched in order
// and merged first-wins. Runtime state for removed sources is dropped;
// retained sources keep their last-good snapshot and status. An empty list
// falls back to the legacy single FetchURL source.
func (p *Provider) SetSources(specs []SourceSpec) {
	p.sourcesMu.Lock()
	defer p.sourcesMu.Unlock()

	kept := make(map[string]struct{}, len(specs))
	next := make([]sourceRuntime, 0, len(specs))
	for _, spec := range specs {
		spec.Name = strings.TrimSpace(spec.Name)
		spec.URL = strings.TrimSpace(spec.URL)
		if spec.URL == "" {
			continue
		}
		if spec.Name == "" {
			spec.Name = spec.URL
		}
		key := sourceKey(spec)
		kept[key] = struct{}{}
		runtime := sourceRuntime{spec: spec}
		for _, old := range p.sources {
			if sourceKey(old.spec) == key {
				runtime = old
				runtime.spec = spec // keep refreshed spec fields (URL/name edits)
				break
			}
		}
		next = append(next, runtime)
	}
	p.sources = next
}

// sourceKey identifies a source runtime across SetSources calls. DB-backed
// sources use their registry id; anonymous sources (tests) use their name.
func sourceKey(spec SourceSpec) string {
	if spec.ID > 0 {
		return "id:" + strconv.FormatInt(spec.ID, 10)
	}
	return "name:" + spec.Name
}

// SeedSourceStatus preloads a source's last-known status from persisted
// state (DB) so status queries reflect history before the first sync.
func (p *Provider) SeedSourceStatus(report SourceReport) {
	p.sourcesMu.Lock()
	defer p.sourcesMu.Unlock()
	for i := range p.sources {
		if p.sources[i].spec.ID == report.ID || (report.ID == 0 && p.sources[i].spec.Name == report.Name) {
			p.sources[i].lastSuccess = report.LastSuccess
			p.sources[i].lastError = report.LastError
			p.sources[i].seededCount = report.ModelCount
			if !report.AttemptedAt.IsZero() {
				p.sources[i].attemptedAt = report.AttemptedAt
			}
			return
		}
	}
}

// SourceStatuses returns the per-source status reports in fetch order.
func (p *Provider) SourceStatuses() []SourceReport {
	p.sourcesMu.RLock()
	defer p.sourcesMu.RUnlock()
	reports := make([]SourceReport, 0, len(p.sources))
	for i := range p.sources {
		reports = append(reports, p.sources[i].report())
	}
	return reports
}

func (p *Provider) sourceSpecs() []SourceSpec {
	p.sourcesMu.RLock()
	defer p.sourcesMu.RUnlock()
	specs := make([]SourceSpec, 0, len(p.sources))
	for _, runtime := range p.sources {
		specs = append(specs, runtime.spec)
	}
	if len(specs) == 0 {
		// Legacy single-URL mode.
		return []SourceSpec{{Name: "models.dev", URL: p.fetchURL, Kind: SourceKindAuto}}
	}
	return specs
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
		if _, err := p.Refresh(ctx); err != nil {
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
				_, _ = p.Refresh(ctx)
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

// Refresh fetches and parses every configured source, merges them
// first-wins, then atomically publishes the merged snapshot. The report
// carries per-source outcomes. Refresh fails only when no source succeeded
// or no source is configured; on any failure the current snapshot is kept
// (presets until the first success), so a flaky endpoint never erases a
// working catalog.
func (p *Provider) Refresh(ctx context.Context) (*RefreshReport, error) {
	specs := p.sourceSpecs()
	report := &RefreshReport{}

	parts := make([]*CatalogSnapshot, 0, len(specs))
	names := make([]string, 0, len(specs))
	anySuccess := false
	for _, spec := range specs {
		snapshot := p.fetchSource(ctx, spec)
		report.Sources = append(report.Sources, p.sourceReportAfter(spec))
		if snapshot != nil {
			anySuccess = true
			parts = append(parts, snapshot)
			names = append(names, spec.Name)
		}
	}

	if !anySuccess {
		if len(specs) == 0 {
			return report, fmt.Errorf("pricingcatalog: no catalog sources configured")
		}
		p.logger.Warn("pricingcatalog: all catalog sources failed; keeping current snapshot")
		return report, fmt.Errorf("pricingcatalog: all %d catalog source(s) failed", len(specs))
	}

	merged := MergeSnapshots(parts)
	if len(specs) == 1 && specs[0].ID <= 0 {
		// Anonymous single-source mode keeps the legacy diagnostic label.
		merged.Source = "models.dev api.json"
	} else {
		merged.Source = "catalog sources: " + strings.Join(names, ", ")
	}
	merged.FetchedAt = time.Now()
	p.snapshot.Store(merged)
	report.Models = merged.Len()
	report.Source = merged.Source
	report.FetchedAt = merged.FetchedAt
	p.logger.Info("pricingcatalog: catalog refreshed", "sources", strings.Join(names, ", "), "models", merged.Len())
	return report, nil
}

// SyncSource fetches and parses a single source (by registry id, or by name
// when id <= 0), re-merges all sources' last-good snapshots, and publishes.
// Other sources keep contributing their last-good data, so a single-source
// sync never erases the rest of the catalog. Fails when the source is
// unknown or its fetch fails (the current snapshot is kept in both cases).
func (p *Provider) SyncSource(ctx context.Context, sourceID int64) (*RefreshReport, error) {
	p.sourcesMu.RLock()
	index := -1
	for i := range p.sources {
		if sourceID > 0 && p.sources[i].spec.ID == sourceID {
			index = i
			break
		}
		if sourceID <= 0 && p.sources[i].spec.ID <= 0 && p.sources[i].spec.Name != "" {
			index = i
			break
		}
	}
	p.sourcesMu.RUnlock()
	if index < 0 {
		return nil, fmt.Errorf("pricingcatalog: unknown catalog source %d", sourceID)
	}

	spec := p.sourceSpecs()[index]
	snapshot := p.fetchSource(ctx, spec)
	if snapshot == nil {
		return nil, fmt.Errorf("pricingcatalog: source %q sync failed", spec.Name)
	}
	return p.publishMerged(spec.Name)
}

// publishMerged re-merges all sources' last-good snapshots in order and
// publishes the result. Used after a single-source sync so the published
// snapshot always reflects the full ordered merge.
func (p *Provider) publishMerged(syncedName string) (*RefreshReport, error) {
	p.sourcesMu.RLock()
	defer p.sourcesMu.RUnlock()

	parts := make([]*CatalogSnapshot, 0, len(p.sources))
	names := make([]string, 0, len(p.sources))
	for i := range p.sources {
		if p.sources[i].lastGood == nil {
			continue
		}
		parts = append(parts, p.sources[i].lastGood)
		names = append(names, p.sources[i].spec.Name)
	}
	if len(parts) == 0 {
		return nil, fmt.Errorf("pricingcatalog: source %q has no catalog data", syncedName)
	}

	merged := MergeSnapshots(parts)
	if len(p.sources) == 1 && p.sources[0].spec.ID <= 0 {
		merged.Source = "models.dev api.json"
	} else {
		merged.Source = "catalog sources: " + strings.Join(names, ", ")
	}
	merged.FetchedAt = time.Now()
	p.snapshot.Store(merged)

	report := &RefreshReport{
		Models:    merged.Len(),
		Source:    merged.Source,
		FetchedAt: merged.FetchedAt,
	}
	for i := range p.sources {
		report.Sources = append(report.Sources, p.sources[i].report())
	}
	return report, nil
}

// sourceReportAfter returns the runtime report for a source after a fetch
// attempt. Kept separate from SourceStatuses to avoid double locking.
func (p *Provider) sourceReportAfter(spec SourceSpec) SourceReport {
	p.sourcesMu.RLock()
	defer p.sourcesMu.RUnlock()
	for i := range p.sources {
		if sourceKey(p.sources[i].spec) == sourceKey(spec) {
			return p.sources[i].report()
		}
	}
	return SourceReport{ID: spec.ID, Name: spec.Name, URL: spec.URL}
}

// fetchSource fetches + parses one source and updates its runtime state.
// Returns nil on failure; the runtime keeps the previous last-good snapshot.
func (p *Provider) fetchSource(ctx context.Context, spec SourceSpec) *CatalogSnapshot {
	attemptedAt := time.Now()
	data, err := p.fetch(ctx, spec.URL)
	var snapshot *CatalogSnapshot
	if err == nil {
		snapshot, err = parseSourceData(spec.Kind, data)
	}

	p.sourcesMu.Lock()
	found := false
	for i := range p.sources {
		if sourceKey(p.sources[i].spec) == sourceKey(spec) {
			p.sources[i].attemptedAt = attemptedAt
			if err != nil {
				p.sources[i].lastError = err.Error()
				p.logger.Warn("pricingcatalog: source fetch failed", "source", spec.Name, "url", spec.URL, "error", err)
			} else {
				p.sources[i].lastGood = snapshot
				now := time.Now()
				p.sources[i].lastSuccess = &now
				p.sources[i].lastError = ""
			}
			found = true
			break
		}
	}
	p.sourcesMu.Unlock()
	// Anonymous single-source mode (legacy FetchURL) has no runtime slot:
	// treat a failed fetch as nil.
	if !found {
		if err != nil {
			p.logger.Warn("pricingcatalog: source fetch failed", "source", spec.Name, "url", spec.URL, "error", err)
		}
		return snapshot
	}
	if err != nil {
		return nil
	}
	return snapshot
}

// parseSourceData parses payload bytes with the source-kind-selected parser.
// SourceKindAuto tries llm-metadata (superset shape) first and falls back to
// the models.dev parser when the payload does not carry llm-metadata data.
func parseSourceData(kind SourceKind, data []byte) (*CatalogSnapshot, error) {
	switch kind {
	case SourceKindLLMMetadata:
		return ParseLLMMetadata(data)
	case SourceKindModelsDev:
		return ParseCatalog(data)
	default:
		snapshot, err := ParseLLMMetadata(data)
		if err == nil && snapshot.Len() > 0 {
			return snapshot, nil
		}
		return ParseCatalog(data)
	}
}

func (p *Provider) fetch(ctx context.Context, url string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("pricingcatalog: build request: %w", err)
	}
	response, err := p.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("pricingcatalog: fetch %s: %w", url, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pricingcatalog: fetch %s: unexpected status %d", url, response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxCatalogPayloadBytes))
	if err != nil {
		return nil, fmt.Errorf("pricingcatalog: read %s: %w", url, err)
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
