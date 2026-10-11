// metapi-go/features/models — domain types for the Models marketplace feature.
//
// The list page is server-driven: search, filters, sort, totals, and facet
// counts come from /api/models/marketplace over the whole marketplace (never
// from the currently rendered page). Facet counts arrive alongside the page
// so the toolbar options stay global while the table renders one page.
//
// The backend `/api/models/marketplace` surface is typed in `lib/api.ts`
// (the rewrite replaced the legacy `any` signatures). These types are
// the feature-level contract: `api.ts` casts the response to
// `ModelsMarketplaceResponse` and the components consume `ModelRow`. Field
// set is ported verbatim from the legacy `pages/Models.tsx` interfaces so
// the marketplace data shape stays 1:1 with the backend.

/**
 * A downstream token identity configured for a model inside an account.
 */
type ModelTokenInfo = {
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
type ModelPricingSource = {
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
type ModelAccountInfo = {
  id: number
  site: string
  username: string | null
  latency: number | null
  balance: number
  tokens: ModelTokenInfo[]
}

/**
 * A single model row in the marketplace. `supportedEndpointTypes` is the set
 * of API surfaces the model answers (e.g. `anthropic`, `openai`); `tags`
 * carries the real capability set hydrated from the model catalog
 * (reasoning / tool_call / vision / ...).
 */
export type ModelRow = {
  name: string
  /** Stored supply, not a count of currently eligible routing candidates. */
  supplyCount?: number
  supplySources?: ModelSupplySource[]
  accountCount: number
  tokenCount: number
  avgLatency: number | null
  successRate: number | null
  description: string | null
  tags: string[]
  supportedEndpointTypes: string[]
  pricingSources: ModelPricingSource[]
  accounts: ModelAccountInfo[]
  /** Catalog lifecycle status: true when the catalog marks the model deprecated. */
  deprecated?: boolean
  /** Catalog release date (ISO). */
  releaseDate?: string | null
  /** Catalog context window in tokens. */
  contextWindow?: number | null
  /** Catalog hydration state: "hydrated" (metadata from catalog) or "unknown". */
  catalogStatus?: 'hydrated' | 'unknown'
}

export type ModelSupplySource = {
  kind: 'direct' | 'account' | 'oauth'
  id: number
  name: string
  connectionName: string
  upstreamModel: string
  evidence: 'configured' | 'availability'
  enabled: boolean
  siteId?: number
  channelId?: number
  credentialId?: number
  credentialKind?: string
  successCount?: number
  failCount?: number
  bindings: Array<{
    itemId: number
    routeId: number
    publicModel: string
    routeEnabled: boolean
    groupEnabled: boolean
    selected: boolean
  }>
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

/** One facet option plus the number of matching models in its facet context. */
type ModelsMarketplaceFacetValue = { value: string; count: number }

/**
 * Whole-marketplace facet counts carried by the paged response. Each
 * dimension is disjunctive (`facetScope: "filteredExcludingSelf"`): its
 * options are counted with that dimension's own filter removed, so selecting
 * a value never hides the alternatives the user could switch to.
 */
export type ModelsMarketplaceFacets = {
  endpointTypes: ModelsMarketplaceFacetValue[]
  capabilities: ModelsMarketplaceFacetValue[]
  sourceKinds: ModelsMarketplaceFacetValue[]
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
  detail: (modelName: string) =>
    [...modelsKeys.all, 'detail', modelName] as const,
}
