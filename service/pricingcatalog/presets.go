package pricingcatalog

import (
	"strings"
	"time"
)

// presetEntries is a small compile-time snapshot of models.dev official
// prices (USD per 1M tokens, base tier). It is ONLY used when the live
// catalog has never been fetched successfully; it is not a substitute for the
// periodic refresh, and it deliberately covers a handful of common models
// rather than pretending to be complete.
//
// Captured 2026-08-16 from https://models.dev/api.json.
var presetEntries = []CatalogEntry{
	{ModelID: "gpt-4o", Provider: "openai", InputPerMillion: 2.5, OutputPerMillion: 10, CacheReadPerMillion: floatPtr(1.25)},
	{ModelID: "gpt-4o-mini", Provider: "openai", InputPerMillion: 0.15, OutputPerMillion: 0.6, CacheReadPerMillion: floatPtr(0.075)},
	{ModelID: "gpt-3.5-turbo", Provider: "openai", InputPerMillion: 0.5, OutputPerMillion: 1.5},
	{ModelID: "gpt-5-nano", Provider: "openai", InputPerMillion: 0.05, OutputPerMillion: 0.4, CacheReadPerMillion: floatPtr(0.005)},
	{ModelID: "claude-sonnet-4-5", Provider: "anthropic", InputPerMillion: 3, OutputPerMillion: 15, CacheReadPerMillion: floatPtr(0.3), CacheWritePerMillion: floatPtr(3.75)},
	{ModelID: "claude-haiku-4-5", Provider: "anthropic", InputPerMillion: 1, OutputPerMillion: 5, CacheReadPerMillion: floatPtr(0.1), CacheWritePerMillion: floatPtr(1.25)},
	{ModelID: "claude-opus-4-5", Provider: "anthropic", InputPerMillion: 5, OutputPerMillion: 25, CacheReadPerMillion: floatPtr(0.5), CacheWritePerMillion: floatPtr(6.25)},
	{ModelID: "deepseek-chat", Provider: "deepseek", InputPerMillion: 0.14, OutputPerMillion: 0.28, CacheReadPerMillion: floatPtr(0.0028)},
	{ModelID: "deepseek-reasoner", Provider: "deepseek", InputPerMillion: 0.14, OutputPerMillion: 0.28, CacheReadPerMillion: floatPtr(0.0028)},
	{ModelID: "grok-4.5", Provider: "xai", InputPerMillion: 2, OutputPerMillion: 6, CacheReadPerMillion: floatPtr(0.3)},
	{ModelID: "gemini-flash-lite-latest", Provider: "google", InputPerMillion: 0.25, OutputPerMillion: 1.5, CacheReadPerMillion: floatPtr(0.025)},
}

func floatPtr(value float64) *float64 {
	return &value
}

// PresetSnapshotSource labels the compile-time fallback snapshot.
const PresetSnapshotSource = "built-in presets (models.dev snapshot 2026-08-16)"

// NewPresetSnapshot builds the fallback snapshot used while the live catalog
// has never loaded. Presets are clearly labeled as such in the snapshot
// Source field; nothing here claims to be the live official catalog.
func NewPresetSnapshot() *CatalogSnapshot {
	snapshot := NewCatalogSnapshot()
	snapshot.Source = PresetSnapshotSource
	snapshot.FetchedAt = time.Time{}
	for _, entry := range presetEntries {
		if strings.TrimSpace(entry.ModelID) == "" {
			continue
		}
		snapshot.addExact(entry)
	}
	return snapshot
}
