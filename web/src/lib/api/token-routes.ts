import { request } from './transport'

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
  batchUpdateAccountTokens: (data: unknown) =>
    request('/api/account-tokens/batch', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  getAccountTokenGroups: (accountId: number) =>
    request(`/api/account-tokens/groups/${accountId}`),
  setDefaultAccountToken: (id: number) =>
    request(`/api/account-tokens/${id}/default`, { method: 'POST' }),
  getAccountTokenValue: (id: number) =>
    request(`/api/account-tokens/${id}/value`),
  syncAccountTokens: (accountId: number) =>
    request(`/api/account-tokens/sync/${accountId}`, {
      method: 'POST',
      timeoutMs: 45_000,
    }),
  syncAllAccountTokens: (wait = false) =>
    request('/api/account-tokens/sync-all', {
      method: 'POST',
      body: JSON.stringify(wait ? { wait: true } : {}),
      timeoutMs: wait ? 150_000 : 30_000,
    }),

  // Check-in
  triggerCheckinAll: () => request('/api/checkin/trigger', { method: 'POST' }),
  triggerCheckin: (id: number) =>
    request(`/api/checkin/trigger/${id}`, { method: 'POST' }),
  getCheckinLogs: (params?: string) =>
    request(`/api/checkin/logs${params ? `?${params}` : ''}`),

  // Routes
  getRoutes: () => request('/api/routes'),
  getRoutesLite: () => request('/api/routes/lite'),
  getRoutesSummary: () => request('/api/routes/summary'),
  getRouteChannels: (routeId: number) =>
    request(`/api/routes/${routeId}/channels`),
  getChannels: () => request('/api/channels'),
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
  addChannel: (routeId: number, data: unknown) =>
    request(`/api/routes/${routeId}/channels`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  updateChannel: (id: number, data: unknown) =>
    request(`/api/channels/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  batchUpdateChannels: (updates: Array<{ id: number; priority: number }>) =>
    request('/api/channels/batch', {
      method: 'PUT',
      body: JSON.stringify({ updates }),
    }),
  deleteChannel: (id: number) =>
    request(`/api/channels/${id}`, { method: 'DELETE' }),
  rebuildRoutes: (refreshModels = true, wait = false) =>
    request('/api/routes/rebuild', {
      method: 'POST',
      body: JSON.stringify({
        refreshModels,
        ...(wait ? { wait: true } : {}),
      }),
      timeoutMs: wait ? 150_000 : 30_000,
    }),
  refreshRouteDecisionSnapshots: () =>
    request('/api/routes/decision/refresh', {
      method: 'POST',
      body: JSON.stringify({}),
    }),
  getRouteDecision: (model: string) =>
    request(`/api/routes/decision?model=${encodeURIComponent(model)}`),
  getRouteDecisionsBatch: (
    models: string[],
    options?: { refreshPricingCatalog?: boolean; persistSnapshots?: boolean }
  ) =>
    request('/api/routes/decision/batch', {
      method: 'POST',
      body: JSON.stringify({
        models,
        ...(options?.refreshPricingCatalog
          ? { refreshPricingCatalog: true }
          : {}),
        ...(options?.persistSnapshots ? { persistSnapshots: true } : {}),
      }),
    }),
  getRouteDecisionsByRouteBatch: (
    items: Array<{ routeId: number; model: string }>,
    options?: { refreshPricingCatalog?: boolean; persistSnapshots?: boolean }
  ) =>
    request('/api/routes/decision/by-route/batch', {
      method: 'POST',
      body: JSON.stringify({
        items,
        ...(options?.refreshPricingCatalog
          ? { refreshPricingCatalog: true }
          : {}),
        ...(options?.persistSnapshots ? { persistSnapshots: true } : {}),
      }),
    }),
  getRouteWideDecisionsBatch: (
    routeIds: number[],
    options?: { refreshPricingCatalog?: boolean; persistSnapshots?: boolean }
  ) =>
    request('/api/routes/decision/route-wide/batch', {
      method: 'POST',
      body: JSON.stringify({
        routeIds,
        ...(options?.refreshPricingCatalog
          ? { refreshPricingCatalog: true }
          : {}),
        ...(options?.persistSnapshots ? { persistSnapshots: true } : {}),
      }),
    }),
}
