package admin

import (
	"strings"

	"github.com/deliciousbuding/metapi-go/app"
	"github.com/deliciousbuding/metapi-go/service/pricingcatalog"
)

// lowestCatalogInputRate reduces the synthetic catalog pricing sources to the
// cheapest per-million input, mirroring the client-side price column so the
// server can sort by it. Nil when no usable rate exists.
func lowestCatalogInputRate(sources []any) *float64 {
	var lowest *float64
	for _, raw := range sources {
		source, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		groups, ok := source["groupPricing"].(map[string]any)
		if !ok {
			continue
		}
		for _, rawGroup := range groups {
			group, ok := rawGroup.(map[string]any)
			if !ok {
				continue
			}
			input, ok := group["inputPerMillion"].(float64)
			if !ok {
				continue
			}
			if lowest == nil || input < *lowest {
				v := input
				lowest = &v
			}
		}
	}
	return lowest
}

// appCatalogSnapshot returns the merged model-catalog snapshot, or nil when
// the catalog manager is unavailable (PRICING_CATALOG_ENABLED=false).
func appCatalogSnapshot() *pricingcatalog.CatalogSnapshot {
	manager := app.CatalogManager()
	if manager == nil {
		return nil
	}
	return manager.Snapshot()
}

func catalogLookup(snapshot *pricingcatalog.CatalogSnapshot, name string) (pricingcatalog.CatalogEntry, bool) {
	if snapshot == nil {
		return pricingcatalog.CatalogEntry{}, false
	}
	return snapshot.Lookup(name)
}

func catalogDescription(entry pricingcatalog.CatalogEntry, hit bool) any {
	if !hit || strings.TrimSpace(entry.Description) == "" {
		return nil
	}
	return entry.Description
}

func catalogTags(entry pricingcatalog.CatalogEntry, hit bool) []string {
	if !hit || len(entry.Tags) == 0 {
		return []string{}
	}
	return entry.Tags
}

func catalogReleaseDate(entry pricingcatalog.CatalogEntry, hit bool) any {
	if !hit || strings.TrimSpace(entry.ReleaseDate) == "" {
		return nil
	}
	return entry.ReleaseDate
}

func catalogStatusOf(hit bool) string {
	if hit {
		return "hydrated"
	}
	return "unknown"
}

// catalogPricingSources builds the synthetic catalog list-price source:
// {siteId: 0, siteName: "catalog", groupPricing: {"catalog": {input/output}}}.
// Uses the explicit USD list price when present; otherwise derives the
// per-million rates from the NewAPI ratio set (ratio × $2 base). Only emitted
// when a usable input rate exists so a partial price never misleads.
func catalogPricingSources(entry pricingcatalog.CatalogEntry, hit bool) []any {
	if !hit {
		return []any{}
	}
	input, output, ok := entry.ReferencePerMillionRates()
	if !ok {
		return []any{}
	}
	return []any{map[string]any{
		"siteId":       0,
		"siteName":     "catalog",
		"accountId":    0,
		"username":     nil,
		"ownerBy":      nil,
		"enableGroups": []string{},
		"groupPricing": map[string]any{
			"catalog": map[string]any{
				"quotaType":        0,
				"inputPerMillion":  input,
				"outputPerMillion": output,
			},
		},
	}}
}

// inferEndpointTypesForModel derives the model's routeable endpoint types:
// the API dialect (anthropic / gemini / openai) and the non-chat endpoint
// families (images, audio, videos, embeddings, rerank). When the model is
// cataloged, recognizable big-three model families keep their native dialect;
// other models use the catalog provider/default dialect. Non-chat families
// come from the catalog modalities
// (vision/audio/video output). The name-suffix heuristic is retained for
// embeddings/rerank (no modality signal exists for them) and as the
// catalog-miss fallback.
func inferEndpointTypesForModel(snapshot *pricingcatalog.CatalogSnapshot, modelName string, accounts []map[string]any) []string {
	if entry, ok := catalogLookup(snapshot, modelName); ok {
		return catalogEndpointTypes(entry, modelName)
	}
	return endpointTypesByName(modelName)
}

