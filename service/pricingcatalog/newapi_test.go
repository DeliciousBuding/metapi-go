package pricingcatalog

import (
	"testing"
)

// sampleNewAPIConfig is a trimmed llm-metadata ratio_config-v1-base.json
// payload: per-model ratio maps plus tiered billing metadata.
const sampleNewAPIConfig = `{
  "data": {
    "billing_expr": {
      "gemini-2.5-pro": "len <= 200000 ? tier(\"0_200k\", p * 1.25 + c * 10 + cr * 0.125) : tier(\"200k_plus\", p * 2.5 + c * 15 + cr * 0.25)"
    },
    "billing_mode": {"gemini-2.5-pro": "tiered_expr"},
    "cache_ratio": {"gemini-2.5-pro": 0.1, "MiniMax-M3": 0.06, "gpt-5.6": 2.5},
    "completion_ratio": {"gemini-2.5-pro": 8, "gpt-5.6": 6},
    "model_price": {},
    "model_ratio": {"gemini-2.5-pro": 1.25, "gpt-5.6": 5, "MiniMax-M3": 0.3}
  },
  "message": "",
  "success": true
}`

// sampleNewAPIModels is a trimmed llm-metadata models.json payload: one row
// with USD prices + ratios + vendor, one ratio-only row, one empty row that
// must be skipped.
const sampleNewAPIModels = `{
  "data": [
    {
      "description": "Flagship high-reasoning generation model",
      "endpoints": null,
      "icon": "OpenAI.Color",
      "model_name": "gpt-5.6",
      "name_rule": 0,
      "price_per_m_cache_read": 25,
      "price_per_m_cache_write": null,
      "price_per_m_input": 10,
      "price_per_m_output": 60,
      "ratio_cache": 2.5,
      "ratio_completion": 6,
      "ratio_model": 5,
      "status": 1,
      "tags": "Vision,128K",
      "vendor_name": "OpenAI"
    },
    {
      "description": "MiniMax-tiered model",
      "model_name": "MiniMax-M3",
      "status": 1,
      "ratio_model": 0.3,
      "ratio_completion": 4,
      "ratio_cache": 0.06,
      "price_per_m_input": null,
      "vendor_name": "MiniMax"
    },
    {
      "description": "No pricing fields at all",
      "model_name": "no-data-row",
      "status": 1,
      "vendor_name": "Ghost"
    }
  ],
  "message": "",
  "success": true
}`

func TestParseNewAPIRatios_ConfigShape(t *testing.T) {
	snapshot, err := ParseNewAPIRatios([]byte(sampleNewAPIConfig))
	if err != nil {
		t.Fatalf("ParseNewAPIRatios: %v", err)
	}
	if snapshot.Len() != 3 {
		t.Fatalf("Len = %d, want 3", snapshot.Len())
	}

	gemini, ok := snapshot.Lookup("gemini-2.5-pro")
	if !ok {
		t.Fatal("gemini-2.5-pro must be present")
	}
	if !gemini.HasRatioData() {
		t.Fatal("gemini-2.5-pro must carry ratio data")
	}
	if gemini.ModelRatio == nil || *gemini.ModelRatio != 1.25 {
		t.Errorf("model_ratio = %v, want 1.25", gemini.ModelRatio)
	}
	if gemini.CompletionRatio == nil || *gemini.CompletionRatio != 8 {
		t.Errorf("completion_ratio = %v, want 8", gemini.CompletionRatio)
	}
	if gemini.CacheRatio == nil || *gemini.CacheRatio != 0.1 {
		t.Errorf("cache_ratio = %v, want 0.1", gemini.CacheRatio)
	}
	if gemini.BillingMode != "tiered_expr" {
		t.Errorf("billing_mode = %q, want tiered_expr", gemini.BillingMode)
	}
	if gemini.BillingExpr == "" {
		t.Error("billing_expr must be carried")
	}
	// Config shape carries no vendor, no USD price.
	if gemini.InputPerMillion != 0 || gemini.Provider != "" {
		t.Errorf("config row carries price/provider = %v/%q, want zero/empty", gemini.InputPerMillion, gemini.Provider)
	}

	// Cost estimate from ratios: (1.25×2 + 1.25×8×2)/1000 = (2.5 + 20)/1000.
	cost, ok := gemini.ReferenceUnitCostFromRatios()
	if !ok || cost != 0.0225 {
		t.Errorf("ratio reference cost = %v/%v, want 0.0225/true", cost, ok)
	}

	// Normalization: "MiniMax-M3" stays case-folded; "gpt-5.6" dots stay.
	minimax, ok := snapshot.Lookup("minimax-m3")
	if !ok {
		t.Fatal("minimax-m3 must be present (case-folded)")
	}
	if !minimax.HasRatioData() || *minimax.ModelRatio != 0.3 {
		t.Errorf("miniMax ratio = %v, want 0.3", minimax.ModelRatio)
	}
	if _, ok := snapshot.Lookup("gpt-5.6"); !ok {
		t.Error("gpt-5.6 (dotted id) must be present")
	}
}

