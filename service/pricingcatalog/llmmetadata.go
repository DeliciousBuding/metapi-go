package pricingcatalog

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// DefaultLLMMetadataURL is the llm-metadata all.json dataset endpoint
// (basellm/llm-metadata GitHub Pages; daily rebuilt, native-provider
// filtered). It is the primary catalog source: a strict superset of the
// models.dev wire shape with description / tags / limits / modalities.
const DefaultLLMMetadataURL = "https://basellm.github.io/llm-metadata/api/all.json"

// llmMetadataCatalog mirrors the llm-metadata all.json wire shape:
// {providerId: {id, name, models: {modelId: {id, name, description, cost,
// limit, modalities, status, ...}}}}. It is a superset of the models.dev
// api.json shape, so models.dev payloads also unmarshal into it (the extra
// fields simply stay empty).
type llmMetadataCatalog map[string]llmMetadataProvider

type llmMetadataProvider struct {
	ID     string                      `json:"id"`
	Name   string                      `json:"name"`
	Models map[string]llmMetadataModel `json:"models"`
}

type llmMetadataModel struct {
	ID               string                `json:"id"`
	Name             string                `json:"name"`
	Family           string                `json:"family"`
	Description      string                `json:"description"`
	ReleaseDate      string                `json:"release_date"`
	LastUpdated      string                `json:"last_updated"`
	Status           string                `json:"status"`
	Reasoning        bool                  `json:"reasoning"`
	ToolCall         bool                  `json:"tool_call"`
	Attachment       bool                  `json:"attachment"`
	StructuredOutput bool                  `json:"structured_output"`
	Cost             llmMetadataCost       `json:"cost"`
	Limit            llmMetadataLimit      `json:"limit"`
	Modalities       llmMetadataModalities `json:"modalities"`
}

type llmMetadataCost struct {
	Input      *float64 `json:"input"`
	Output     *float64 `json:"output"`
	CacheRead  *float64 `json:"cache_read"`
	CacheWrite *float64 `json:"cache_write"`
}

type llmMetadataLimit struct {
	Context *int64 `json:"context"`
	Output  *int64 `json:"output"`
}

type llmMetadataModalities struct {
	Input  []string `json:"input"`
	Output []string `json:"output"`
}

// ParseLLMMetadata parses llm-metadata all.json bytes into a normalized
// catalog snapshot. Unlike the models.dev parser, models WITHOUT a usable
// input/output cost are still kept: they contribute description / tags /
// limits to the marketplace while their zero cost keeps them out of routing
// (ReferenceUnitCost is 0 and the resolver declines non-positive costs).
// Malformed JSON is an error.
func ParseLLMMetadata(data []byte) (*CatalogSnapshot, error) {
	var raw llmMetadataCatalog
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("pricingcatalog: parse llm-metadata payload: %w", err)
	}

	snapshot := NewCatalogSnapshot()
	for providerSlug, provider := range raw {
		if len(provider.Models) == 0 {
			continue
		}
		for _, model := range provider.Models {
			entry, ok := parseLLMMetadataEntry(providerSlug, provider.ID, model)
			if !ok {
				continue
			}
			snapshot.addExact(entry)
		}
	}
	return snapshot, nil
}

func parseLLMMetadataEntry(providerSlug, providerID string, model llmMetadataModel) (CatalogEntry, bool) {
	modelID := normalizeCatalogModelID(model.ID)
	if modelID == "" {
		return CatalogEntry{}, false
	}
	provider := strings.TrimSpace(providerID)
	if provider == "" {
		provider = strings.TrimSpace(providerSlug)
	}

	entry := CatalogEntry{
		ModelID:        modelID,
		Provider:       provider,
		ReleaseDate:    strings.TrimSpace(model.ReleaseDate),
		Description:    strings.TrimSpace(model.Description),
		DisplayName:    strings.TrimSpace(model.Name),
		Family:         strings.TrimSpace(model.Family),
		Status:         strings.TrimSpace(model.Status),
		Tags:           synthesizeLLMTags(model),
		ContextLimit:   validLimit(model.Limit.Context),
		MaxOutputLimit: validLimit(model.Limit.Output),
		Modalities:     synthesizeModalities(model.Modalities),
		LastUpdated:    strings.TrimSpace(model.LastUpdated),
	}

	// Cost is optional: metadata-only entries keep zero pricing.
	if input, ok := validCostField(model.Cost.Input); ok {
		if output, outOK := validCostField(model.Cost.Output); outOK {
			entry.InputPerMillion = input
			entry.OutputPerMillion = output
		}
	}
	entry.CacheReadPerMillion = cloneCostField(model.Cost.CacheRead)
	entry.CacheWritePerMillion = cloneCostField(model.Cost.CacheWrite)
	return entry, true
}

// validCostField reports a non-negative finite cost value.
func validCostField(value *float64) (float64, bool) {
	if value == nil || *value < 0 || math.IsNaN(*value) || math.IsInf(*value, 0) {
		return 0, false
	}
	return *value, true
}

// validLimit returns a positive integer limit or nil.
func validLimit(value *int64) *int64 {
	if value == nil || *value <= 0 {
		return nil
	}
	cloned := *value
	return &cloned
}

// synthesizeLLMTags derives stable capability tags from the model's
// boolean capability flags and modality set. Tag literals are the faceted
// filter values on the models page, so they are kept short and stable.
func synthesizeLLMTags(model llmMetadataModel) []string {
	var tags []string
	if model.Reasoning {
		tags = append(tags, "reasoning")
	}
	if model.ToolCall {
		tags = append(tags, "tool_call")
	}
	if model.Attachment {
		tags = append(tags, "attachment")
	}
	if model.StructuredOutput {
		tags = append(tags, "structured_output")
	}
	for _, m := range synthesizeModalities(model.Modalities) {
		switch m {
		case "image", "video":
			tags = append(tags, "vision")
		case "audio":
			tags = append(tags, "audio")
		case "pdf":
			tags = append(tags, "pdf")
		}
	}
	return tags
}

// synthesizeModalities returns the deduped union of input/output modality
// names, preserving input order first.
func synthesizeModalities(modalities llmMetadataModalities) []string {
	seen := make(map[string]struct{}, 6)
	out := make([]string, 0, 6)
	for _, list := range [][]string{modalities.Input, modalities.Output} {
		for _, name := range list {
			name = strings.ToLower(strings.TrimSpace(name))
			if name == "" || name == "text" {
				continue
			}
			if _, dup := seen[name]; dup {
				continue
			}
			seen[name] = struct{}{}
			out = append(out, name)
		}
	}
	return out
}
