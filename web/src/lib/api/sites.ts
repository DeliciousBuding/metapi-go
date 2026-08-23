import { request, streamSse } from './transport'

/**
 * One model outcome from the site probe pass (probe-now / probe-stream).
 * `status` mirrors the backend vocabulary honestly — 'error' is a probe
 * machinery failure (target load error), not a success or a failure.
 */
export type SiteProbeResult = {
  channelId: number
  accountId: number
  model: string
  status: 'success' | 'failure' | 'error'
  latencyMs: number
  error?: string
}

/** Incremental probe-stream complete payload. */
export type SiteProbeCompletePayload = {
  totalModels: number
  available: number
  unavailable: number
  truncated?: boolean
  reason?: string
}

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
  /**
   * Live probe pass (GET /api/sites/{id}/probe-stream, SSE). Each run of the
   * stream performs its own probe pass; results arrive incrementally as
   * `probe-result` events and finish with a `complete` event carrying the
   * honest totals (including truncated + reason when the pass was cut short).
   */
  streamSiteProbe: (
    siteId: number,
    handlers: {
      onStart?: () => void
      onResult?: (result: SiteProbeResult) => void
      onComplete?: (payload: SiteProbeCompletePayload) => void
      signal?: AbortSignal
    }
  ) =>
    streamSse(`/api/sites/${siteId}/probe-stream`, {
      signal: handlers.signal,
      onEvent: (event, payload) => {
        if (event === 'probe-start') {
          handlers.onStart?.()
        } else if (event === 'probe-result') {
          handlers.onResult?.(payload as SiteProbeResult)
        } else if (event === 'complete') {
          handlers.onComplete?.(payload as SiteProbeCompletePayload)
        }
      },
    }),
}
