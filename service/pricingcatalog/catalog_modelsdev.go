package pricingcatalog

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// DefaultCatalogURL is the models.dev official pricing dataset endpoint.
const DefaultCatalogURL = "https://models.dev/api.json"

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
		ListPriceKnown:       true,
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
