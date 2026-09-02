// Cache-contract tests for the import commit mutation.
//
// The wizard can be opened from /sites AND from /accounts, and both hosts keep
// their table mounted while it runs. The accounts table reads the server-paged
// key (`accountQueryKeys.page(…)`) — the snapshot key has no observer there —
// so invalidating only `accountQueryKeys.snapshot()` left the mounted reader
// untouched: an import "succeeded" and the table did not change until the user
// edited a filter or navigated away and back (no refetchOnWindowFocus, no
// polling, and staleTime expiry alone never triggers a fetch).
//
// These tests pin the contract at the cache layer: after the mutation settles,
// every cached variant of both resources is invalidated, and a mounted paged
// reader actually refetches so imported rows appear.

import '@testing-library/jest-dom/vitest'
import {
  QueryClient,
  QueryClientProvider,
  useQuery,
} from '@tanstack/react-query'
import { act, renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { accountQueryKeys, fetchAccountsPage } from '@/features/accounts'
import { sitesKeys } from '@/features/sites'
import { api } from '@/lib/api'

import { useImportSites } from '../api'
import type { ImportSitesPayload } from '../types'

vi.mock('@/lib/api', () => ({
  api: {
    importSites: vi.fn(),
    getAccountsPage: vi.fn(),
  },
}))

const mockImportSites = vi.mocked(api.importSites)
const mockGetAccountsPage = vi.mocked(api.getAccountsPage)

/** Table state the /accounts route owns: first page, no filters. */
const PAGE_PARAMS = {
  pageIndex: 0,
  pageSize: 20,
  q: '',
  status: '',
  site: '',
}
const PAGE_KEY = accountQueryKeys.page(
  PAGE_PARAMS.pageIndex,
  PAGE_PARAMS.pageSize,
  PAGE_PARAMS.q,
  PAGE_PARAMS.status,
  PAGE_PARAMS.site
)

const IMPORT_PAYLOAD: ImportSitesPayload = {
  items: [{ name: 'Imported', url: 'https://imported.example.com' }],
  duplicateStrategy: 'skip',
}

const IMPORT_RESULT = {
  imported: 1,
  skipped: 0,
  failed: 0,
  results: [{ name: 'Imported', status: 'imported' as const, siteId: 2 }],
}

function pageResponse(usernames: string[]) {
  return {
    items: usernames.map((username, index) => ({
      id: index + 1,
      siteId: 1,
      username,
    })),
    total: usernames.length,
    sites: [],
    generatedAt: '2026-09-02T00:00:00Z',
  }
}

describe('useImportSites — cache invalidation', () => {
  let queryClient: QueryClient

  function wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )
  }

  beforeEach(() => {
    vi.clearAllMocks()
    mockImportSites.mockResolvedValue(IMPORT_RESULT)
    queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false, staleTime: 0, refetchOnWindowFocus: false },
        mutations: { retry: false },
      },
    })
  })

  it('invalidates the paged accounts key the table reads, plus sites', async () => {
    // Seed every cached variant of the two resources the import touches:
    // the server-paged table page (the /accounts read path), the snapshot
    // (its own consumers on other pages) and the sites list.
    queryClient.setQueryData(PAGE_KEY, pageResponse(['existing']))
    queryClient.setQueryData(accountQueryKeys.snapshot(), {
      generatedAt: '2026-09-02T00:00:00Z',
      accounts: [{ id: 1, siteId: 1, username: 'existing' }],
      sites: [],
    })
    queryClient.setQueryData(sitesKeys.list(), [{ id: 1, name: 'Existing' }])
    queryClient.setQueryData(sitesKeys.detail(1), { id: 1, name: 'Existing' })

    const { result } = renderHook(() => useImportSites(), { wrapper })
    await act(async () => {
      result.current.mutate(IMPORT_PAYLOAD)
    })
    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true)
    })

    // The regression: this key stayed untouched while only `snapshot()` was
    // invalidated, so the mounted table never learned about the import.
    expect(
      queryClient.getQueryState(PAGE_KEY)?.isInvalidated,
      'paged accounts cache must be invalidated'
    ).toBe(true)
    expect(
      queryClient.getQueryState(accountQueryKeys.snapshot())?.isInvalidated,
      'accounts snapshot must stay invalidated for its own consumers'
    ).toBe(true)
    expect(queryClient.getQueryState(sitesKeys.list())?.isInvalidated).toBe(
      true
    )
    expect(queryClient.getQueryState(sitesKeys.detail(1))?.isInvalidated).toBe(
      true
    )
  })

  it('refetches a mounted accounts page so imported rows appear', async () => {
    mockGetAccountsPage.mockResolvedValue(pageResponse(['existing']))

    const { result } = renderHook(
      () => ({
        // Same key + queryFn the /accounts table uses (useAccountsPage).
        page: useQuery({
          queryKey: PAGE_KEY,
          queryFn: () => fetchAccountsPage(PAGE_PARAMS),
          staleTime: 10 * 1000,
        }),
        importSites: useImportSites(),
      }),
      { wrapper }
    )

    await waitFor(() => {
      expect(result.current.page.isSuccess).toBe(true)
    })
    expect(result.current.page.data?.items).toHaveLength(1)
    expect(mockGetAccountsPage).toHaveBeenCalledTimes(1)

    // The backend now returns the imported account on the same page.
    mockGetAccountsPage.mockResolvedValue(
      pageResponse(['existing', 'imported'])
    )

    await act(async () => {
      result.current.importSites.mutate(IMPORT_PAYLOAD)
    })
    await waitFor(() => {
      expect(result.current.importSites.isSuccess).toBe(true)
    })

    // Invalidation must reach the ACTIVE observer — a refetch, not just a
    // stale flag — otherwise the table keeps rendering the pre-import page.
    await waitFor(() => {
      expect(mockGetAccountsPage).toHaveBeenCalledTimes(2)
    })
    await waitFor(() => {
      expect(result.current.page.data?.items).toHaveLength(2)
    })
  })
})
