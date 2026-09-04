import { request, buildQueryString } from './transport'
import type {
  OAuthProvidersResponse,
  OAuthStartResponse,
  OAuthSessionInfo,
  OAuthQuotaInfo,
  OAuthConnectionsResponse,
} from './types'

export const oauthApi = {
  // OAuth
  getOAuthProviders: () =>
    request('/api/oauth/providers') as Promise<OAuthProvidersResponse>,
  startOAuthProvider: (
    provider: string,
    data?: {
      accountId?: number
      projectId?: string
      proxyUrl?: string | null
      useSystemProxy?: boolean
    }
  ) =>
    request(`/api/oauth/providers/${encodeURIComponent(provider)}/start`, {
      method: 'POST',
      body: JSON.stringify(data || {}),
      // The start dialog surfaces its own specific startFailed message.
      skipErrorHandler: true,
    }) as Promise<OAuthStartResponse>,
  getOAuthSession: (state: string) =>
    request(
      `/api/oauth/sessions/${encodeURIComponent(state)}`
    ) as Promise<OAuthSessionInfo>,
  submitOAuthManualCallback: (state: string, callbackUrl: string) =>
    request(
      `/api/oauth/sessions/${encodeURIComponent(state)}/manual-callback`,
      {
        method: 'POST',
        body: JSON.stringify({ callbackUrl }),
        // The pending-session panel surfaces its own error toast.
        skipErrorHandler: true,
      }
    ) as Promise<{ success: true }>,
  getOAuthConnections: (params?: { limit?: number; offset?: number }) =>
    request(
      `/api/oauth/connections${buildQueryString(params)}`
    ) as Promise<OAuthConnectionsResponse>,
  refreshOAuthConnectionQuota: (accountId: number) =>
    request(`/api/oauth/connections/${accountId}/quota/refresh`, {
      method: 'POST',
      body: JSON.stringify({}),
      // The row action surfaces a per-account refreshFailed toast.
      skipErrorHandler: true,
    }) as Promise<{ success: true; quota: OAuthQuotaInfo }>,
  rebindOAuthConnection: (
    accountId: number,
    data?: { proxyUrl?: string | null; useSystemProxy?: boolean }
  ) =>
    request(`/api/oauth/connections/${accountId}/rebind`, {
      method: 'POST',
      body: JSON.stringify(data || {}),
      // The row action surfaces a per-account rebindFailed toast.
      skipErrorHandler: true,
    }) as Promise<OAuthStartResponse>,
  deleteOAuthConnection: (accountId: number) =>
    request(`/api/oauth/connections/${accountId}`, {
      method: 'DELETE',
    }) as Promise<{ success: true }>,
}
