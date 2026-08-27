import { request } from './transport'

// Read-only probe-history endpoints backing the row-level probe health bars
// on the channels/accounts pages (handler/admin/probe_history.go). One batch
// call per page render — never per row.
export const probeHistoryApi = {
  getChannelProbeHistory: (limit: number) =>
    request(`/api/channels/probe-history?limit=${limit}`),
  getAccountProbeHistory: (limit: number) =>
    request(`/api/accounts/probe-history?limit=${limit}`),
}
