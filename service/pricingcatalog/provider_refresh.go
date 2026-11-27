package pricingcatalog

import (
	"context"
	"fmt"
	"strings"
	"time"
)

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

	p.publishSnapshot(parts, names, len(specs) == 1 && specs[0].ID <= 0, report)
	p.logger.Info("pricingcatalog: catalog refreshed", "sources", strings.Join(names, ", "), "models", report.Models)
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

	report := &RefreshReport{}
	p.publishSnapshot(parts, names, len(p.sources) == 1 && p.sources[0].spec.ID <= 0, report)
	for i := range p.sources {
		report.Sources = append(report.Sources, p.sources[i].report())
	}
	return report, nil
}

func (p *Provider) publishSnapshot(parts []*CatalogSnapshot, names []string, anonymousSingleSource bool, report *RefreshReport) {
	merged := MergeSnapshots(parts)
	if anonymousSingleSource {
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
}
