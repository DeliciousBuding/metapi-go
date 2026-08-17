import { request } from './transport'

export const sitesApi = {
  // Sites
  getSites: () => request('/api/sites'),
  addSite: (data: unknown) =>
    request('/api/sites', { method: 'POST', body: JSON.stringify(data) }),
  updateSite: (id: number, data: unknown) =>
    request(`/api/sites/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  deleteSite: (id: number) => request(`/api/sites/${id}`, { method: 'DELETE' }),
  batchUpdateSites: (data: unknown) =>
    request('/api/sites/batch', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  detectSite: (url: string) =>
    request('/api/sites/detect', {
      method: 'POST',
      body: JSON.stringify({ url }),
      skipErrorHandler: true,
    }),
  importSites: (data: unknown) =>
    request('/api/sites/import', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  getSiteDisabledModels: (siteId: number) =>
    request(`/api/sites/${siteId}/disabled-models`),
  updateSiteDisabledModels: (siteId: number, models: string[]) =>
    request(`/api/sites/${siteId}/disabled-models`, {
      method: 'PUT',
      body: JSON.stringify({ models }),
    }),
  getSiteAvailableModels: (siteId: number) =>
    request(`/api/sites/${siteId}/available-models`),
  probeSiteNow: (
    siteId: number,
    options?: {
      scope?: 'single' | 'all'
      modelName?: string
      latencyThresholdMs?: number
    }
  ) =>
    request(`/api/sites/${siteId}/probe-now`, {
      method: 'POST',
      body: JSON.stringify(options || {}),
      timeoutMs: options?.scope === 'all' ? 120_000 : 30_000,
    }),
}
