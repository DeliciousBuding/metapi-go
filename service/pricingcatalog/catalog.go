package pricingcatalog

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
)

// DefaultCatalogURL is the models.dev official pricing dataset endpoint.
const DefaultCatalogURL = "https://models.dev/api.json"

// CatalogSource labels the provenance of a resolved catalog unit cost. The
// literals MUST stay in sync with routing.CatalogSourceOfficial /
// routing.CatalogSourceRelayEstimate (asserted by a routing package test).
const (
	// SourceOfficial marks a models.dev official vendor list price.
	SourceOfficial = "catalog"
	// SourceRelayEstimate marks an official list price used only as an
	// estimate for a third-party relay site — never a real payment price.
	SourceRelayEstimate = "catalog_estimate"
)

// CatalogEntry is one model's official pricing row. Rates are USD per 1M
// tokens (models.dev cost unit).
type CatalogEntry struct {
	// ModelID is the normalized (lowercase, provider prefix stripped)
	// models.dev model id, e.g. "gpt-4o" or "claude-sonnet-4-20250514".
	ModelID string
	// Provider is the models.dev provider slug, e.g. "openai".
	Provider string
	// InputPerMillion / OutputPerMillion are USD per 1M tokens.
	InputPerMillion  float64
	OutputPerMillion float64
	// CacheReadPerMillion / CacheWritePerMillion are optional prompt-cache
	// rates; nil when the catalog omits them.
	CacheReadPerMillion  *float64
	CacheWritePerMillion *float64
	// ReleaseDate is the models.dev release_date (ISO), used to pick the
	// latest snapshot when a Claude family alias is ambiguous.
	ReleaseDate string
	// Aliased is true when the entry was resolved through a Claude family
	// alias (e.g. "claude-3-5-sonnet" → latest dated snapshot) instead of an
	// exact model id.
	Aliased bool
}

// ReferenceUnitCost converts a catalog entry into the per-request unit cost
// consumed by routing.EffectiveUnitCost, using the same reference sample as
// routing.DefaultPriceCompareSampleUsage (1k input + 1k output tokens).
// Catalog rates are USD/1M, so the sample cost is (input + output) / 1000.
// Cache rates do not contribute: the reference sample carries no cache tokens.
func (e CatalogEntry) ReferenceUnitCost() float64 {
	return (e.InputPerMillion + e.OutputPerMillion) / 1000
}

// CatalogSnapshot is an immutable parsed catalog. Route selection reads it
// lock-free through an atomic pointer, so lookups never block on network I/O.
type CatalogSnapshot struct {
	models        map[string]CatalogEntry
	claudeAliases map[string]CatalogEntry
	// Source describes where the snapshot came from ("models.dev api.json"
	// or the built-in preset table). Diagnostic only.
	Source    string
	FetchedAt time.Time
}

// NewCatalogSnapshot returns an empty snapshot ready for entries.
func NewCatalogSnapshot() *CatalogSnapshot {
	return &CatalogSnapshot{
		models:        make(map[string]CatalogEntry),
		claudeAliases: make(map[string]CatalogEntry),
	}
}

// Len returns the number of exact catalog entries.
func (s *CatalogSnapshot) Len() int {
	if s == nil {
		return 0
	}
	return len(s.models)
}

// Lookup resolves a model name to a catalog entry. Query names are lowercased,
// provider prefixes ("anthropic/claude-...") stripped, and Claude dot notation
// ("claude-3.7-sonnet") normalized to the catalog dash form ("claude-3-7-sonnet").
// Claude family aliases without a date suffix resolve to the latest dated
// snapshot. Dots are only normalized for Claude-family names so real dotted
// model ids like "gpt-4.1" stay intact.
func (s *CatalogSnapshot) Lookup(modelName string) (CatalogEntry, bool) {
	if s == nil {
		return CatalogEntry{}, false
	}
	name := normalizeQueryModelName(modelName)
	if name == "" {
		return CatalogEntry{}, false
	}
	if entry, ok := s.models[name]; ok {
		return entry, true
	}
	if entry, ok := s.claudeAliases[name]; ok {
		entry.Aliased = true
		return entry, true
	}
	return CatalogEntry{}, false
}

// modelsDevCatalog mirrors the models.dev api.json wire shape:
// {provider: {models: {id: {cost: {input, output, cache_read, cache_write}}}}}.
type modelsDevCatalog struct {
	Providers map[string]modelsDevProvider
}

type modelsDevProvider struct {
	ID     string                    `json:"id"`
	Models map[string]modelsDevModel `json:"models"`
}

type modelsDevModel struct {
	ID          string        `json:"id"`
	ReleaseDate string        `json:"release_date"`
	Cost        modelsDevCost `json:"cost"`
}

type modelsDevCost struct {
	Input      *float64 `json:"input"`
	Output     *float64 `json:"output"`
	CacheRead  *float64 `json:"cache_read"`
	CacheWrite *float64 `json:"cache_write"`
}

