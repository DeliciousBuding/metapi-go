package pricingcatalog

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// DefaultLLMMetadataRatioURL is the llm-metadata NewAPI ratio set endpoint
// (same origin as DefaultLLMMetadataURL). It carries the canonical
// NewAPI-compatible ratio multipliers per model — the billing basis of
// NewAPI-family relays — plus tiered billing expressions for models with
// tiered pricing.
const DefaultLLMMetadataRatioURL = "https://basellm.github.io/llm-metadata/api/newapi/ratio_config-v1-base.json"

// NewAPIRatioBasePerMillion is the canonical NewAPI currency anchor:
// 1 ratio = $2 per 1M input tokens. Mirrors the llm-metadata newapi-builder
// constant (maxInput / 2, 基准: $2 per 1M tokens).
const NewAPIRatioBasePerMillion = 2.0

// newAPIPayload is the common envelope of both llm-metadata /api/newapi
// payloads: {data, message, success}.
type newAPIPayload struct {
	Data    json.RawMessage `json:"data"`
	Message string          `json:"message"`
	Success *bool           `json:"success"`
}

// newAPIRatioConfig mirrors ratio_config-v1-base.json's data object:
// per-model ratio multiplier maps plus tiered billing metadata.
type newAPIRatioConfig struct {
	ModelRatio      map[string]*float64 `json:"model_ratio"`
	CompletionRatio map[string]*float64 `json:"completion_ratio"`
	CacheRatio      map[string]*float64 `json:"cache_ratio"`
	BillingMode     map[string]string   `json:"billing_mode"`
	BillingExpr     map[string]string   `json:"billing_expr"`
	// ModelPrice is currently empty upstream; reserved for future use.
	ModelPrice map[string]json.RawMessage `json:"model_price"`
}

// newAPIModelRow mirrors one models.json data row: vendor-attributed model
// with USD per-1M prices and the same ratio multipliers.
type newAPIModelRow struct {
	ModelName           string   `json:"model_name"`
	VendorName          string   `json:"vendor_name"`
	PricePerMInput      *float64 `json:"price_per_m_input"`
	PricePerMOutput     *float64 `json:"price_per_m_output"`
	PricePerMCacheRead  *float64 `json:"price_per_m_cache_read"`
	PricePerMCacheWrite *float64 `json:"price_per_m_cache_write"`
	RatioModel          *float64 `json:"ratio_model"`
	RatioCompletion     *float64 `json:"ratio_completion"`
	RatioCache          *float64 `json:"ratio_cache"`
	Status              int      `json:"status"`
	Description         string   `json:"description"`
	Tags                string   `json:"tags"`
}

// ParseNewAPIRatios parses an llm-metadata /api/newapi payload into a
// normalized catalog snapshot. Both payload shapes are accepted:
//
//   - ratio_config-v1-base.json: {data: {model_ratio, completion_ratio,
//     cache_ratio, billing_mode, billing_expr}} — entries carry the ratio
//     set (no vendor, no USD price);
//   - models.json: {data: [{model_name, vendor_name, price_per_m_*,
//     ratio_*, ...}]} — entries carry ratios plus vendor-attributed USD
//     prices and the row's capability tags/description.
//
// Malformed JSON is an error; rows without any usable ratio or price data
// are skipped. Ratio-bearing entries merge into the catalog snapshot as
// augmentations (see MergeSnapshots): they never replace an earlier source's
// USD list price.
func ParseNewAPIRatios(data []byte) (*CatalogSnapshot, error) {
	var payload newAPIPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("pricingcatalog: parse newapi ratio payload: %w", err)
	}
	if len(payload.Data) == 0 || string(payload.Data) == "null" {
		return nil, fmt.Errorf("pricingcatalog: parse newapi ratio payload: missing data")
	}

	// Config shape: data is an object of ratio maps.
	var config newAPIRatioConfig
	if err := json.Unmarshal(payload.Data, &config); err == nil {
		return parseNewAPIRatioConfig(config)
	}

	// Models shape: data is an array of rows.
	var rows []newAPIModelRow
	if err := json.Unmarshal(payload.Data, &rows); err != nil {
		return nil, fmt.Errorf("pricingcatalog: parse newapi ratio payload: data is neither ratio config nor model rows: %w", err)
	}
	return parseNewAPIModelRows(rows)
}

