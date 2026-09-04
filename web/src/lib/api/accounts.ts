import { request, buildQueryString } from './transport'

export const accountsApi = {
  getAccountsSnapshot: (options?: { refresh?: boolean }) =>
    request(
      `/api/accounts${buildQueryString(options?.refresh ? { refresh: 1 } : undefined)}`
    ) as Promise<{
      generatedAt: string
      accounts: unknown[]
      sites: unknown[]
    }>,
  getAccountsPage: (params: {
    page: number
    pageSize: number
    /** Server-side global search (matches username/site name/platform/url). */
    q?: string
    /** Comma-separated status whitelist (active/disabled/expired). */
    status?: string
    /** Comma-separated site ids. */
    site?: string
  }) =>
    request(
      `/api/accounts${buildQueryString({
        page: params.page,
        pageSize: params.pageSize,
        q: params.q || undefined,
        status: params.status || undefined,
        site: params.site || undefined,
      })}`
    ) as Promise<{
      items?: unknown[]
      total?: number
      page?: number
      pageSize?: number
      generatedAt?: string
      sites?: unknown[]
    }>,
  addAccount: (data: unknown) =>
    request('/api/accounts', { method: 'POST', body: JSON.stringify(data) }),
  loginAccount: (data: {
    siteId: number
    username: string
    password: string
  }) =>
    request('/api/accounts/login', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  verifyToken: (
    data: {
      siteId: number
      accessToken: string
      platformUserId?: number
      proxyUrl?: string
      credentialMode?: 'auto' | 'session' | 'apikey'
    },
    options?: { skipErrorHandler?: boolean }
  ) =>
    request('/api/accounts/verify-token', {
      method: 'POST',
      body: JSON.stringify(data),
      skipErrorHandler: options?.skipErrorHandler,
    }),
  updateAccount: (id: number, data: unknown) =>
    request(`/api/accounts/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  deleteAccount: (id: number) =>
    request(`/api/accounts/${id}`, { method: 'DELETE' }),
  batchUpdateAccounts: (data: unknown) =>
    request('/api/accounts/batch', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  refreshBalance: (id: number) =>
    request(`/api/accounts/${id}/balance`, { method: 'POST' }),
}
