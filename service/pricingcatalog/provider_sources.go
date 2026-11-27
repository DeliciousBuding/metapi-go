package pricingcatalog

import (
	"strconv"
	"strings"
	"time"
)

// SourceKind selects the wire-shape parser for a catalog source.
type SourceKind string

const (
	// SourceKindAuto tries the llm-metadata parser first (superset shape),
	// then the newapi ratio parser, then the models.dev parser, so official
	// datasets, ratio payloads and custom mirrors of either shape work
	// without URL sniffing.
	SourceKindAuto SourceKind = "auto"
	// SourceKindLLMMetadata forces the llm-metadata all.json parser.
	SourceKindLLMMetadata SourceKind = "llm-metadata"
	// SourceKindModelsDev forces the models.dev api.json parser.
	SourceKindModelsDev SourceKind = "models.dev"
	// SourceKindNewAPIRatios forces the llm-metadata /api/newapi ratio
	// parser (ratio_config-v1-base.json and models.json shapes).
	SourceKindNewAPIRatios SourceKind = "newapi-ratios"
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

// SetSources replaces the fetch source list. Sources are fetched in order
// and merged first-wins. Runtime state for removed sources is dropped;
// retained sources keep their last-good snapshot and status. An empty list
// falls back to the legacy single FetchURL source.
func (p *Provider) SetSources(specs []SourceSpec) {
	p.sourcesMu.Lock()
	defer p.sourcesMu.Unlock()

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