func TestParseNewAPIRatios_ModelsShape(t *testing.T) {
	snapshot, err := ParseNewAPIRatios([]byte(sampleNewAPIModels))
	if err != nil {
		t.Fatalf("ParseNewAPIRatios: %v", err)
	}
	if snapshot.Len() != 2 {
		t.Fatalf("Len = %d, want 2 (row without pricing skipped)", snapshot.Len())
	}

	gpt, ok := snapshot.Lookup("gpt-5.6")
	if !ok {
		t.Fatal("gpt-5.6 must be present")
	}
	if gpt.Provider != "OpenAI" || gpt.Description == "" || len(gpt.Tags) != 2 {
		t.Errorf("row fields = %+v, want vendor OpenAI + description + 2 tags", gpt)
	}
	if gpt.InputPerMillion != 10 || gpt.OutputPerMillion != 60 {
		t.Errorf("usd price = %v/%v, want 10/60", gpt.InputPerMillion, gpt.OutputPerMillion)
	}
	if gpt.CacheReadPerMillion == nil || *gpt.CacheReadPerMillion != 25 {
		t.Errorf("cache price = %v, want 25", gpt.CacheReadPerMillion)
	}
	if gpt.ModelRatio == nil || *gpt.ModelRatio != 5 || gpt.CompletionRatio == nil || *gpt.CompletionRatio != 6 {
		t.Errorf("ratios = %v/%v, want 5/6", gpt.ModelRatio, gpt.CompletionRatio)
	}

	// Ratio-only row: vendor attributed, no USD price.
	minimax, ok := snapshot.Lookup("minimax-m3")
	if !ok {
		t.Fatal("MiniMax-M3 must be present")
	}
	if minimax.InputPerMillion != 0 || !minimax.HasRatioData() {
		t.Errorf("ratio-only row = %+v, want ratios without price", minimax)
	}
}

func TestParseNewAPIRatios_Malformed(t *testing.T) {
	if _, err := ParseNewAPIRatios([]byte(`{"data": "not-an-object-or-array"}`)); err == nil {
		t.Error("string data must error")
	}
	if _, err := ParseNewAPIRatios([]byte(`{"success": true}`)); err == nil {
		t.Error("missing data must error")
	}
	if _, err := ParseNewAPIRatios([]byte(`not json`)); err == nil {
		t.Error("malformed json must error")
	}
}

func TestParseNewAPIRatios_InvalidRatiosRejected(t *testing.T) {
	negative := `{"data": {"model_ratio": {"bad-model": -1, "good-model": 1.5}}}`
	snapshot, err := ParseNewAPIRatios([]byte(negative))
	if err != nil {
		t.Fatalf("ParseNewAPIRatios: %v", err)
	}
	if _, ok := snapshot.Lookup("bad-model"); ok {
		t.Error("negative model_ratio must be skipped")
	}
	if _, ok := snapshot.Lookup("good-model"); !ok {
		t.Error("valid model_ratio must be kept")
	}
}

