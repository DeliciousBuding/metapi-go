// Behavior test for useBatchUpdateRoutes feedback (#889): batch enable/disable
// previously only invalidated queries — the operator got no outcome signal.
// Now a full update toasts success and a short count toasts a partial warning.

import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { useBatchUpdateRoutes } from '../api'

const { mockBatchUpdateRoutes, mockToastSuccess, mockToastWarning } =
  vi.hoisted(() => ({
    mockBatchUpdateRoutes: vi.fn(),
    mockToastSuccess: vi.fn(),
    mockToastWarning: vi.fn(),
  }))

vi.mock('@/lib/api', () => ({
  api: {
    batchUpdateRoutes: mockBatchUpdateRoutes,
  },
}))

vi.mock('@/lib/toast', () => ({
  toast: {
    success: mockToastSuccess,
    warning: mockToastWarning,
    error: vi.fn(),
    info: vi.fn(),
  },
}))

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )
  }
}

beforeEach(() => {
  mockBatchUpdateRoutes.mockReset()
  mockToastSuccess.mockReset()
  mockToastWarning.mockReset()
})

afterEach(() => {
  vi.clearAllMocks()
})

describe('useBatchUpdateRoutes outcome toasts', () => {
  it('toasts success when every requested route was updated', async () => {
    mockBatchUpdateRoutes.mockResolvedValue({
      success: true,
      updatedCount: 2,
    })
    const { result } = renderHook(() => useBatchUpdateRoutes(), {
      wrapper: createWrapper(),
    })

    await act(() =>
      result.current.mutateAsync({ ids: [1, 2], action: 'enable' })
    )

    await waitFor(() => {
      expect(mockToastSuccess).toHaveBeenCalledWith(
        '2 route(s) updated successfully.'
      )
    })
    expect(mockToastWarning).not.toHaveBeenCalled()
  })

  it('toasts a partial warning when some routes were not updated', async () => {
    mockBatchUpdateRoutes.mockResolvedValue({
      success: true,
      updatedCount: 1,
    })
    const { result } = renderHook(() => useBatchUpdateRoutes(), {
      wrapper: createWrapper(),
    })

    await act(() =>
      result.current.mutateAsync({ ids: [1, 2, 3], action: 'disable' })
    )

    await waitFor(() => {
      expect(mockToastWarning).toHaveBeenCalledWith('1 succeeded, 2 failed.')
    })
    expect(mockToastSuccess).not.toHaveBeenCalled()
  })

  it('toasts nothing on success when the business envelope rejects', async () => {
    mockBatchUpdateRoutes.mockResolvedValue({
      success: false,
      message: 'ids must be non-empty',
    })
    const { result } = renderHook(() => useBatchUpdateRoutes(), {
      wrapper: createWrapper(),
    })

    await act(() =>
      result.current
        .mutateAsync({ ids: [1], action: 'enable' })
        .catch(() => undefined)
    )

    await waitFor(() => {
      expect(result.current.isError).toBe(true)
    })
    expect(mockToastSuccess).not.toHaveBeenCalled()
    expect(mockToastWarning).not.toHaveBeenCalled()
  })
})
