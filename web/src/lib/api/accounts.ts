import { request, buildQueryString } from './transport'

export const accountsApi = {
  // Accounts
  getAccounts: async (params?: { includeOauth?: boolean }) => {
    const result = await request<{ accounts?: unknown[] } | unknown[]>(
      `/api/accounts${buildQueryString(params)}`
    )
    const accounts = Array.isArray(result) ? result : result?.accounts
    return Array.isArray(accounts) ? accounts : result
  },
  getAccountsSnapshot: (options?: { refresh?: boolean }) =>
    request(
      `/api/accounts${buildQueryString(options?.refresh ? { refresh: 1 } : undefined)}`
    ) as Promise<{
      generatedAt: string
      accounts: unknown[]
      sites: unknown[]
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
  verifyToken: (data: {
    siteId: number
    accessToken: string
    platformUserId?: number
    credentialMode?: 'auto' | 'session' | 'apikey'
  }) =>
    request('/api/accounts/verify-token', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  rebindAccountSession: (
    id: number,
    data: {
      accessToken: string
      platformUserId?: number
      refreshToken?: string
      tokenExpiresAt?: number
    }
  ) =>
    request(`/api/accounts/${id}/rebind-session`, {
      method: 'POST',
      body: JSON.stringify(data),
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
  getAccountModels: (id: number) => request(`/api/accounts/${id}/models`),
  addAccountAvailableModels: (accountId: number, models: string[]) =>
    request(`/api/accounts/${accountId}/models/manual`, {
      method: 'POST',
      body: JSON.stringify({ models }),
    }),
  refreshAccountHealth: (data?: { accountId?: number; wait?: boolean }) =>
    request('/api/accounts/health/refresh', {
      method: 'POST',
      body: JSON.stringify(data || {}),
      timeoutMs: data?.wait ? 150_000 : 30_000,
    }),
}
