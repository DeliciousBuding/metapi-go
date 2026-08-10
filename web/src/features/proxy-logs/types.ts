// metapi-go/features/proxy-logs — domain types for the proxy request log.
//
// Re-uses the backend contract declared in `lib/api.ts` (`ProxyLogListItem` /
// `ProxyLogDetail` and friends) so the feature speaks the same vocabulary as
// the transport layer. The feature-level `ProxyLog` alias and the query key
// factory live here so hooks and components share a single source of truth.
//
// The backend `/api/stats/proxy-logs/:id` detail response currently surfaces
// billing/route/channel + http status. Raw request/response bodies and
// headers are not part of the stable contract yet; they are declared here as
// optional fields (`requestBody` / `responseBody` / `requestHeaders` /
// `responseHeaders`) so the detail sheet can render them when the backend
// starts returning them, without a downstream type churn.

import type {
  ProxyLogBillingDetails,
  ProxyLogClientConfidence,
  ProxyLogClientOption,
  ProxyLogDetail as BackendProxyLogDetail,
  ProxyLogListItem,
  ProxyLogStatusFilter,
  ProxyLogUsageSource,
  ProxyLogsQuery,
  ProxyLogsResponse,
  ProxyLogsSummary,
} from '@/lib/api'

export {
  type ProxyLogBillingDetails,
  type ProxyLogClientConfidence,
  type ProxyLogClientOption,
  type ProxyLogStatusFilter,
  type ProxyLogUsageSource,
  type ProxyLogsQuery,
  type ProxyLogsResponse,
  type ProxyLogsSummary,
}

// --- List item + detail (feature contract) -------------------------------

/**
 * A single proxy log row as returned by `/api/stats/proxy-logs`. Re-exported
 * from `lib/api.ts` under the shorter feature-level name so column/component
 * code reads as `ProxyLog` rather than `ProxyLogListItem`.
 */
export type ProxyLog = ProxyLogListItem

/**
 * Detail payload returned by `/api/stats/proxy-logs/:id`. Extends the list
 * item with routing/channel/http-status/billing. The optional body/headers
 * fields are forward-compatible slots — populated when the backend exposes
 * them, otherwise `undefined` and the detail sheet hides the sections.
 */
export type ProxyLogDetail = BackendProxyLogDetail & {
  /** Raw upstream request body (JSON text), when surfaced by the backend. */
  requestBody?: string | null
  /** Raw upstream response body (JSON text), when surfaced by the backend. */
  responseBody?: string | null
  /** Upstream request headers (JSON text), when surfaced by the backend. */
  requestHeaders?: string | null
  /** Upstream response headers (JSON text), when surfaced by the backend. */
  responseHeaders?: string | null
}

// --- URL filter state -----------------------------------------------------

/**
 * Client-side view of the active filters, mirrored to the URL search string
 * so a deep link restores the exact view. The backend `ProxyLogsQuery` is a
 * subset of this (no latency range); latency range is applied client-side on
 * the fetched page because the backend does not support latency filtering.
 */
export type ProxyLogFilters = {
  search: string
  status: ProxyLogStatusFilter
  siteId: number | null
  client: string
  from: string
  to: string
  latencyMin: number | null
  latencyMax: number | null
}

/**
 * TanStack Query key factory. Centralised so cache invalidation is
 * grep-able and the keys stay stable across hooks.
 */
export const proxyLogsKeys = {
  all: ['proxy-logs'] as const,
  list: (params: ProxyLogsQuery) =>
    [...proxyLogsKeys.all, 'list', params] as const,
  detail: (id: number) => [...proxyLogsKeys.all, 'detail', id] as const,
  meta: (params: Omit<ProxyLogsQuery, 'limit' | 'offset'>) =>
    [...proxyLogsKeys.all, 'meta', params] as const,
}
