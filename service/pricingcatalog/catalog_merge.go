package pricingcatalog

// MergeSnapshots merges per-source snapshots with first-wins priority:
// parts[0] overrides parts[1], which overrides parts[2], and so on. nil
// parts and empty snapshots are skipped. The Claude alias table is rebuilt
// from the merged exact entries. The merged snapshot's Source / FetchedAt
// are left for the caller to set (per-source snapshots carry their own
// provenance there).
//
// Late sources carrying augmentation-only data (the newapi ratio set from
// llm-metadata, registered after the price dataset) do not replace an
// earlier entry wholesale: entries that already exist only gain the ratio
// fields and, when they carry no usable USD price, the incoming price. This
// keeps the official list price (first source) authoritative while making
// the ratio set available to the resolver for relay-site estimates.
func MergeSnapshots(parts []*CatalogSnapshot) *CatalogSnapshot {
	merged := NewCatalogSnapshot()
	for i := 0; i < len(parts); i++ {
		part := parts[i]
		if part == nil {
			continue
		}
		for key, entry := range part.models {
			if existing, exists := merged.models[key]; exists {
				merged.models[key] = augmentEntry(existing, entry)
				continue
			}
			merged.models[key] = entry
		}
	}
	for _, entry := range merged.models {
		merged.addAlias(entry)
	}
	return merged
}

// augmentEntry fills augmentation-only fields from an incoming entry into a
// previously merged one. Never touches the existing entry's authoritative
// fields (USD prices, description, modalities, provider), so an earlier
// high-priority source always wins its own data.
func augmentEntry(existing, incoming CatalogEntry) CatalogEntry {
	if !incoming.ListPriceKnown && incoming.ModelRatio == nil && incoming.InputPerMillion <= 0 && incoming.OutputPerMillion <= 0 {
		return existing
	}
	out := existing
	if out.ModelRatio == nil && incoming.ModelRatio != nil {
		out.ModelRatio = incoming.ModelRatio
		out.CompletionRatio = incoming.CompletionRatio
		out.CacheRatio = incoming.CacheRatio
		if out.BillingMode == "" {
			out.BillingMode = incoming.BillingMode
		}
		if out.BillingExpr == "" {
			out.BillingExpr = incoming.BillingExpr
		}
	}
	if !out.ListPriceKnown && !isUsableAmount(out.InputPerMillion) && (incoming.ListPriceKnown || isUsableAmount(incoming.InputPerMillion)) {
		out.InputPerMillion = incoming.InputPerMillion
		out.ListPriceKnown = incoming.ListPriceKnown
		out.PriceSource = incoming.PriceSource
		out.PriceFetchedAt = incoming.PriceFetchedAt
		if isUsableAmount(incoming.OutputPerMillion) {
			out.OutputPerMillion = incoming.OutputPerMillion
		}
		if out.CacheReadPerMillion == nil || *out.CacheReadPerMillion < 0 {
			out.CacheReadPerMillion = incoming.CacheReadPerMillion
		}
		if out.CacheWritePerMillion == nil || *out.CacheWritePerMillion < 0 {
			out.CacheWritePerMillion = incoming.CacheWritePerMillion
		}
	}
	return out
}
