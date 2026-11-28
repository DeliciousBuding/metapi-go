// metapi-go features/accounts/account-queries — TanStack Query hooks for the accounts
// domain. Establishes the query-key conventions for the rewrite:
//   ['accounts']                — factory root; the invalidation prefix
//   ['accounts','snapshot']     — snapshot list (GET /api/accounts)
//   ['accounts','page',{…}]     — one server-paged table page: what the
//                                 /accounts table actually renders
//   ['accounts','detail',id]    — a single account
//   ['account-tokens', id]      — tokens for an account (see tokens/api.ts)
// `page` and `snapshot` are siblings, so cache writes that must reach the
// table go through `accountQueryKeys.pages()` (or `.all`), never `.snapshot()`.

import { useQuery, type UseQueryOptions } from '@tanstack/react-query'

import { api } from '@/lib/api'

import type { Site, AccountsSnapshot } from './types'

export const accountQueryKeys = {
  all: ['accounts'] as const,
  snapshot: () => [...accountQueryKeys.all, 'snapshot'] as const,
  /**
   * Prefix of every server-paged table cache produced by `page(…)`.
   *
   * `page` and `snapshot` are SIBLINGS under `all`, so neither is a prefix of
   * the other: anything that has to reach the /accounts table (invalidation,
   * optimistic patching) targets `all` or `pages()` — never `snapshot()`
   * alone, which the table does not read.
   */
  pages: () => [...accountQueryKeys.all, 'page'] as const,
  page: (
    pageIndex: number,
    pageSize: number,
    q?: string,
    status?: string,
    site?: string
  ) =>
    [
      ...accountQueryKeys.all,
      'page',
      {
        pageIndex,
        pageSize,
        q: q || undefined,
        status: status || undefined,
        site: site || undefined,
      },
    ] as const,
  detail: (id: number) => ['accounts', 'detail', id] as const,
}

// ---------------------------------------------------------------------------
// useAccountsPage — one server-side page of the accounts/sites projection
// ---------------------------------------------------------------------------

/** One page returned by GET /api/accounts?page=&pageSize=. */
export type AccountsPageData = {
  /** Legacy snapshot-shaped fixture compatibility; production pages use items. */
  accounts?: object[]
  items: object[]
  total: number
  sites: Site[]
  generatedAt: string
}

/**
 * Fetch + shape one server-side page. The page-gated endpoint keeps the same
 * account/site projection as the snapshot and slices before JSON encoding,
 * so large fleets no longer transfer every row to the browser. `q` / `status`
 * / `site` are pushed server-side (#1108): the filters match the whole fleet,
 * not just the loaded page. A missing or malformed total degrades to the
 * returned page length (the pager then shows one page rather than an
 * invented total).
 */
export async function fetchAccountsPage(params: {
  pageIndex: number
  pageSize: number
  q: string
  status: string
  site: string
}): Promise<AccountsPageData> {
  const response = await api.getAccountsPage({
    page: params.pageIndex + 1,
    pageSize: params.pageSize,
    q: params.q,
    status: params.status,
    site: params.site,
  })
  const items = Array.isArray(response.items)
    ? (response.items as object[])
    : []
  const sites = Array.isArray(response.sites) ? (response.sites as Site[]) : []
  return {
    items,
    sites,
    generatedAt:
      typeof response.generatedAt === 'string' ? response.generatedAt : '',
    total:
      typeof response.total === 'number' && Number.isFinite(response.total)
        ? response.total
        : items.length,
  }
}

/**
 * Fetch one server-side accounts page by URL-owned table state. The query is
 * keyed by pageIndex/pageSize AND the server-side filters (q/status/site), so
 * a filter change refetches a freshly-filtered page from the backend (#1108).
 */
export function useAccountsPage(
  params: {
    pageIndex: number
    pageSize: number
    q: string
    status: string
    site: string
  },
  options?: Omit<UseQueryOptions<AccountsPageData>, 'queryKey' | 'queryFn'>
) {
  return useQuery<AccountsPageData>({
    queryKey: accountQueryKeys.page(
      params.pageIndex,
      params.pageSize,
      params.q,
      params.status,
      params.site
    ),
    queryFn: () => fetchAccountsPage(params),
    placeholderData: (previous) => previous,
    staleTime: 10 * 1000,
    ...options,
  })
}

// ---------------------------------------------------------------------------
// useAccounts — snapshot list (accounts + sites + generatedAt)
// ---------------------------------------------------------------------------

export function useAccounts(
  options?: Omit<UseQueryOptions<AccountsSnapshot>, 'queryKey' | 'queryFn'>
) {
  return useQuery({
    queryKey: accountQueryKeys.snapshot(),
    queryFn: async () => {
      const snapshot = await api.getAccountsSnapshot()
      if (!snapshot || !Array.isArray(snapshot.accounts)) {
        throw new Error('Failed to load accounts snapshot')
      }
      return snapshot as AccountsSnapshot
    },
    staleTime: 10 * 1000,
    ...options,
  })
}
