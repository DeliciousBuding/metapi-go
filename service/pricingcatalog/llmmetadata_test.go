package pricingcatalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// sampleLLMMetadataJSON is a trimmed llm-metadata all.json-shaped fixture:
// two providers, one model with full metadata + cost, one metadata-only
// model (no cost), and one deprecated model with cache rates.
const sampleLLMMetadataJSON = `{
  "openai": {
    "id": "openai",
    "name": "OpenAI",
    "models": {
      "gpt-4o": {
        "attachment": true,
        "cost": {"input": 2.5, "output": 10, "cache_read": 1.25},
        "description": "Flagship multimodal model for chat and agents",
        "family": "gpt-4o",
        "id": "gpt-4o",
        "limit": {"context": 128000, "output": 16384},
        "modalities": {"input": ["text", "image"], "output": ["text"]},
        "name": "GPT-4o",
        "reasoning": false,
        "release_date": "2024-05-13",
        "last_updated": "2026-08-01",
        "structured_output": true,
        "tool_call": true
      },
      "o1-pro-preview": {
        "description": "Reasoning preview with no structured metadata",
        "id": "o1-pro-preview",
        "modalities": {"input": ["text"], "output": ["text"]},
        "reasoning": true
      }
    }
  },
  "google": {
    "id": "google",
    "name": "Google",
    "models": {
      "gemini-legacy-turbo": {
        "cost": {"input": 0.5, "output": 1.5, "cache_read": 0.05, "cache_write": 0.1},
        "description": "Deprecated legacy Gemini variant",
        "id": "gemini-legacy-turbo",
        "limit": {"context": 32768},
        "modalities": {"input": ["text", "image", "audio"], "output": ["text"]},
        "name": "Gemini Legacy Turbo",
        "release_date": "2024-02-15",
        "status": "deprecated",
        "tool_call": true
      }
    }
  }
}`

func TestParseLLMMetadata_FieldsAndMetadataOnly(t *testing.T) {
	snapshot, err := ParseLLMMetadata([]byte(sampleLLMMetadataJSON))
	if err != nil {
		t.Fatalf("ParseLLMMetadata: %v", err)
	}
	if snapshot.Len() != 3 {
		t.Fatalf("Len = %d, want 3", snapshot.Len())
	}

	gpt4o, ok := snapshot.Lookup("gpt-4o")
	if !ok {
		t.Fatal("gpt-4o must be present")
	}
	if gpt4o.InputPerMillion != 2.5 || gpt4o.OutputPerMillion != 10 {
		t.Errorf("gpt-4o cost = %v/%v, want 2.5/10", gpt4o.InputPerMillion, gpt4o.OutputPerMillion)
	}
	if gpt4o.CacheReadPerMillion == nil || *gpt4o.CacheReadPerMillion != 1.25 {
		t.Errorf("gpt-4o cache_read = %v, want 1.25", gpt4o.CacheReadPerMillion)
	}
	if gpt4o.Description == "" || gpt4o.DisplayName != "GPT-4o" || gpt4o.Family != "gpt-4o" {
		t.Errorf("gpt-4o metadata = %+v, want description/displayName/family set", gpt4o)
	}
	if gpt4o.ContextLimit == nil || *gpt4o.ContextLimit != 128000 {
		t.Errorf("gpt-4o context limit = %v, want 128000", gpt4o.ContextLimit)
	}
	if gpt4o.MaxOutputLimit == nil || *gpt4o.MaxOutputLimit != 16384 {
		t.Errorf("gpt-4o max output = %v, want 16384", gpt4o.MaxOutputLimit)
	}
	wantTags := []string{"tool_call", "attachment", "structured_output", "vision"}
	if len(gpt4o.Tags) != len(wantTags) {
		t.Errorf("gpt-4o tags = %v, want %v", gpt4o.Tags, wantTags)
	}
	if gpt4o.Status != "" {
		t.Errorf("gpt-4o status = %q, want empty (active)", gpt4o.Status)
	}
	if gpt4o.Provider != "openai" {
		t.Errorf("gpt-4o provider = %q, want openai", gpt4o.Provider)
	}

	// Metadata-only model (no cost) must still be present with zero pricing.
	o1, ok := snapshot.Lookup("o1-pro-preview")
	if !ok {
		t.Fatal("o1-pro-preview must be present (metadata-only)")
	}
	if o1.InputPerMillion != 0 || o1.OutputPerMillion != 0 {
		t.Errorf("o1 cost = %v/%v, want 0/0 (no cost in payload)", o1.InputPerMillion, o1.OutputPerMillion)
	}
	if o1.ReferenceUnitCost() != 0 {
		t.Errorf("o1 reference cost = %v, want 0 (routing declines)", o1.ReferenceUnitCost())
	}
	if len(o1.Tags) == 0 || o1.Tags[0] != "reasoning" {
		t.Errorf("o1 tags = %v, want [reasoning]", o1.Tags)
	}

	// Deprecated model carries status + cache rates.
	gemini, ok := snapshot.Lookup("gemini-legacy-turbo")
	if !ok {
		t.Fatal("gemini-legacy-turbo must be present")
	}
	if gemini.Status != "deprecated" {
		t.Errorf("gemini status = %q, want deprecated", gemini.Status)
	}
	if gemini.CacheReadPerMillion == nil || *gemini.CacheReadPerMillion != 0.05 {
		t.Errorf("gemini cache_read = %v, want 0.05", gemini.CacheReadPerMillion)
	}
	if gemini.CacheWritePerMillion == nil || *gemini.CacheWritePerMillion != 0.1 {
		t.Errorf("gemini cache_write = %v, want 0.1", gemini.CacheWritePerMillion)
	}
}

