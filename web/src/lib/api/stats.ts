/* eslint-disable no-nested-ternary -- legacy chained ternary in refresh fallback */
import { request, buildQueryString } from './transport'
import type {
  SchedulerRunStatus,
  TriggerModelProbeResponse,
  ModelCostDistributionResponse,
  LatencyHistogramResponse,
  LatencyTrendResponse,
  VerifyBatchResponse,
  VerifyHistoryResponse,
  TagIndexResponse,
  Announcement,
  AnnouncementsResponse,
  ModelRedirectsResponse,
  RedirectApplyResponse,
  RateOverviewResponse,
  ProxyLogDetail,
  ProxyLogsQuery,
  ProxyLogsResponse,
  ProxyDebugTraceDetail,
  ProxyDebugTracesResponse,
} from './types'

export const statsApi = {
  // Stats
  getDashboard: () => request('/api/stats/dashboard'),
  getDashboardSnapshot: (options?: { refresh?: boolean }) =>
    request(
      `/api/stats/dashboard${buildQueryString({
        view: 'summary',
        ...(options?.refresh ? { refresh: 1 } : {}),
      })}`
    ),
  getDashboardInsights: (options?: { refresh?: boolean }) =>
    request(
      `/api/stats/dashboard${buildQueryString({
        view: 'insights',
        ...(options?.refresh ? { refresh: 1 } : {}),
      })}`
    ),
  getProxyLogs: (params?: ProxyLogsQuery) =>
    request(
      `/api/stats/proxy-logs${buildQueryString(params)}`
    ) as Promise<ProxyLogsResponse>,
  getProxyLogsQuery: (params?: ProxyLogsQuery) =>
    request(
      `/api/stats/proxy-logs${buildQueryString({ ...params, view: 'query' })}`
    ) as Promise<{
      items: ProxyLogsResponse['items']
      total: number
      page: number
      pageSize: number
    }>,
  getProxyLogsMeta: (
    params?: Omit<ProxyLogsQuery, 'limit' | 'offset'> & {
      refresh?: number | boolean
    }
  ) => {
    const refresh =
      params?.refresh === true
        ? 1
        : typeof params?.refresh === 'number'
          ? params.refresh
          : undefined
    const queryParams = {
      ...params,
      view: 'meta',
      ...(refresh !== undefined ? { refresh } : {}),
    } as Record<string, string | number | boolean | null | undefined>
    if (refresh === undefined) delete queryParams.refresh
    return request(
      `/api/stats/proxy-logs${buildQueryString(queryParams)}`
    ) as Promise<{
      clientOptions: ProxyLogsResponse['clientOptions']
      summary: ProxyLogsResponse['summary']
      sites: Array<{ id: number; name: string; status?: string | null }>
    }>
  },
  getProxyLogDetail: (id: number) =>
    request(`/api/stats/proxy-logs/${id}`) as Promise<ProxyLogDetail>,
  getUsageHeatmap: (params?: { days?: number; dimension?: 'site' | 'model' }) =>
    request(`/api/stats/usage-heatmap${buildQueryString(params)}`),
  getSlowRequests: (params?: {
    limit?: number
    minLatencyMs?: number
    hours?: number
  }) => request(`/api/stats/slow-requests${buildQueryString(params)}`),
  getProxyDebugTraces: (params?: { limit?: number }) =>
    request(
      `/api/stats/proxy-debug/traces${buildQueryString(params)}`
    ) as Promise<ProxyDebugTracesResponse>,
  getProxyDebugTraceDetail: (id: number) =>
    request(
      `/api/stats/proxy-debug/traces/${id}`
    ) as Promise<ProxyDebugTraceDetail>,
  checkModels: (accountId: number) =>
    request(`/api/models/check/${accountId}`, { method: 'POST' }),
  getSiteDistribution: () => request('/api/stats/site-distribution'),
  getSiteTrend: (days = 7) => request(`/api/stats/site-trend?days=${days}`),
  getBalanceHistory: (accountId: number, days = 30) =>
    request(`/api/stats/balance-history?accountId=${accountId}&days=${days}`),
  // A3: income vs outcome balance analysis.
  getBalanceIncomeOutcome: (days = 30) =>
    request(`/api/stats/balance-income-outcome?days=${days}`),
  // B1: admin write-operation audit log.
  getAdminAuditLogs: (params?: URLSearchParams) =>
    request(`/api/admin/audit-logs${params ? `?${params.toString()}` : ''}`),
  getAttention: (limit = 20) => request(`/api/stats/attention?limit=${limit}`),
  // A2: model cost distribution + latency chart gallery.
  getModelCostDistribution: (days = 30, topN = 8) =>
    request(
      `/api/stats/model-cost-distribution?days=${days}&topN=${topN}`
    ) as Promise<ModelCostDistributionResponse>,
  getLatencyHistogram: (days = 7, bucketMs = 500) =>
    request(
      `/api/stats/latency-histogram?days=${days}&bucketMs=${bucketMs}`
    ) as Promise<LatencyHistogramResponse>,
  getLatencyTrend: (days = 7) =>
    request(
      `/api/stats/latency-trend?days=${days}`
    ) as Promise<LatencyTrendResponse>,
  // Model availability probe: queue a background scheduler pass. Results
  // land in the scheduler-status recentRuns + model_probe_results history.
  probeModelsNow: () =>
    request('/api/models/probe', {
      method: 'POST',
      skipErrorHandler: true,
    }) as Promise<TriggerModelProbeResponse>,
  // G1: batch model verification + history.
  verifyModelsBatch: (
    models: string[],
    accountId = 0,
    limit = 50,
    // A large batch with the 15s per-probe ceiling can outgrow the 30s
    // default; allow long-running verification passes.
    timeoutMs = 180_000
  ) =>
    request('/api/models/verify-batch', {
      method: 'POST',
      body: JSON.stringify({ models, accountId, limit }),
      timeoutMs,
    }) as Promise<VerifyBatchResponse>,
  getModelVerifyHistory: (limit = 50, model = '') =>
    request(
      `/api/models/verify-history?limit=${limit}${model ? `&model=${encodeURIComponent(model)}` : ''}`
    ) as Promise<VerifyHistoryResponse>,
  // I1: accounts/sites global tag system.
  getTags: () => request('/api/tags') as Promise<TagIndexResponse>,
  updateAccountTags: (accountId: number, tags: string[]) =>
    request(`/api/accounts/${accountId}/tags`, {
      method: 'PUT',
      body: JSON.stringify({ tags }),
    }) as Promise<{ success: boolean; tags: string[] }>,
  updateSiteTags: (siteId: number, tags: string[]) =>
    request(`/api/sites/${siteId}/tags`, {
      method: 'PUT',
      body: JSON.stringify({ tags }),
    }) as Promise<{ success: boolean; tags: string[] }>,
  // H1: product risk banners.
  getActiveAnnouncements: () =>
    request('/api/announcements/active') as Promise<AnnouncementsResponse>,
  getAnnouncements: () =>
    request('/api/announcements') as Promise<AnnouncementsResponse>,
  createAnnouncement: (payload: {
    title: string
    message: string
    severity: Announcement['severity']
    link?: string | null
    enabled?: boolean
  }) =>
    request('/api/announcements', {
      method: 'POST',
      body: JSON.stringify(payload),
      // The announcements section surfaces its own saveFailed toast.
      skipErrorHandler: true,
    }) as Promise<AnnouncementsResponse>,
  updateAnnouncement: (
    id: number,
    payload: {
      title: string
      message: string
      severity: Announcement['severity']
      link?: string | null
      enabled?: boolean
    }
  ) =>
    request(`/api/announcements/${id}`, {
      method: 'PUT',
      body: JSON.stringify(payload),
      // The announcements section surfaces its own saveFailed toast.
      skipErrorHandler: true,
    }) as Promise<{ success: boolean; revision: boolean }>,
  deleteAnnouncement: (id: number) =>
    request(`/api/announcements/${id}`, {
      method: 'DELETE',
      // The announcements section surfaces its own deleteFailed toast.
      skipErrorHandler: true,
    }) as Promise<{ success: boolean }>,
  dismissAnnouncement: (id: number) =>
    request(`/api/announcements/${id}/dismiss`, {
      method: 'POST',
      body: '{}',
    }) as Promise<{ success: boolean }>,
  // K1a: model name redirects.
  getModelRedirects: (params?: { accountId?: number; source?: string }) =>
    request(
      `/api/model-redirects${buildQueryString(params)}`
    ) as Promise<ModelRedirectsResponse>,
  updateModelRedirect: (
    id: number,
    payload: { actual?: string; source?: string }
  ) =>
    request(`/api/model-redirects/${id}`, {
      method: 'PUT',
      body: JSON.stringify(payload),
      // The redirects section surfaces its own promoteFailed toast.
      skipErrorHandler: true,
    }) as Promise<{ success: boolean }>,
  deleteModelRedirect: (id: number) =>
    request(`/api/model-redirects/${id}`, {
      method: 'DELETE',
      // The redirects section surfaces its own deleteFailed toast.
      skipErrorHandler: true,
    }) as Promise<{ success: boolean }>,
  generateModelRedirects: (accountId = 0) =>
    request('/api/model-redirects/generate', {
      method: 'POST',
      body: JSON.stringify({ accountId }),
      // The redirects section surfaces its own generateFailed toast.
      skipErrorHandler: true,
    }) as Promise<{
      success: boolean
      created: number
      accounts?: number
    }>,
  applyModelRedirects: (dryRun: boolean) =>
    request('/api/model-redirects/apply', {
      method: 'POST',
      body: JSON.stringify({ dryRun }),
      // The redirects section surfaces its own applyFailed toast.
      skipErrorHandler: true,
    }) as Promise<RedirectApplyResponse>,
  // multiplier/rate overview (GET) + batch edit (PUT).
  getRateOverview: () =>
    request('/api/models/rates') as Promise<RateOverviewResponse>,
  // batch rate editing — unit_cost + weight only
  updateRates: (body: {
    accounts?: Array<{ id: number; unitCost: number }>
    channels?: Array<{ id: number; weight: number }>
  }) =>
    request<{
      success: boolean
      updatedAccounts: number
      updatedChannels: number
    }>('/api/models/rates', { method: 'PUT', body: JSON.stringify(body) }),
  // C1: unified recurring-scheduler run history.
  getSchedulerStatus: () =>
    request<{ items: SchedulerRunStatus[]; generatedAt: string }>(
      '/api/scheduler/status'
    ),
  getSiteSnapshot: async (days = 7, options?: { refresh?: boolean }) => {
    const query = buildQueryString({
      days,
      ...(options?.refresh ? { refresh: 1 } : {}),
    })
    const [distribution, trend, sites] = await Promise.all([
      request<{ distribution: unknown[] }>(
        `/api/stats/site-distribution${query}`
      ),
      request<{ trend: unknown[] }>(`/api/stats/site-trend${query}`),
      request<unknown[]>('/api/sites'),
    ])
    return {
      generatedAt: new Date().toISOString(),
      distribution: Array.isArray(distribution?.distribution)
        ? distribution.distribution
        : [],
      trend: Array.isArray(trend?.trend) ? trend.trend : [],
      sites: Array.isArray(sites) ? sites : [],
    }
  },
  getModelBySite: (siteId?: number, days = 7) =>
    request(
      `/api/stats/model-by-site?${siteId ? `siteId=${siteId}&` : ''}days=${days}`
    ),
}
