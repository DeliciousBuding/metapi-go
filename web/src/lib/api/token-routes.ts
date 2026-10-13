import type { RouteDecision } from '@/lib/helpers/token-route-contract'

/** Admission blocker with the live chain's reason code. */
type KeyAdmissionBlocker = {
  dimension: 'policy' | 'state' | 'expiry' | 'budget' | 'quota' | 'ip' | 'rate'
  code: string
  message: string
}

export type KeyContextDecisionResponse = {
  success: boolean
  keyContext: {
    keyId: number
    name: string
    admission: {
      verdict: 'allow' | 'deny'
      blockers: KeyAdmissionBlocker[]
      notes: string[]
      ip: { checked: boolean; allowed: boolean; reason?: string }
      cost?: { used: number; max: number | null }
      requests?: { used: number; max: number | null }
      rate?: {
        rpm: { used: number; max: number | null }
        tpm: { used: number; max: number | null }
        scope: string
      }
      quota?: {
        windowStart: string
        windowEnd: string
        requestsUsed: number
        requestsMax?: number
        tokensUsed: number
        tokensMax?: number
        costUsed: number
        costMax?: number
        unknownTokens: number
        unknownCost: number
        historyMissing: boolean
        blocker: string
        message: string
      }
    }
  }
  decision: RouteDecision
}

import { request, buildQueryString } from './transport'

/** PUT /api/channels/batch response envelope — per-item truth. */
export type BatchUpdateChannelsResult = {
  success: boolean
  successIds: number[]
  failedItems: Array<{ id: number; message: string }>
  channels: Array<Record<string, unknown>>
}

// Only explicit synchronous callers wait for a fleet-wide rebuild. The UI
// submits wait:false and observes the task through the existing tasks API.
const REBUILD_ROUTES_TIMEOUT_MS = 300_000

export const tokenRoutesApi = {
  // Account tokens
  getAccountTokens: (accountId?: number) =>
    request(`/api/account-tokens${accountId ? `?accountId=${accountId}` : ''}`),
  addAccountToken: (data: unknown) =>
    request('/api/account-tokens', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  updateAccountToken: (id: number, data: unknown) =>
    request(`/api/account-tokens/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  deleteAccountToken: (id: number) =>
    request(`/api/account-tokens/${id}`, { method: 'DELETE' }),
  setDefaultAccountToken: (id: number) =>
    request(`/api/account-tokens/${id}/default`, { method: 'POST' }),
  syncAccountTokens: (accountId: number) =>
    request(`/api/account-tokens/sync/${accountId}`, {
      method: 'POST',
      timeoutMs: 45_000,
    }),

  // Check-in
  // skipBusinessError: the trigger answers 200 with `success:(failed==0)` + a
  // per-account summary. Both callers (checkin page, scheduling section)
  // render that breakdown themselves, so the generic interceptor toast — whose
  // message is the always-"执行完成" envelope text and would read as success
  // even on partial failure — is suppressed in favour of the precise feedback.
  triggerCheckinAll: () =>
    request('/api/checkin/trigger', {
      method: 'POST',
      skipBusinessError: true,
    }),
  triggerCheckin: (id: number) =>
    request(`/api/checkin/trigger/${id}`, { method: 'POST' }),
  getCheckinLogs: (
    params?: Record<string, string | number | boolean | null | undefined>
  ) => request(`/api/checkin/logs${buildQueryString(params)}`),

  // Routes
  getRoutesSummary: () => request('/api/routes/summary'),
  /**
   * Key-context admission + route explanation: evaluates one downstream key's
   * admission chain read-only (no counters, no quota consumption, no upstream
   * call) and explains route selection under that key's policy.
   */
  getRouteDecisionKeyContext: (input: {
    keyId: number
    model: string
    endpoint?: string
    clientIp?: string
  }) => {
    const params = new URLSearchParams()
    params.set('keyId', String(input.keyId))
    params.set('model', input.model)
    if (input.endpoint) params.set('endpoint', input.endpoint)
    if (input.clientIp) params.set('clientIp', input.clientIp)
    return request<KeyContextDecisionResponse>(
      `/api/routes/decision/key-context?${params.toString()}`
    )
  },
  getRouteDecision: (input: { model: string; endpoint: string }) =>
    request<{ success: boolean; decision: RouteDecision }>(
      `/api/routes/decision${buildQueryString(input)}`
    ),
  getRouteChannels: (routeId: number) =>
    request(`/api/routes/${routeId}/channels`),
  getChannels: (options?: {
    page?: number
    pageSize?: number
    status?: string
    refresh?: boolean
  }) => request(`/api/channels${buildQueryString(options)}`),
  getChannelsErrorSummary: () => request('/api/channels/error-summary'),
  batchAddChannels: (
    routeId: number,
    channels: Array<{
      accountId: number
      tokenId?: number
      sourceModel?: string
    }>
  ) =>
    request(`/api/routes/${routeId}/channels/batch`, {
      method: 'POST',
      body: JSON.stringify({ channels }),
    }),
  addRoute: (data: unknown) =>
    request('/api/routes', { method: 'POST', body: JSON.stringify(data) }),
  updateRoute: (id: number, data: unknown) =>
    request(`/api/routes/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  deleteRoute: (id: number) =>
    request(`/api/routes/${id}`, { method: 'DELETE' }),
  clearRouteCooldown: (id: number) =>
    request(`/api/routes/${id}/cooldown/clear`, { method: 'POST' }),
  batchUpdateRoutes: (data: { ids: number[]; action: 'enable' | 'disable' }) =>
    request('/api/routes/batch', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  updateChannel: (id: number, data: unknown) =>
    request(`/api/channels/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  // Partial-update semantics: each item applies only the fields present
  // (priority / weight / enabled). The 200 envelope carries per-item truth
  // (successIds + failedItems); skipBusinessError keeps a partial failure
  // (success:false) from tripping the generic interceptor toast so the
  // caller renders the precise per-item breakdown instead.
  batchUpdateChannels: (
    updates: Array<{
      id: number
      priority?: number
      weight?: number
      enabled?: boolean
    }>
  ) =>
    request<BatchUpdateChannelsResult>('/api/channels/batch', {
      method: 'PUT',
      body: JSON.stringify({ updates }),
      skipBusinessError: true,
    }),
  deleteChannel: (id: number) =>
    request(`/api/channels/${id}`, { method: 'DELETE' }),
  rebuildRoutes: (refreshModels = true, wait = false) =>
    request('/api/routes/rebuild', {
      method: 'POST',
      body: JSON.stringify({
        refreshModels,
        wait,
      }),
      ...(wait ? { timeoutMs: REBUILD_ROUTES_TIMEOUT_MS } : {}),
    }),
  refreshRouteDecisionSnapshots: () =>
    request('/api/routes/decision/refresh', {
      method: 'POST',
      body: JSON.stringify({}),
    }),
}