func TestParseLLMMetadata_AcceptsModelsDevShape(t *testing.T) {
	// The llm-metadata wire shape is a strict superset of models.dev
	// api.json; a models.dev payload must parse into priced entries.
	snapshot, err := ParseLLMMetadata([]byte(sampleCatalogJSON))
	if err != nil {
		t.Fatalf("ParseLLMMetadata on models.dev payload: %v", err)
	}
	entry, ok := snapshot.Lookup("gpt-4o")
	if !ok || entry.InputPerMillion != 2.5 || entry.OutputPerMillion != 10 {
		t.Errorf("models.dev payload through llm-metadata parser = %+v, want gpt-4o 2.5/10", entry)
	}
}

func TestParseLLMMetadata_Malformed(t *testing.T) {
	if _, err := ParseLLMMetadata([]byte(`{"openai": `)); err == nil {
		t.Fatal("malformed payload must error")
	}
}

func TestMergeSnapshots_FirstWins(t *testing.T) {
	primary, err := ParseLLMMetadata([]byte(sampleLLMMetadataJSON))
	if err != nil {
		t.Fatalf("primary: %v", err)
	}
	// Fallback source overrides gpt-4o description and adds a new model.
	fallback := NewCatalogSnapshot()
	fallback.addExact(CatalogEntry{
		ModelID:          "gpt-4o",
		Provider:         "openai",
		InputPerMillion:  99,
		OutputPerMillion: 99,
		Description:      "fallback description (must NOT win)",
	})
	fallback.addExact(CatalogEntry{
		ModelID:          "fallback-only-model",
		Provider:         "openai",
		InputPerMillion:  1,
		OutputPerMillion: 2,
	})

	merged := MergeSnapshots([]*CatalogSnapshot{primary, fallback})
	entry, ok := merged.Lookup("gpt-4o")
	if !ok {
		t.Fatal("gpt-4o missing from merge")
	}
	if entry.Description == "fallback description (must NOT win)" {
		t.Error("first-wins violated: fallback description overwrote primary")
	}
	if entry.InputPerMillion != 2.5 {
		t.Errorf("gpt-4o price = %v, want primary 2.5", entry.InputPerMillion)
	}
	if _, ok := merged.Lookup("fallback-only-model"); !ok {
		t.Error("fallback-only model missing from merge")
	}
	// Claude aliases must be rebuilt from merged entries.
	claudeSnap := NewCatalogSnapshot()
	claudeSnap.addExact(CatalogEntry{
		ModelID:          "claude-sonnet-4-20250514",
		Provider:         "anthropic",
		InputPerMillion:  3,
		OutputPerMillion: 15,
		ReleaseDate:      "2025-05-14",
	})
	merged = MergeSnapshots([]*CatalogSnapshot{claudeSnap, fallback})
	if _, ok := merged.Lookup("claude-sonnet-4"); !ok {
		t.Error("claude family alias missing after merge")
	}
}

