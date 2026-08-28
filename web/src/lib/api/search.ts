import { request } from './transport'

export type SearchSite = {
  id: number
  name: string
  url?: string | null
  platform?: string | null
  status?: string | null
}

export type SearchAccount = {
  id: number
  username?: string | null
  siteName?: string | null
  sitePlatform?: string | null
}

export type SearchAccountToken = {
  id: number
  name: string
  tokenMasked?: string | null
  /** Owning account id — already returned by the backend (`at.account_id`),
   * lets the palette deep-link straight to the account detail sheet. */
  accountId?: number | null
  accountUsername?: string | null
  siteName?: string | null
}

export type SearchCheckinLog = {
  id: number
  accountId?: number | null
  accountUsername?: string | null
  status?: string | null
  message?: string | null
  reward?: string | null
  createdAt?: string | null
}

export type SearchProxyLog = {
  id: number
  modelRequested?: string | null
  modelActual?: string | null
  status?: string | null
  latencyMs?: number | null
  totalTokens?: number | null
  errorMessage?: string | null
  createdAt?: string | null
}

export type SearchModel = {
  modelName: string
  tokenCount?: number | null
  accountCount?: number | null
  siteCount?: number | null
}

export type SearchResponse = {
  sites: SearchSite[]
  accounts: SearchAccount[]
  accountTokens: SearchAccountToken[]
  checkinLogs: SearchCheckinLog[]
  proxyLogs: SearchProxyLog[]
  models: SearchModel[]
}

export type SearchOptions = {
  signal?: AbortSignal
  /** Per-category result cap passed to the backend (clamped to 10 server-side). */
  limit?: number
}

// The backend splits `limit` evenly across the six categories, so ask for
// 6 × 10 to get the maximum of 10 hits per category (server-side cap) and
// let the palette trim to its own display cap.
const SEARCH_REQUEST_LIMIT = 60

export const searchApi = {
  // Search
  search: (query: string, options?: SearchOptions) =>
    request<SearchResponse>('/api/search', {
      method: 'POST',
      body: JSON.stringify({
        query,
        limit: options?.limit ?? SEARCH_REQUEST_LIMIT,
      }),
      signal: options?.signal,
    }),
}
