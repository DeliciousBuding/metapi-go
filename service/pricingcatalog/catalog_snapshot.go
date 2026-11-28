package pricingcatalog

import (
	"strings"
	"time"
)

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

// addExact registers an exact model id and maintains the Claude family alias
// table (family → latest dated snapshot).
func (s *CatalogSnapshot) addExact(entry CatalogEntry) {
	if entry.ModelID == "" {
		return
	}
	s.models[entry.ModelID] = entry
	s.addAlias(entry)
}

// addAlias maintains the Claude family alias table for an already-registered
// exact entry. Kept separate from addExact so merged snapshots can rebuild
// their alias table without re-inserting models.
func (s *CatalogSnapshot) addAlias(entry CatalogEntry) {
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