func TestProvider_MultiSourceFirstWinsAndStatus(t *testing.T) {
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sampleLLMMetadataJSON))
	}))
	defer primary.Close()

	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// models.dev-shaped payload with a cheaper gpt-4o — must lose.
		_, _ = w.Write([]byte(`{"openai":{"id":"openai","models":{"gpt-4o":{"id":"gpt-4o","release_date":"2024-05-13","cost":{"input":0.01,"output":0.02}}}}}`))
	}))
	defer fallback.Close()

	provider := NewProvider(Options{
		Logger: testLogger(),
		Sources: []SourceSpec{
			{ID: 1, Name: "llm-metadata", URL: primary.URL, Kind: SourceKindAuto},
			{ID: 2, Name: "models.dev", URL: fallback.URL, Kind: SourceKindAuto},
		},
	})

	report, err := provider.Refresh(context.Background())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if len(report.Sources) != 2 {
		t.Fatalf("report sources = %d, want 2", len(report.Sources))
	}
	if report.Sources[0].ModelCount != 3 || report.Sources[0].LastSuccess == nil || report.Sources[0].LastError != "" {
		t.Errorf("primary report = %+v, want 3 models success", report.Sources[0])
	}
	if report.Sources[1].LastSuccess == nil {
		t.Errorf("fallback report = %+v, want success", report.Sources[1])
	}

	entry, ok := provider.Snapshot().Lookup("gpt-4o")
	if !ok {
		t.Fatal("gpt-4o missing from merged snapshot")
	}
	if entry.InputPerMillion != 2.5 {
		t.Errorf("gpt-4o price = %v, want 2.5 (first source wins)", entry.InputPerMillion)
	}
	if entry.Description == "" {
		t.Error("description must come from the primary llm-metadata source")
	}
	if provider.Snapshot().Source != "catalog sources: llm-metadata, models.dev" {
		t.Errorf("snapshot source = %q", provider.Snapshot().Source)
	}

	// Statuses survive a later full failure.
	fallback.Close()
	if _, err := provider.Refresh(context.Background()); err != nil {
		t.Fatalf("partial failure must not fail the whole refresh: %v", err)
	}
	statuses := provider.SourceStatuses()
	if len(statuses) != 2 || statuses[1].LastError == "" {
		t.Errorf("statuses = %+v, want fallback error recorded", statuses)
	}
	if statuses[1].ModelCount != 1 || statuses[1].LastSuccess == nil {
		t.Errorf("fallback status = %+v, want last-good count kept", statuses[1])
	}
	// The merged snapshot must still carry the fallback-only model.
	if _, ok := provider.Snapshot().Lookup("gpt-4o"); !ok {
		t.Error("snapshot lost primary data after partial failure")
	}
}

func TestProvider_SyncSourceReusesOtherSources(t *testing.T) {
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sampleLLMMetadataJSON))
	}))
	defer primary.Close()

	provider := NewProvider(Options{
		Logger:  testLogger(),
		Sources: []SourceSpec{{ID: 7, Name: "primary", URL: primary.URL, Kind: SourceKindAuto}},
	})
	if _, err := provider.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	// Single-source sync on an unknown id must fail.
	if _, err := provider.SyncSource(context.Background(), 99); err == nil {
		t.Fatal("unknown source id must error")
	}

	report, err := provider.SyncSource(context.Background(), 7)
	if err != nil {
		t.Fatalf("SyncSource: %v", err)
	}
	if report.Models != 3 {
		t.Errorf("sync report models = %d, want 3", report.Models)
	}
	if _, ok := provider.Snapshot().Lookup("gpt-4o"); !ok {
		t.Error("snapshot must still publish after single-source sync")
	}

	// Failed single-source sync keeps the published snapshot.
	primary.Close()
	if _, err := provider.SyncSource(context.Background(), 7); err == nil {
		t.Fatal("sync against dead source must error")
	}
	if _, ok := provider.Snapshot().Lookup("gpt-4o"); !ok {
		t.Error("snapshot lost after failed single-source sync")
	}
}

func TestProvider_SeedSourceStatusRestoresHistory(t *testing.T) {
	provider := NewProvider(Options{
		Logger:  testLogger(),
		Sources: []SourceSpec{{ID: 3, Name: "primary", URL: "http://127.0.0.1:1/x.json", Kind: SourceKindAuto}},
	})
	lastSuccess := time.Now().Add(-time.Hour)
	provider.SeedSourceStatus(SourceReport{
		ID:          3,
		Name:        "primary",
		URL:         "http://127.0.0.1:1/x.json",
		ModelCount:  42,
		LastSuccess: &lastSuccess,
		LastError:   "old error",
		AttemptedAt: lastSuccess,
	})
	statuses := provider.SourceStatuses()
	if len(statuses) != 1 {
		t.Fatalf("statuses = %d, want 1", len(statuses))
	}
	if statuses[0].ModelCount != 42 || statuses[0].LastSuccess == nil || !statuses[0].LastSuccess.Equal(lastSuccess) {
		t.Errorf("seeded status = %+v, want history restored", statuses[0])
	}
}
