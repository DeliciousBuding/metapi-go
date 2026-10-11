import { request } from './transport'
import type { AboutBuildInfoResponse } from './types'

export const systemApi = {
  /**
   * Build provenance of the running Go binary: version, commit, build time and
   * Go runtime version. Uninjected local builds report empty commit/buildTime.
   */
  getAbout: () => request<AboutBuildInfoResponse>('/api/about'),

  // Monitor embed
  getMonitorHealth: () => request('/api/monitor/health'),
  // Clears the HttpOnly `meta_monitor_auth` cookie (Path=/monitor-proxy/);
  // must be called while Bearer auth is still valid.
  clearMonitorSession: () =>
    request('/api/monitor/session', { method: 'DELETE' }),

  // Models marketplace
  getModelsMarketplace: (options?: {
    refresh?: boolean
    includePricing?: boolean
    page?: number
    pageSize?: number
    /** Whole-marketplace substring search (name / description / supply names). */
    q?: string
    /** Server-side filter CSVs; semantics are whitelisted in docs/api/models.md. */
    capability?: string
    endpointType?: string
    sourceKind?: string
    /** First table sort term (`column:asc|desc`); unknown columns fall back to name.asc. */
    sort?: string
  }) => {
    const params = new URLSearchParams()
    if (options?.refresh) params.set('refresh', '1')
    if (options?.includePricing) params.set('includePricing', '1')
    if (options?.page != null) params.set('page', String(options.page))
    if (options?.pageSize != null) {
      params.set('pageSize', String(options.pageSize))
    }
    if (options?.q) params.set('q', options.q)
    if (options?.capability) params.set('capability', options.capability)
    if (options?.endpointType) params.set('endpointType', options.endpointType)
    if (options?.sourceKind) params.set('sourceKind', options.sourceKind)
    if (options?.sort) params.set('sort', options.sort)
    const query = params.toString()
    return request<
      | { models?: unknown[]; meta?: unknown }
      | {
          items?: unknown[]
          total?: number
          page?: number
          pageSize?: number
          facets?: unknown
          facetScope?: string
          meta?: unknown
        }
      | unknown[]
    >(`/api/models/marketplace${query ? `?${query}` : ''}`, {
      timeoutMs: options?.refresh ? 45_000 : 15_000,
    })
  },
  /**
   * One marketplace row by exact model name, independent of pagination — the
   * resolver behind the `?model=<name>` deep link. 404 arrives as a thrown
   * error whose message is the server's "model not found".
   */
  getModelMarketplaceDetail: (name: string) => {
    const params = new URLSearchParams({ name })
    return request<{ model?: unknown }>(
      `/api/models/marketplace/detail?${params.toString()}`,
      { timeoutMs: 15_000 }
    )
  },
  /** Cross-site effective model price comparison. */
  getModelPriceCompare: (options?: {
    model?: string
    days?: number
    limit?: number
    topModels?: number
    exactModel?: boolean
  }) => {
    const params = new URLSearchParams()
    if (options?.model) params.set('model', options.model)
    if (options?.days != null) params.set('days', String(options.days))
    if (options?.limit != null) params.set('limit', String(options.limit))
    if (options?.topModels != null) {
      params.set('topModels', String(options.topModels))
    }
    if (options?.exactModel) params.set('exactModel', 'true')
    const query = params.toString()
    return request(`/api/models/price-compare${query ? `?${query}` : ''}`, {
      timeoutMs: 20_000,
    })
  },
  getModelTokenCandidates: () => request('/api/models/token-candidates'),
}