func TestMergeSnapshots_RatioAugmentation(t *testing.T) {
	// Base: llm-metadata all.json entry with USD price + metadata.
	base, err := ParseLLMMetadata([]byte(`{
	  "openai": {"id": "openai", "models": {
	    "gpt-5.6": {
	      "id": "gpt-5.6", "description": "Flagship high-reasoning generation model",
	      "cost": {"input": 10, "output": 60, "cache_read": 25},
	      "modalities": {"input": ["text", "image"], "output": ["text"]}
	    },
	    "metadata-only": {"id": "metadata-only", "description": "No price row"}
	  }}
	}`))
	if err != nil {
		t.Fatalf("ParseLLMMetadata: %v", err)
	}
	ratios, err := ParseNewAPIRatios([]byte(sampleNewAPIConfig))
	if err != nil {
		t.Fatalf("ParseNewAPIRatios: %v", err)
	}

	merged := MergeSnapshots([]*CatalogSnapshot{base, ratios})

	// gpt-5.6: ratio data gained, USD price kept (later source must not clobber).
	entry, ok := merged.Lookup("gpt-5.6")
	if !ok {
		t.Fatal("gpt-5.6 must be present")
	}
	if entry.InputPerMillion != 10 || entry.OutputPerMillion != 60 {
		t.Errorf("usd price lost = %v/%v, want 10/60", entry.InputPerMillion, entry.OutputPerMillion)
	}
	if entry.ModelRatio == nil || *entry.ModelRatio != 5 || entry.CompletionRatio == nil || *entry.CompletionRatio != 6 {
		t.Errorf("ratio not augmented = %+v", entry)
	}
	if entry.CacheRatio == nil || *entry.CacheRatio != 2.5 {
		t.Errorf("cache ratio not augmented = %v", entry.CacheRatio)
	}
	if entry.Modalities == nil || len(entry.Modalities) != 1 {
		t.Errorf("modalities clobbered = %v", entry.Modalities)
	}

	// gemini-2.5-pro: ratio-only entry enters the snapshot standalone.
	gemini, ok := merged.Lookup("gemini-2.5-pro")
	if !ok || gemini.ModelRatio == nil || *gemini.ModelRatio != 1.25 {
		t.Fatalf("gemini-2.5-pro = %+v ok=%v, want augmented ratio entry", gemini, ok)
	}
	if gemini.BillingMode != "tiered_expr" {
		t.Errorf("billing_mode = %q, want tiered_expr", gemini.BillingMode)
	}

	// metadata-only: no ratio, no price → stays metadata-only.
	meta, ok := merged.Lookup("metadata-only")
	if !ok || meta.HasRatioData() || meta.InputPerMillion != 0 {
		t.Errorf("metadata-only = %+v, want untouched", meta)
	}
}

func TestMergeSnapshots_PriceFillFromModelsRows(t *testing.T) {
	// Base snapshot carries gpt-5.6 metadata but no cost; the models.json
	// row supplies the USD price (and ratios) without clobbering metadata.
	lowBase, err := ParseLLMMetadata([]byte(`{
	  "openai": {"id": "openai", "models": {
	    "gpt-5.6": {"id": "gpt-5.6", "description": "no price row"}
	  }}
	}`))
	if err != nil {
		t.Fatalf("ParseLLMMetadata: %v", err)
	}
	rows, err := ParseNewAPIRatios([]byte(sampleNewAPIModels))
	if err != nil {
		t.Fatalf("ParseNewAPIRatios: %v", err)
	}
	merged := MergeSnapshots([]*CatalogSnapshot{lowBase, rows})
	entry, ok := merged.Lookup("gpt-5.6")
	if !ok {
		t.Fatal("gpt-5.6 must be present")
	}
	if entry.InputPerMillion != 10 || entry.OutputPerMillion != 60 {
		t.Errorf("price not filled = %v/%v, want 10/60", entry.InputPerMillion, entry.OutputPerMillion)
	}
	if entry.Description != "no price row" {
		t.Errorf("description clobbered = %q, want base description", entry.Description)
	}
}

func TestReferencePerMillionRates_ExplicitAndRatioDerived(t *testing.T) {
	// Explicit USD wins.
	explicit := CatalogEntry{InputPerMillion: 2.5, OutputPerMillion: 10}
	input, output, ok := explicit.ReferencePerMillionRates()
	if !ok || input != 2.5 || output != 10 {
		t.Errorf("explicit rates = %v/%v/%v, want 2.5/10/true", input, output, ok)
	}

	// Explicit zero-output embedding row keeps its zero.
	embedding := CatalogEntry{InputPerMillion: 0.13, OutputPerMillion: 0}
	input, output, ok = embedding.ReferencePerMillionRates()
	if !ok || input != 0.13 || output != 0 {
		t.Errorf("embedding rates = %v/%v/%v, want 0.13/0/true", input, output, ok)
	}

	// Ratio-derived fallback: 5×2 = 10 input, 5×6×2 = 60 output.
	ratio := 5.0
	completion := 6.0
	derived := CatalogEntry{ModelRatio: &ratio, CompletionRatio: &completion}
	input, output, ok = derived.ReferencePerMillionRates()
	if !ok || input != 10 || output != 60 {
		t.Errorf("ratio-derived rates = %v/%v/%v, want 10/60/true", input, output, ok)
	}

	// Nothing usable.
	if _, _, ok := (CatalogEntry{}).ReferencePerMillionRates(); ok {
		t.Error("empty entry must not yield rates")
	}
}
