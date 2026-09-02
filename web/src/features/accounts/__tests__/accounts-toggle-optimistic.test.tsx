// Behavior tests for the accounts row-toggle optimistic updates (issue #889
// perf section). Pin / check-in / status toggles previously waited for the
// server round-trip before the row flipped; they now patch the cache in
// onMutate (instant flip), roll every patched key back in onError, and
// invalidate in onSettled so the server truth wins.
//
// The cache under test is the one the /accounts table ACTUALLY reads:
// `accountQueryKeys.page(…)`. `page` and `snapshot` are sibling keys, so a
// snapshot-only patch never reached the table (the row only moved after the
// settle refetch) — and a snapshot-only test stayed green while it did. Both
// caches are seeded and asserted here, with the paged one first.
//
// The real api.updateAccount is stubbed (network boundary); a controllable
// deferred promise lets each case assert the cache state BEFORE the server
// answers, then drive success / failure.

import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'
import { api } from '@/lib/api'

import {
  accountQueryKeys,
  useToggleAccountCheckin,
  useToggleAccountPin,
  useToggleAccountStatus,
  type AccountsPageData,
} from '../api'
import { accountSchema, type AccountsSnapshot } from '../types'

vi.mock('@/lib/api', () => ({
  api: {
    updateAccount: vi.fn(),
  },
}))

vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() },
}))

const mockUpdateAccount = vi.mocked(api.updateAccount)

/**
 * Two cached pages of the same fleet (different pagination + filters): the
 * optimistic patch must reach EVERY page under the `['accounts','page']`
 * prefix, because any of them can be the mounted table.
 */
const PAGE_KEYS = [
  accountQueryKeys.page(0, 20, '', '', ''),
  accountQueryKeys.page(1, 50, 'second', 'active', '1'),
] as const

type ToggleField = 'isPinned' | 'checkinEnabled' | 'status'

const ACCOUNT_ROWS: Array<Record<string, unknown>> = [
  {
    id: 1,
    siteId: 1,
    username: 'first',
    isPinned: false,
    checkinEnabled: false,
    status: 'active',
  },
  {
    id: 2,
    siteId: 1,
    username: 'second',
    isPinned: true,
    checkinEnabled: true,
    status: 'active',
  },
]

function buildPage(): AccountsPageData {
  return {
    items: ACCOUNT_ROWS.map((row) => ({ ...row })),
    total: ACCOUNT_ROWS.length,
    sites: [],
    generatedAt: '2026-08-20T00:00:00Z',
  }
}

function buildSnapshot(): AccountsSnapshot {
  return {
    generatedAt: '2026-08-20T00:00:00Z',
    accounts: [
      accountSchema.parse({ id: 1, siteId: 1, username: 'first' }),
      accountSchema.parse({
        id: 2,
        siteId: 1,
        username: 'second',
        isPinned: true,
        checkinEnabled: true,
        status: 'active',
      }),
    ],
    sites: [],
  }
}

function createQueryClient() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: 0, refetchOnWindowFocus: false },
      mutations: { retry: false },
    },
  })
  for (const pageKey of PAGE_KEYS) {
    queryClient.setQueryData(pageKey, buildPage())
  }
  queryClient.setQueryData(accountQueryKeys.snapshot(), buildSnapshot())
  return queryClient
}

/** Field value in a cached server page — the /accounts table's read path. */
function pageField(
  queryClient: QueryClient,
  pageKey: readonly unknown[],
  accountId: number,
  field: ToggleField
): unknown {
  const page = queryClient.getQueryData<AccountsPageData>(pageKey)
  const row = page?.items.find(
    (item) => Number((item as { id?: unknown }).id) === accountId
  ) as Record<string, unknown> | undefined
  return row?.[field]
}

/** Field value in the snapshot cache (its own consumers on other pages). */
function snapshotField(
  queryClient: QueryClient,
  accountId: number,
  field: ToggleField
): unknown {
  const snapshot = queryClient.getQueryData<AccountsSnapshot>(
    accountQueryKeys.snapshot()
  )
  return snapshot?.accounts.find((account) => account.id === accountId)?.[field]
}