// catalogEndpointTypes builds the endpoint type set from catalog metadata.
// The dialect first recognizes native big-three model families because catalog
// provider slugs can name a reseller rather than the model protocol. Otherwise
// it maps anthropic/google providers and defaults to OpenAI-compatible. Non-chat
// families come from
// the directed modality lists: an image output means image generation (with
// the edit surface when the model also takes image input), an audio output
// means speech, an audio input means transcription, a video output means
// video creation. A vision chat model (image in, text out) stays on the
// chat surface. Embeddings and rerank have no modality signal, so their
// families still come from the model name.
func catalogEndpointTypes(entry pricingcatalog.CatalogEntry, modelName string) []string {
	var types []string
	if dialect := endpointDialectByName(modelName); dialect != "" {
		// models.dev provider slugs identify the listing/vendor, not always
		// the model protocol (for example, a Claude row can be listed under
		// a relay provider). Keep recognizable big-three model families on
		// their native API surface.
		types = append(types, dialect)
	} else {
		switch strings.ToLower(strings.TrimSpace(entry.Provider)) {
		case "anthropic":
			types = append(types, "anthropic")
		case "google":
			types = append(types, "gemini")
		default:
			// Catalog-only providers (deepseek, meta, mistral, x-ai,
			// relay vendors, and ratio-only rows) use the
			// OpenAI-compatible surface.
			types = append(types, "openai")
		}
	}

	input := modalitySet(entry.ModalitiesInput)
	output := modalitySet(entry.ModalitiesOutput)
	if output["image"] {
		types = append(types, "images.generations")
		if input["image"] {
			// Generation models that accept an input image also serve the
			// edit surface; a vision-only chat model does not.
			types = append(types, "images.edits")
		}
	}
	if output["audio"] {
		types = append(types, "audio.speech")
	}
	if input["audio"] {
		types = append(types, "audio.transcriptions")
	}
	if output["video"] {
		types = append(types, "videos.create")
	}
	return appendEndpointFamilies(types, modelName)
}

// modalitySet is the ordered membership check helper for modality lists.
func modalitySet(modalities []string) map[string]bool {
	set := make(map[string]bool, len(modalities))
	for _, modality := range modalities {
		set[strings.ToLower(strings.TrimSpace(modality))] = true
	}
	return set
}

// endpointTypesByName is the catalog-miss fallback: name-prefix heuristics
// for the big-three dialects plus name-suffix families that no field can
// express (embedding/rerank models are text-in/text-out like chat models).
func endpointTypesByName(modelName string) []string {
	return appendEndpointFamilies(nil, modelName)
}

// appendEndpointFamilies adds the name-derived endpoint families
// (embeddings / rerank) and the big-three dialect when no dialect value was
// supplied yet. Values are deduped and order-stable.
func appendEndpointFamilies(types []string, modelName string) []string {
	lower := strings.ToLower(modelName)
	if len(types) == 0 {
		if dialect := endpointDialectByName(modelName); dialect != "" {
			types = append(types, dialect)
		}
	}
	if containsAnyFold(lower, "embedding", "-embed", "bge-", "e5-") {
		types = append(types, "embeddings")
	}
	if containsAnyFold(lower, "rerank") {
		types = append(types, "rerank")
	}
	return dedupeStrings(types)
}

func endpointDialectByName(modelName string) string {
	lower := strings.ToLower(modelName)
	switch {
	case strings.Contains(lower, "claude"), strings.HasPrefix(lower, "anthropic"):
		return "anthropic"
	case strings.Contains(lower, "gemini"):
		return "gemini"
	case strings.Contains(lower, "gpt"), strings.Contains(lower, "o1"), strings.Contains(lower, "o3"), strings.Contains(lower, "o4"):
		return "openai"
	default:
		return ""
	}
}

func containsAnyFold(haystack string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(haystack, needle) {
			return true
		}
	}
	return false
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, dup := seen[value]; dup {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
