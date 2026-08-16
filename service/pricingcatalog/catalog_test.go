package pricingcatalog

import (
	"testing"
)

const sampleCatalogJSON = `{
	"openai": {
		"id": "openai",
		"models": {
			"gpt-4o": {
				"id": "gpt-4o",
				"cost": {"input": 2.5, "output": 10, "cache_read": 1.25}
			},
			"gpt-4o-mini": {
				"id": "gpt-4o-mini",
				"cost": {"input": 0.15, "output": 0.6, "cache_read": 0.075}
			},
			"gpt-image-1.5": {
				"id": "gpt-image-1.5",
				"cost": {}
			}
		}
	},
	"anthropic": {
		"id": "anthropic",
		"models": {
			"claude-3-5-sonnet-20240620": {
				"id": "claude-3-5-sonnet-20240620",
				"release_date": "2024-06-20",
				"cost": {"input": 3, "output": 15}
			},
			"claude-3-5-sonnet-20241022": {
				"id": "claude-3-5-sonnet-20241022",
				"release_date": "2024-10-22",
				"cost": {"input": 3, "output": 15, "cache_read": 0.3, "cache_write": 3.75}
			}
		}
	},
	"deepseek": {
		"id": "deepseek",
		"models": {
			"deepseek/deepseek-chat": {
				"id": "deepseek/deepseek-chat",
				"cost": {"input": 0.14, "output": 0.28}
			}
		}
	},
	"broken": {
		"id": "broken",
		"models": {
			"negative-cost": {
				"id": "negative-cost",
				"cost": {"input": -1, "output": 2}
			}
		}
	}
}`

func TestParseCatalog_NormalizesAndFilters(t *testing.T) {
	snapshot, err := ParseCatalog([]byte(sampleCatalogJSON))
	if err != nil {
		t.Fatalf("ParseCatalog: %v", err)
	}

	if _, ok := snapshot.Lookup("gpt-image-1.5"); ok {
		t.Error("model without input/output cost must be skipped")
	}
	if _, ok := snapshot.Lookup("negative-cost"); ok {
		t.Error("model with negative cost must be skipped")
	}
	if snapshot.Len() != 5 {
		t.Errorf("Len() = %d, want 5 (2 openai + 2 anthropic + 1 deepseek)", snapshot.Len())
	}
}

func TestCatalogSnapshot_LookupExactAndNormalized(t *testing.T) {
	snapshot, err := ParseCatalog([]byte(sampleCatalogJSON))
	if err != nil {
		t.Fatalf("ParseCatalog: %v", err)
	}

	entry, ok := snapshot.Lookup("gpt-4o")
	if !ok {
		t.Fatal("gpt-4o lookup failed")
	}
	if entry.Provider != "openai" || entry.InputPerMillion != 2.5 || entry.OutputPerMillion != 10 {
		t.Fatalf("unexpected entry: %+v", entry)
	}
	if entry.CacheReadPerMillion == nil || *entry.CacheReadPerMillion != 1.25 {
		t.Errorf("cache_read = %v, want 1.25", entry.CacheReadPerMillion)
	}
	if entry.CacheWritePerMillion != nil {
		t.Errorf("cache_write = %v, want nil when absent", *entry.CacheWritePerMillion)
	}

	// Case-insensitive lookup.
	if _, ok := snapshot.Lookup("GPT-4O"); !ok {
		t.Error("case-insensitive lookup failed")
	}

	// Provider-prefixed model id stored in the catalog is indexed bare.
	entry, ok = snapshot.Lookup("deepseek-chat")
	if !ok || entry.ModelID != "deepseek-chat" {
		t.Fatalf("provider-prefix stripping failed: %+v ok=%v", entry, ok)
	}
	// Provider-prefixed query name is also normalized.
	if _, ok := snapshot.Lookup("deepseek/deepseek-chat"); !ok {
		t.Error("provider-prefixed query name lookup failed")
	}
}

func TestCatalogSnapshot_LookupClaudeAliases(t *testing.T) {
	snapshot, err := ParseCatalog([]byte(sampleCatalogJSON))
	if err != nil {
		t.Fatalf("ParseCatalog: %v", err)
	}

	// Exact dated id resolves directly, without the alias flag.
	entry, ok := snapshot.Lookup("claude-3-5-sonnet-20241022")
	if !ok || entry.Aliased {
		t.Fatalf("exact dated lookup: ok=%v aliased=%v", ok, entry.Aliased)
	}

	// Family alias resolves to the LATEST dated snapshot.
	entry, ok = snapshot.Lookup("claude-3-5-sonnet")
	if !ok {
		t.Fatal("family alias lookup failed")
	}
	if !entry.Aliased {
		t.Error("family alias resolution must be flagged as aliased")
	}
	if entry.ModelID != "claude-3-5-sonnet-20241022" {
		t.Errorf("alias resolved to %q, want latest claude-3-5-sonnet-20241022", entry.ModelID)
	}

	// Dot notation normalized to dashes for Claude-family names.
	if _, ok := snapshot.Lookup("claude-3.5-sonnet"); !ok {
		t.Error("claude dot notation normalization failed")
	}
	// Anthropic-prefixed query works too.
	if _, ok := snapshot.Lookup("anthropic/claude-3-5-sonnet"); !ok {
		t.Error("anthropic-prefixed family lookup failed")
	}
}

func TestCatalogEntry_ReferenceUnitCost(t *testing.T) {
	entry := CatalogEntry{InputPerMillion: 2.5, OutputPerMillion: 10}
	got := entry.ReferenceUnitCost()
	want := 0.0125 // (2.5 + 10) / 1000 for the 1k+1k reference sample
	if got != want {
		t.Errorf("ReferenceUnitCost() = %f, want %f", got, want)
	}
}

func TestParseCatalog_MalformedJSON(t *testing.T) {
	if _, err := ParseCatalog([]byte(`{"openai": `)); err == nil {
		t.Error("malformed JSON must error")
	}
}

func TestParseCatalog_UnknownModelLookup(t *testing.T) {
	snapshot, err := ParseCatalog([]byte(sampleCatalogJSON))
	if err != nil {
		t.Fatalf("ParseCatalog: %v", err)
	}
	if _, ok := snapshot.Lookup("definitely-not-a-model"); ok {
		t.Error("unknown model must not resolve")
	}
	if _, ok := snapshot.Lookup(""); ok {
		t.Error("empty model name must not resolve")
	}
}

func TestNewPresetSnapshot_ServesFallback(t *testing.T) {
	snapshot := NewPresetSnapshot()
	if snapshot.Len() == 0 {
		t.Fatal("preset snapshot must not be empty")
	}
	if snapshot.Source != PresetSnapshotSource {
		t.Errorf("Source = %q, want %q", snapshot.Source, PresetSnapshotSource)
	}
	entry, ok := snapshot.Lookup("gpt-4o")
	if !ok {
		t.Fatal("preset gpt-4o lookup failed")
	}
	if entry.ReferenceUnitCost() <= 0 {
		t.Errorf("preset reference cost must be positive, got %f", entry.ReferenceUnitCost())
	}
}
