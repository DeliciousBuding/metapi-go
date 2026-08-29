import { request } from './transport'
import type { AboutBuildInfoResponse } from './types'

export const systemApi = {
  /**
   * Build provenance of the running Go binary: version, commit, build time and
   * Go runtime version. Uninjected local builds report empty commit/buildTime.
   */
  getAbout: () => request<AboutBuildInfoResponse>('/api/about'),

  // Monitor embed
  getMonitorConfig: () => request('/api/monitor/config'),
  getMonitorHealth: () => request('/api/monitor/health'),
  updateMonitorConfig: (data: { ldohCookie?: string | null }) =>
    request('/api/monitor/config', {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
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
  }) => {
    const params = new URLSearchParams()
    if (options?.refresh) params.set('refresh', '1')
    if (options?.includePricing) params.set('includePricing', '1')
    if (options?.page != null) params.set('page', String(options.page))
    if (options?.pageSize != null) params.set('pageSize', String(options.pageSize))
    const query = params.toString()
    return request<
      | { models?: unknown[]; meta?: unknown }
      | {
          items?: unknown[]
          total?: number
          page?: number
          pageSize?: number
          meta?: unknown
        }
      | unknown[]
    >(
      `/api/models/marketplace${query ? `?${query}` : ''}`,
      {
        timeoutMs: options?.refresh ? 45_000 : 15_000,
      }
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