/** Deferred promise so cases can inspect the optimistic cache mid-flight. */
function createDeferred() {
  let resolve!: (value: { success: boolean }) => void
  let reject!: (error: Error) => void
  const promise = new Promise<{ success: boolean }>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

describe('accounts toggle mutations — optimistic updates', () => {
  let queryClient: QueryClient

  function wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )
  }

  beforeEach(() => {
    mockUpdateAccount.mockReset()
    queryClient = createQueryClient()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('flips isPinned in every cached accounts page before the server answers', async () => {
    const deferred = createDeferred()
    mockUpdateAccount.mockReturnValue(deferred.promise)

    const { result } = renderHook(() => useToggleAccountPin(), { wrapper })

    act(() => {
      result.current.mutate({ id: 1, isPinned: true })
    })

    // Optimistic: the paged cache the table renders flips while the request is
    // still in flight — and it flips on EVERY cached page, not just one.
    await waitFor(() => {
      for (const pageKey of PAGE_KEYS) {
        expect(pageField(queryClient, pageKey, 1, 'isPinned')).toBe(true)
      }
    })
    // Sibling rows stay untouched.
    for (const pageKey of PAGE_KEYS) {
      expect(pageField(queryClient, pageKey, 2, 'isPinned')).toBe(true)
    }
    // The snapshot (other pages' consumer) stays coherent with the table.
    expect(snapshotField(queryClient, 1, 'isPinned')).toBe(true)

    await act(async () => {
      deferred.resolve({ success: true })
    })
    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true)
    })
    for (const pageKey of PAGE_KEYS) {
      expect(pageField(queryClient, pageKey, 1, 'isPinned')).toBe(true)
    }
  })

  it('rolls the pin flip back in the paged caches when the request fails', async () => {
    const deferred = createDeferred()
    mockUpdateAccount.mockReturnValue(deferred.promise)

    const { result } = renderHook(() => useToggleAccountPin(), { wrapper })

    act(() => {
      result.current.mutate({ id: 1, isPinned: true }, { onError: () => {} })
    })

    await waitFor(() => {
      expect(pageField(queryClient, PAGE_KEYS[0], 1, 'isPinned')).toBe(true)
    })

    await act(async () => {
      deferred.reject(new Error('boom'))
    })
    await waitFor(() => {
      expect(result.current.isError).toBe(true)
    })
    // Rolled back to the pre-mutation payload on every patched page.
    for (const pageKey of PAGE_KEYS) {
      expect(pageField(queryClient, pageKey, 1, 'isPinned')).toBe(false)
      expect(pageField(queryClient, pageKey, 2, 'isPinned')).toBe(true)
    }
    expect(snapshotField(queryClient, 1, 'isPinned')).toBe(false)
  })

  it('flips checkinEnabled optimistically and rolls back on failure', async () => {
    const deferred = createDeferred()
    mockUpdateAccount.mockReturnValue(deferred.promise)

    const { result } = renderHook(() => useToggleAccountCheckin(), { wrapper })

    act(() => {
      result.current.mutate(
        { id: 1, checkinEnabled: true },
        { onError: () => {} }
      )
    })

    await waitFor(() => {
      for (const pageKey of PAGE_KEYS) {
        expect(pageField(queryClient, pageKey, 1, 'checkinEnabled')).toBe(true)
      }
    })

    await act(async () => {
      deferred.reject(new Error('boom'))
    })
    await waitFor(() => {
      expect(result.current.isError).toBe(true)
    })
    for (const pageKey of PAGE_KEYS) {
      expect(pageField(queryClient, pageKey, 1, 'checkinEnabled')).toBe(false)
      // Unrelated rows keep their value through the rollback.
      expect(pageField(queryClient, pageKey, 2, 'checkinEnabled')).toBe(true)
    }
    expect(snapshotField(queryClient, 1, 'checkinEnabled')).toBe(false)
  })

  it('flips the account status optimistically and keeps it on success', async () => {
    const deferred = createDeferred()
    mockUpdateAccount.mockReturnValue(deferred.promise)

    const { result } = renderHook(() => useToggleAccountStatus(), { wrapper })

    act(() => {
      result.current.mutate({ id: 2, status: 'disabled' })
    })

    await waitFor(() => {
      for (const pageKey of PAGE_KEYS) {
        expect(pageField(queryClient, pageKey, 2, 'status')).toBe('disabled')
      }
    })

    await act(async () => {
      deferred.resolve({ success: true })
    })
    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true)
    })
    for (const pageKey of PAGE_KEYS) {
      expect(pageField(queryClient, pageKey, 2, 'status')).toBe('disabled')
      expect(pageField(queryClient, pageKey, 1, 'status')).toBe('active')
    }
    expect(snapshotField(queryClient, 2, 'status')).toBe('disabled')
  })

  it('leaves an untouched page cache alone (no row is invented)', async () => {
    const deferred = createDeferred()
    mockUpdateAccount.mockReturnValue(deferred.promise)

    // A page that does not contain the toggled account must keep its payload
    // byte-for-byte (identity), so unrelated tables do not re-render.
    const otherPageKey = accountQueryKeys.page(3, 20, '', 'disabled', '')
    const otherPage: AccountsPageData = {
      items: [{ id: 99, siteId: 2, username: 'other' }],
      total: 1,
      sites: [],
      generatedAt: '2026-08-20T00:00:00Z',
    }
    queryClient.setQueryData(otherPageKey, otherPage)

    const { result } = renderHook(() => useToggleAccountStatus(), { wrapper })

    act(() => {
      result.current.mutate({ id: 2, status: 'disabled' })
    })

    await waitFor(() => {
      expect(pageField(queryClient, PAGE_KEYS[0], 2, 'status')).toBe('disabled')
    })
    expect(queryClient.getQueryData(otherPageKey)).toBe(otherPage)
    expect(
      (queryClient.getQueryData<AccountsPageData>(otherPageKey)?.items ?? [])[0]
    ).toEqual({ id: 99, siteId: 2, username: 'other' })
  })
})
