package pricingcatalog

import (
	"math"
	"time"
)

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
// tokens (models.dev cost unit). The metadata fields beyond the models.dev
// cost shape (Description / Tags / Status / limits / Modalities) are
// hydrated by the llm-metadata parser and stay empty for models.dev rows.
type CatalogEntry struct {
	// ListPriceKnown distinguishes explicit free pricing from metadata-only rows.
	ListPriceKnown bool
	PriceSource    string
	PriceFetchedAt time.Time
	// ModelID is the normalized (lowercase, provider prefix stripped)
	// catalog model id, e.g. "gpt-4o" or "claude-sonnet-4-20250514".
	ModelID string
	// Provider is the catalog provider slug, e.g. "openai".
	Provider string
	// InputPerMillion / OutputPerMillion are USD per 1M tokens. Zero when
	// the catalog entry carries metadata but no usable price.
	InputPerMillion  float64
	OutputPerMillion float64
	// CacheReadPerMillion / CacheWritePerMillion are optional prompt-cache
	// rates; nil when the catalog omits them.
	CacheReadPerMillion  *float64
	CacheWritePerMillion *float64
	// ReleaseDate is the catalog release_date (ISO), used to pick the
	// latest snapshot when a Claude family alias is ambiguous.
	ReleaseDate string
	// Aliased is true when the entry was resolved through a Claude family
	// alias (e.g. "claude-3-5-sonnet" → latest dated snapshot) instead of an
	// exact model id.
	Aliased bool
	// ---- llm-metadata superset (empty for plain models.dev rows) ----
	// Description is the model's short prose description.
	Description string
	// DisplayName is the human-facing model name (e.g. "GPT-4o").
	DisplayName string
	// Family is the model family slug (e.g. "gpt-4o").
	Family string
	// Status is the catalog lifecycle status: "" (active), "deprecated",
	// "beta", "experimental".
	Status string
	// Tags are synthesized capability tags (reasoning, tool_call, vision,
	// audio, ...) shown as the model's capability set.
	Tags []string
	// ContextLimit is the context window in tokens (nil when unknown).
	ContextLimit *int64
	// MaxOutputLimit is the max output tokens (nil when unknown).
	MaxOutputLimit *int64
	// Modalities is the union of input/output modalities (text, image,
	// audio, video, pdf).
	Modalities []string
	// ModalitiesInput / ModalitiesOutput are the directed modality lists
	// ("text", "image", "audio", "video", "pdf"). They carry the input vs
	// output direction the union loses — the endpoint-type inference needs
	// it to tell a vision chat model (image in, text out) from an image
	// generator (image output) or a TTS model (audio output).
	ModalitiesInput  []string
	ModalitiesOutput []string
	// LastUpdated is the catalog row's last_updated date (ISO).
	LastUpdated string
	// ---- llm-metadata /api/newapi ratio set (empty for non-newapi rows) ----
	// ModelRatio is the NewAPI model multiplier: the upstream bills
	// ModelRatio × NewAPIRatioBasePerMillion ($2) per 1M input tokens, so
	// the ratio set is the true cost basis of NewAPI-family relay sites.
	ModelRatio *float64
	// CompletionRatio is the output relative multiplier (output rate =
	// ModelRatio × CompletionRatio × base).
	CompletionRatio *float64
	// CacheRatio is the cache-read relative multiplier (cache rate =
	// ModelRatio × CacheRatio × base).
	CacheRatio *float64
	// BillingMode is the NewAPI billing mode: "" (flat) or "tiered_expr".
	BillingMode string
	// BillingExpr is the NewAPI tiered billing expression (expr-lang
	// syntax, opaque to metapi — used by upstream to compute tiered rates).
	BillingExpr string
}

// HasRatioData reports whether the entry carries a usable NewAPI ratio set.
func (e CatalogEntry) HasRatioData() bool {
	return e.ModelRatio != nil && isUsableAmount(*e.ModelRatio)
}

// ReferenceUnitCostFromRatios estimates the per-request unit cost from the
// NewAPI ratio set with the canonical $2/1M input anchor — the same 1k
// input + 1k output reference sample as ReferenceUnitCost. Ratios are the
// upstream's actual billing multipliers for NewAPI-family relays, so this
// estimate matches how such relays charge (ratio × base × group ratio).
// ok=false when the model ratio is missing or not positive-finite.
func (e CatalogEntry) ReferenceUnitCostFromRatios() (cost float64, ok bool) {
	if !e.HasRatioData() {
		return 0, false
	}
	modelRatio := *e.ModelRatio
	completion := 1.0
	if e.CompletionRatio != nil && isUsableAmount(*e.CompletionRatio) {
		completion = *e.CompletionRatio
	}
	input := modelRatio * NewAPIRatioBasePerMillion
	output := modelRatio * completion * NewAPIRatioBasePerMillion
	cost = (input + output) / 1000
	if !(cost > 0) || math.IsNaN(cost) || math.IsInf(cost, 0) {
		return 0, false
	}
	return cost, true
}

// ReferencePerMillionRates returns the entry's USD per-1M input/output
// rates. The explicit list price wins when both rates are usable; otherwise
// the rates are derived from the NewAPI ratio set (ratio × $2 base). An
// explicit 0 output rate (e.g. embedding rows where only input is billed)
// is treated as usable only when input is also usable — the zero is real. A
// ratio-derived fallback keeps metadata-only rows priceable. ok=false when
// neither path yields a usable input rate.
func (e CatalogEntry) ReferencePerMillionRates() (input, output float64, ok bool) {
	if isUsableAmount(e.InputPerMillion) && isUsableAmount(e.OutputPerMillion) {
		return e.InputPerMillion, e.OutputPerMillion, true
	}
	if isUsableAmount(e.InputPerMillion) && e.OutputPerMillion == 0 && e.OutputPerMillion >= 0 {
		// Explicit zero output: embedded-style pricing, input only.
		return e.InputPerMillion, 0, true
	}
	if !e.HasRatioData() {
		return 0, 0, false
	}
	rate := *e.ModelRatio * NewAPIRatioBasePerMillion
	outRate := rate
	if e.CompletionRatio != nil && isUsableAmount(*e.CompletionRatio) {
		outRate = rate * *e.CompletionRatio
	}
	if !isUsableAmount(rate) {
		return 0, 0, false
	}
	return rate, outRate, true
}

// isUsableAmount reports a positive finite value.
func isUsableAmount(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

// ReferenceUnitCost converts a catalog entry into the per-request unit cost
// consumed by routing.EffectiveUnitCost, using the same reference sample as
// routing.DefaultPriceCompareSampleUsage (1k input + 1k output tokens).
// Catalog rates are USD/1M, so the sample cost is (input + output) / 1000.
// Cache rates do not contribute: the reference sample carries no cache tokens.
func (e CatalogEntry) ReferenceUnitCost() float64 {
	return (e.InputPerMillion + e.OutputPerMillion) / 1000
}