func parseNewAPIRatioConfig(config newAPIRatioConfig) (*CatalogSnapshot, error) {
	snapshot := NewCatalogSnapshot()
	for name, ratio := range config.ModelRatio {
		entry, ok := newAPIEntry(name)
		if !ok {
			continue
		}
		entry.ModelRatio = validRatio(ratio)
		if entry.ModelRatio == nil {
			continue
		}
		entry.CompletionRatio = validRatio(config.CompletionRatio[name])
		entry.CacheRatio = validRatio(config.CacheRatio[name])
		entry.BillingMode = strings.TrimSpace(config.BillingMode[name])
		entry.BillingExpr = strings.TrimSpace(config.BillingExpr[name])
		snapshot.addExact(entry)
	}
	return snapshot, nil
}

func parseNewAPIModelRows(rows []newAPIModelRow) (*CatalogSnapshot, error) {
	snapshot := NewCatalogSnapshot()
	for _, row := range rows {
		if row.Status < 0 {
			// Upstream status semantics: 1 = active; negative values would
			// mark a disabled row. Keep explicit 0 rows (unknown/offline is
			// not evidence of deletion) and skip negative statuses.
			continue
		}
		entry, ok := newAPIEntry(row.ModelName)
		if !ok {
			continue
		}
		entry.Provider = strings.TrimSpace(row.VendorName)
		entry.ModelRatio = validRatio(row.RatioModel)
		entry.CompletionRatio = validRatio(row.RatioCompletion)
		entry.CacheRatio = validRatio(row.RatioCache)
		entry.Description = strings.TrimSpace(row.Description)
		entry.Tags = splitCommaTags(row.Tags)
		if input, inOK := validCostField(row.PricePerMInput); inOK {
			if output, outOK := validCostField(row.PricePerMOutput); outOK {
				entry.InputPerMillion = input
				entry.OutputPerMillion = output
			}
		}
		entry.CacheReadPerMillion = cloneCostField(row.PricePerMCacheRead)
		entry.CacheWritePerMillion = cloneCostField(row.PricePerMCacheWrite)
		if entry.ModelRatio == nil && entry.InputPerMillion <= 0 && entry.OutputPerMillion <= 0 {
			continue // nothing usable
		}
		snapshot.addExact(entry)
	}
	return snapshot, nil
}

// newAPIEntry builds a skeleton entry from a model name, normalized the same
// way as the other catalog parsers (lowercased, provider prefix stripped).
func newAPIEntry(name string) (CatalogEntry, bool) {
	modelID := normalizeCatalogModelID(name)
	if modelID == "" {
		return CatalogEntry{}, false
	}
	return CatalogEntry{ModelID: modelID}, true
}

// validRatio returns a non-negative finite ratio, nil otherwise. Unlike cost
// fields, an explicit 0 is a legitimate ratio only for cache; model and
// completion ratios are required to be positive by their callers' usage, but
// an explicit 0 from upstream is preserved here and the consumer decides
// (ReferenceUnitCostFromRatios requires a positive model ratio).
func validRatio(value *float64) *float64 {
	if value == nil || math.IsNaN(*value) || math.IsInf(*value, 0) || *value < 0 {
		return nil
	}
	cloned := *value
	return &cloned
}

// splitCommaTags splits the models.json comma-separated tags field
// ("Open Weights,128K" → ["Open Weights", "128K"]), trimming whitespace and
// dropping empties.
func splitCommaTags(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// isNewAPIPayload reports whether payload bytes look like an llm-metadata
// /api/newapi envelope (top-level data + success keys). Used by the auto
// parser to select the ratio parser before the generic shape attempts.
func isNewAPIPayload(data []byte) bool {
	var probe struct {
		Data    json.RawMessage `json:"data"`
		Success *bool           `json:"success"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return false
	}
	return probe.Success != nil && len(probe.Data) > 0 && string(probe.Data) != "null"
}
