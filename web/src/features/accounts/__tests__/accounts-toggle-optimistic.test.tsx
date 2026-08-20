// Behavior tests for the accounts row-toggle optimistic updates (issue #889
// perf section). Pin / check-in / status toggles previously waited for the
// server round-trip before the row flipped; they now follow the sites
// feature pattern: onMutate patches the snapshot cache (instant flip),
// onError rolls the cache back to the pre-mutation snapshot, and onSettled
// invalidates so the server truth wins.
//
// The real api.updateAccount is stubbed (network boundary); a controllable
// deferred promise lets each case assert the cache state BEFORE the server
// answers, then drive success / failure.

import '@testing-library/jest-dom/vitest'
import {
  QueryClient,
  QueryClientProvider,
} from '@tanstack/react-query'
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
  queryClient.setQueryData(accountQueryKeys.snapshot(), buildSnapshot())
  return queryClient
}

function accountField(
  queryClient: QueryClient,
  accountId: number,
  field: 'isPinned' | 'checkinEnabled' | 'status'
): unknown {
  const snapshot = queryClient.getQueryData<AccountsSnapshot>(
    accountQueryKeys.snapshot()
  )
  return snapshot?.accounts.find((account) => account.id === accountId)?.[
    field
  ]
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

  it('flips isPinned in the cache before the server answers', async () => {
    const deferred = createDeferred()
    mockUpdateAccount.mockReturnValue(deferred.promise)

    const { result } = renderHook(() => useToggleAccountPin(), { wrapper })

    act(() => {
      result.current.mutate({ id: 1, isPinned: true })
    })

    // Optimistic: the cache flips while the request is still in flight.
    await waitFor(() => {
      expect(accountField(queryClient, 1, 'isPinned')).toBe(true)
    })
    // Sibling rows stay untouched.
    expect(accountField(queryClient, 2, 'isPinned')).toBe(true)

    await act(async () => {
      deferred.resolve({ success: true })
    })
    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true)
    })
    expect(accountField(queryClient, 1, 'isPinned')).toBe(true)
  })

  it('rolls the pin flip back when the request fails', async () => {
    const deferred = createDeferred()
    mockUpdateAccount.mockReturnValue(deferred.promise)

    const { result } = renderHook(() => useToggleAccountPin(), { wrapper })

    act(() => {
      result.current.mutate({ id: 1, isPinned: true }, { onError: () => {} })
    })

    await waitFor(() => {
      expect(accountField(queryClient, 1, 'isPinned')).toBe(true)
    })

    await act(async () => {
      deferred.reject(new Error('boom'))
    })
    await waitFor(() => {
      expect(result.current.isError).toBe(true)
    })
    // Rolled back to the pre-mutation snapshot.
    expect(accountField(queryClient, 1, 'isPinned')).toBe(false)
    expect(accountField(queryClient, 2, 'isPinned')).toBe(true)
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
      expect(accountField(queryClient, 1, 'checkinEnabled')).toBe(true)
    })

    await act(async () => {
      deferred.reject(new Error('boom'))
    })
    await waitFor(() => {
      expect(result.current.isError).toBe(true)
    })
    expect(accountField(queryClient, 1, 'checkinEnabled')).toBe(false)
    // Unrelated rows keep their value through the rollback.
    expect(accountField(queryClient, 2, 'checkinEnabled')).toBe(true)
  })

  it('flips the account status optimistically and keeps it on success', async () => {
    const deferred = createDeferred()
    mockUpdateAccount.mockReturnValue(deferred.promise)

    const { result } = renderHook(() => useToggleAccountStatus(), { wrapper })

    act(() => {
      result.current.mutate({ id: 2, status: 'disabled' })
    })

    await waitFor(() => {
      expect(accountField(queryClient, 2, 'status')).toBe('disabled')
    })

    await act(async () => {
      deferred.resolve({ success: true })
    })
    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true)
    })
    expect(accountField(queryClient, 2, 'status')).toBe('disabled')
    expect(accountField(queryClient, 1, 'status')).toBe('active')
  })
})
