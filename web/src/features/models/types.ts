// metapi-go/features/models — domain types for the Models marketplace feature.
//
// The backend `/api/models/marketplace` surface is untyped in `lib/api.ts`
// (legacy signatures preserved as `any` during the rewrite). These types are
// the feature-level contract: `api.ts` casts the response to
// `ModelsMarketplaceResponse` and the components consume `ModelRow`. Field
// set is ported verbatim from the legacy `pages/Models.tsx` interfaces so
// the marketplace data shape stays 1:1 with the backend.

/**
 * A downstream token identity configured for a model inside an account.
 */
export type ModelTokenInfo = {
  id: number
  name: string
  isDefault: boolean
}

/**
 * Per-group pricing for a model on a given account. `quotaType` is the
 * backend's numeric quota unit kind; the per-million fields are in USD.
 */
export type ModelGroupPricing = {
  quotaType: number
  inputPerMillion?: number
  outputPerMillion?: number
  perCallInput?: number
  perCallOutput?: number
  perCallTotal?: number
}

/**
 * One account's pricing contribution for a model. A single model can be
 * served by multiple accounts across sites, each exposing its own group
 * pricing matrix (keyed by downstream group name).
 */
export type ModelPricingSource = {
  siteId: number
  siteName: string
  accountId: number
  username: string | null
  ownerBy: string | null
  enableGroups: string[]
  groupPricing: Record<string, ModelGroupPricing>
}

/**
 * Per-account availability row for a model. `latency` is the recent average
 * round-trip in ms (null when never probed). `tokens` lists the downstream
 * token entries that resolve to this model on that account.
 */
export type ModelAccountInfo = {
  id: number
  site: string
  username: string | null
  latency: number | null
  balance: number
  tokens: ModelTokenInfo[]
}

/**
 * A single model row in the marketplace. `supportedEndpointTypes` is the set
 * of API surfaces the model answers (e.g. `chat/completions`,
 * `embeddings`, `images/generations`) and doubles as the capability set.
 */
export type ModelRow = {
  name: string
  accountCount: number
  tokenCount: number
  avgLatency: number | null
  successRate: number | null
  description: string | null
  tags: string[]
  supportedEndpointTypes: string[]
  pricingSources: ModelPricingSource[]
  accounts: ModelAccountInfo[]
}

/**
 * Envelope returned by `api.getModelsMarketplace`. `meta` carries the
 * refresh-job state when the caller passes `refresh: true`.
 */
export type ModelsMarketplaceResponse = {
  models: ModelRow[]
  meta?: {
    refreshRequested?: boolean
    refreshQueued?: boolean
    refreshReused?: boolean
    refreshRunning?: boolean
    refreshJobId?: string | null
  }
}

/**
 * Derived capability view used by the detail sheet: the union of
 * `supportedEndpointTypes` and `tags`, deduped and sorted for display.
 */
export type ModelCapabilitySummary = {
  endpointTypes: string[]
  tags: string[]
  all: string[]
}

/**
 * TanStack Query key factory. Centralised so invalidation is grep-able and
 * the keys stay stable across hooks. The marketplace options (`refresh`,
 * `includePricing`) are part of the key so a pricing-hydrated fetch does
 * not shadow a fast base fetch.
 */
export const modelsKeys = {
  all: ['models'] as const,
  marketplace: (options?: { refresh?: boolean; includePricing?: boolean }) =>
    [
      ...modelsKeys.all,
      'marketplace',
      {
        refresh: options?.refresh ?? false,
        includePricing: options?.includePricing ?? false,
      },
    ] as const,
  detail: (modelName: string) => [...modelsKeys.all, 'detail', modelName] as const,
}
