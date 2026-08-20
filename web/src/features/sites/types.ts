// metapi-go/features/sites — domain types for the Sites feature.
//
// The backend `/api/sites` surface is untyped in `lib/api.ts` (legacy
// signatures preserved as `any` during the rewrite). These types are the
// feature-level contract: `api.ts` casts responses to `Site` and the
// components consume them. Field set is derived from the legacy `SiteRow`
// shape plus `accountCount` (enriched later from the accounts snapshot) so
// the list column can render without a second request today.

export type SiteStatus = 'active' | 'disabled'

export type SiteProbeScope = 'single' | 'all'

export type SiteApiEndpoint = {
  id?: number
  url: string
  enabled?: boolean
  sortOrder?: number
  cooldownUntil?: string | null
  lastFailureReason?: string | null
}

type SiteSubscriptionSummary = {
  activeCount: number
  totalUsedUsd?: number
  totalMonthlyLimitUsd?: number | null
  totalRemainingUsd?: number | null
  nextExpiresAt?: string | null
  planNames?: string[]
  updatedAt?: string | null
}

export type Site = {
  id: number
  name: string
  url: string
  externalCheckinUrl?: string | null
  platform?: string
  status?: SiteStatus
  proxyUrl?: string | null
  useSystemProxy?: boolean
  customHeaders?: string | null
  customHeadersOverrideRequestHeaders?: boolean
  globalWeight?: number
  maxConcurrency?: number
  isPinned?: boolean
  sortOrder?: number
  totalBalance?: number
  accountCount?: number
  subscriptionSummary?: SiteSubscriptionSummary | null
  createdAt?: string
  postRefreshProbeEnabled?: boolean
  postRefreshProbeModel?: string | null
  postRefreshProbeScope?: SiteProbeScope | null
  postRefreshProbeLatencyThresholdMs?: number | null
  resinEnabled?: boolean | null
  useUtls?: boolean | null
  browserUa?: string | null
  cfClearance?: string | null
  apiEndpoints?: SiteApiEndpoint[]
  tags?: string[]
}

/**
 * Wire format for POST /api/sites and PUT /api/sites/:id. Matches the
 * legacy `SiteSavePayload` shape so the backend accepts it unchanged.
 */
export type SiteFormPayload = {
  name: string
  url: string
  externalCheckinUrl: string
  platform: string
  proxyUrl: string
  useSystemProxy: boolean
  apiEndpoints: Array<{
    url: string
    enabled: boolean
    sortOrder: number
  }>
  customHeaders: string
  customHeadersOverrideRequestHeaders?: boolean
  globalWeight: number
  maxConcurrency: number
  postRefreshProbeEnabled?: boolean
  postRefreshProbeModel?: string
  postRefreshProbeScope?: SiteProbeScope
  postRefreshProbeLatencyThresholdMs?: number
  resinEnabled?: boolean | null
  useUtls?: boolean | null
}

/**
 * Result of batch operations on sites. The backend returns the subset of
 * ids that succeeded plus the failures with reasons.
 */
export type SiteBatchResult = {
  successIds?: number[]
  failedItems?: Array<{ id: number; reason?: string }>
}

export type SiteBatchAction =
  | 'enable'
  | 'disable'
  | 'delete'
  | 'enableSystemProxy'
  | 'disableSystemProxy'

/**
 * TanStack Query key factory. Centralised so invalidation is grep-able and
 * the keys stay stable across hooks.
 */
export const sitesKeys = {
  all: ['sites'] as const,
  list: () => [...sitesKeys.all, 'list'] as const,
  detail: (id: number) => [...sitesKeys.all, 'detail', id] as const,
}