// ParseCatalog parses models.dev api.json bytes into a normalized catalog
// snapshot. Models without a usable input/output cost are skipped; malformed
// JSON is an error. The top-level provider map is polymorphic (models.dev
// historically shipped both arrays and objects there), so both shapes are
// accepted.
func ParseCatalog(data []byte) (*CatalogSnapshot, error) {
	var raw modelsDevCatalog
	if err := json.Unmarshal(data, &raw.Providers); err != nil {
		// models.dev changed the top-level container shape over time; accept
		// a bare array of providers as a defensive fallback.
		var rawList []modelsDevProvider
		if listErr := json.Unmarshal(data, &rawList); listErr != nil {
			return nil, fmt.Errorf("pricingcatalog: parse models.dev payload: %w", err)
		}
		if raw.Providers == nil {
			raw.Providers = make(map[string]modelsDevProvider, len(rawList))
		}
		for _, provider := range rawList {
			if _, exists := raw.Providers[provider.ID]; !exists {
				raw.Providers[provider.ID] = provider
			}
		}
	}

	snapshot := NewCatalogSnapshot()
	for providerSlug, provider := range raw.Providers {
		for _, model := range provider.Models {
			entry, ok := parseCatalogEntry(providerSlug, provider.ID, model)
			if !ok {
				continue
			}
			snapshot.addExact(entry)
		}
	}
	return snapshot, nil
}

func parseCatalogEntry(providerSlug, providerID string, model modelsDevModel) (CatalogEntry, bool) {
	if model.Cost.Input == nil || model.Cost.Output == nil {
		return CatalogEntry{}, false
	}
	input, output := *model.Cost.Input, *model.Cost.Output
	if input < 0 || output < 0 || math.IsNaN(input) || math.IsInf(input, 0) || math.IsNaN(output) || math.IsInf(output, 0) {
		return CatalogEntry{}, false
	}

	modelID := normalizeCatalogModelID(model.ID)
	if modelID == "" {
		return CatalogEntry{}, false
	}
	provider := strings.TrimSpace(providerID)
	if provider == "" {
		provider = strings.TrimSpace(providerSlug)
	}
	return CatalogEntry{
		ModelID:              modelID,
		Provider:             provider,
		InputPerMillion:      input,
		OutputPerMillion:     output,
		CacheReadPerMillion:  cloneCostField(model.Cost.CacheRead),
		CacheWritePerMillion: cloneCostField(model.Cost.CacheWrite),
		ReleaseDate:          strings.TrimSpace(model.ReleaseDate),
	}, true
}

func cloneCostField(value *float64) *float64 {
	if value == nil || math.IsNaN(*value) || math.IsInf(*value, 0) || *value < 0 {
		return nil
	}
	cloned := *value
	return &cloned
}

// addExact registers an exact model id and maintains the Claude family alias
// table (family → latest dated snapshot).
func (s *CatalogSnapshot) addExact(entry CatalogEntry) {
	if entry.ModelID == "" {
		return
	}
	s.models[entry.ModelID] = entry
	if !strings.HasPrefix(entry.ModelID, "claude-") {
		return
	}
	family := claudeFamilyOf(entry.ModelID)
	if family == "" || family == entry.ModelID {
		return
	}
	previous, exists := s.claudeAliases[family]
	if !exists || claudeEntryNewer(entry, previous) {
		s.claudeAliases[family] = entry
	}
}

// claudeFamilyOf strips the trailing dated suffix (-\d{8}) from a Claude
// model id: "claude-3-5-sonnet-20241022" → "claude-3-5-sonnet". Returns ""
// when the id has no dated suffix.
func claudeFamilyOf(modelID string) string {
	dash := strings.LastIndex(modelID, "-")
	if dash < 0 {
		return ""
	}
	suffix := modelID[dash+1:]
	if len(suffix) != 8 || !allDigits(suffix) {
		return ""
	}
	return modelID[:dash]
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// claudeEntryNewer orders dated Claude snapshots: later release date wins;
// ties break lexicographically on the model id (higher version suffixes sort
// later, e.g. "claude-sonnet-4-6" beats "claude-sonnet-4-5").
func claudeEntryNewer(candidate, current CatalogEntry) bool {
	if candidate.ReleaseDate != current.ReleaseDate {
		return candidate.ReleaseDate > current.ReleaseDate
	}
	return candidate.ModelID > current.ModelID
}

// normalizeCatalogModelID lowercases and strips a provider prefix from a
// models.dev model id ("deepseek/deepseek-chat" → "deepseek-chat").
func normalizeCatalogModelID(modelID string) string {
	return normalizeModelName(modelID)
}

// normalizeQueryModelName normalizes a looked-up model name: lowercase,
// provider prefix stripped, and Claude dot notation folded to dashes.
func normalizeQueryModelName(modelName string) string {
	name := normalizeModelName(modelName)
	if name == "" {
		return ""
	}
	if strings.Contains(name, "claude") {
		name = strings.ReplaceAll(name, ".", "-")
	}
	return name
}

func normalizeModelName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if slash := strings.LastIndex(name, "/"); slash >= 0 {
		name = name[slash+1:]
	}
	return name
}
