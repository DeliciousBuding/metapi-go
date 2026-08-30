// metapi-go/lib — undoable-delete tests (S7 删除+undo 档).
//
// Pins the deferred-delete contract: optimistic removal on trigger, commit
// only when the window closes (auto-close or dismiss), snapshot restore on
// undo, snapshot restore + error toast on commit failure, and idempotence
// (undo after commit / commit after undo never double-fires).

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, renderHook } from '@testing-library/react'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  useUndoableDelete,
  type UndoableDeleteParams,
} from '../undoable-delete'

const { mockToastMessage, mockToastError, mockToastDismiss } = vi.hoisted(
  () => ({
    mockToastMessage: vi.fn((_title: unknown, _options?: unknown) => 42),
    mockToastError: vi.fn(),
    mockToastDismiss: vi.fn(),
  })
)

vi.mock('@/lib/toast', () => ({
  toast: {
    message: mockToastMessage,
    error: mockToastError,
    dismiss: mockToastDismiss,
  },
}))

type Row = { id: number; name: string }

const QUERY_KEY = ['rows'] as const
const ROWS: Row[] = [
  { id: 1, name: 'alpha' },
  { id: 2, name: 'beta' },
]

function setup() {
  const queryClient = new QueryClient()
  queryClient.setQueryData(QUERY_KEY, ROWS)
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  )
  const { result } = renderHook(() => useUndoableDelete(), { wrapper })
  return { queryClient, trigger: result.current }
}

function params(
  overrides: Partial<UndoableDeleteParams<Row[], Row>> = {}
): UndoableDeleteParams<Row[], Row> {
  return {
    item: ROWS[1] as Row,
    queryKey: QUERY_KEY,
    removeFromCache: (data: Row[], item: Row) =>
      data.filter((row) => row.id !== item.id),
    deleteFn: vi.fn(async () => ({ success: true })),
    title: 'Deleted',
    undoLabel: 'Undo',
    errorTitle: 'Delete failed',
    ...overrides,
  }
}

function toastOptions() {
  return mockToastMessage.mock.calls[0]?.[1] as {
    action: { label: string; onClick: () => void }
    onAutoClose: () => void
    onDismiss: () => void
  }
}

beforeEach(() => {
  mockToastMessage.mockClear()
  mockToastError.mockClear()
  mockToastDismiss.mockClear()
})

describe('useUndoableDelete', () => {
  it('removes the row optimistically and commits on auto-close', async () => {
    const { queryClient, trigger } = setup()
    const p = params()

    act(() => trigger(p))

    expect(queryClient.getQueryData(QUERY_KEY)).toEqual([ROWS[0]])
    expect(p.deleteFn).not.toHaveBeenCalled()
    expect(mockToastMessage).toHaveBeenCalledWith(
      'Deleted',
      expect.objectContaining({ duration: 6000 })
    )

    await act(async () => toastOptions().onAutoClose())

    expect(p.deleteFn).toHaveBeenCalledWith(ROWS[1])
  })

  it('dismiss (swipe) also commits', async () => {
    const { trigger } = setup()
    const p = params()

    act(() => trigger(p))
    await act(async () => toastOptions().onDismiss())

    expect(p.deleteFn).toHaveBeenCalledWith(ROWS[1])
  })

  it('undo restores the snapshot, dismisses the toast, and blocks the commit', async () => {
    const { queryClient, trigger } = setup()
    const p = params()

    act(() => trigger(p))
    await act(async () => toastOptions().action.onClick())

    expect(queryClient.getQueryData(QUERY_KEY)).toEqual(ROWS)
    expect(mockToastDismiss).toHaveBeenCalledWith(42)

    await act(async () => toastOptions().onAutoClose())
    await act(async () => toastOptions().onDismiss())
    expect(p.deleteFn).not.toHaveBeenCalled()
  })

  it('undo after commit does not resurrect the row', async () => {
    const { queryClient, trigger } = setup()
    const p = params()

    act(() => trigger(p))
    await act(async () => toastOptions().onAutoClose())
    await act(async () => toastOptions().action.onClick())

    expect(p.deleteFn).toHaveBeenCalledTimes(1)
    expect(queryClient.getQueryData(QUERY_KEY)).toEqual([ROWS[0]])
  })

  it('restores the snapshot and toasts the error when the commit rejects', async () => {
    const { queryClient, trigger } = setup()
    const p = params({
      deleteFn: vi.fn(async () => {
        throw new Error('boom')
      }),
      errorTitle: (error: unknown) => `failed: ${(error as Error).message}`,
    })

    act(() => trigger(p))
    await act(async () => toastOptions().onAutoClose())
    // flush the rejection handler
    await act(async () => {})

    expect(queryClient.getQueryData(QUERY_KEY)).toEqual(ROWS)
    expect(mockToastError).toHaveBeenCalledWith('failed: boom')
  })

  it('invalidates the primary and extra query keys after commit', async () => {
    const { queryClient, trigger } = setup()
    const spy = vi.spyOn(queryClient, 'invalidateQueries')
    const p = params({ alsoInvalidate: [['other'], ['parent', 'all']] })

    act(() => trigger(p))
    await act(async () => toastOptions().onAutoClose())

    expect(spy).toHaveBeenCalledWith({ queryKey: QUERY_KEY })
    expect(spy).toHaveBeenCalledWith({ queryKey: ['other'] })
    expect(spy).toHaveBeenCalledWith({ queryKey: ['parent', 'all'] })
  })

  it('honours a custom undo window', () => {
    const { trigger } = setup()
    act(() => trigger(params({ windowMs: 50 })))
    expect(mockToastMessage).toHaveBeenCalledWith(
      'Deleted',
      expect.objectContaining({ duration: 50 })
    )
  })
})
