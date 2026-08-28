// Behavior test for useRebuildRoutes feedback truth (#1024): the legacy toast
// consumed phantom `created`/`channelCount` fields the Go backend never
// returned, so every rebuild reported "0 routes / 0 channels added". The hook
// now consumes the truthful stats envelope and branches:
//   routesConsidered === 0            -> warning guidance (no routes exist)
//   inserted === 0 && removed === 0   -> info hint (channels come from models)
//   otherwise                         -> success with real counts

import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { useRebuildRoutes } from '../api'

const { mockRebuildRoutes, mockToastSuccess, mockToastWarning, mockToastInfo } =
  vi.hoisted(() => ({
    mockRebuildRoutes: vi.fn(),
    mockToastSuccess: vi.fn(),
    mockToastWarning: vi.fn(),
    mockToastInfo: vi.fn(),
  }))

vi.mock('@/lib/api', () => ({
  api: {
    rebuildRoutes: mockRebuildRoutes,
  },
}))

vi.mock('@/lib/toast', () => ({
  toast: {
    success: mockToastSuccess,
    warning: mockToastWarning,
    info: mockToastInfo,
    error: vi.fn(),
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
  mockRebuildRoutes.mockReset()
  mockToastSuccess.mockReset()
  mockToastWarning.mockReset()
  mockToastInfo.mockReset()
})

afterEach(() => {
  vi.clearAllMocks()
})

describe('useRebuildRoutes truthful outcome toasts (#1024)', () => {
  it('reports real channel movement with the rebuilt route count', async () => {
    mockRebuildRoutes.mockResolvedValue({
      success: true,
      queued: false,
      status: 'completed',
      routesConsidered: 3,
      channelsInserted: 4,
      channelsRemoved: 1,
      changed: true,
    })
    const { result } = renderHook(() => useRebuildRoutes(), {
      wrapper: createWrapper(),
    })

    await act(() => result.current.mutateAsync({ refreshModels: true }))

    await waitFor(() => {
      expect(mockToastSuccess).toHaveBeenCalledTimes(1)
    })
    const message = mockToastSuccess.mock.calls[0][0] as string
    expect(message).toContain('3')
    expect(message).toContain('4')
    expect(message).toContain('1')
    expect(mockToastWarning).not.toHaveBeenCalled()
    expect(mockToastInfo).not.toHaveBeenCalled()
  })

  it('warns with guidance when no routes exist to rebuild', async () => {
    mockRebuildRoutes.mockResolvedValue({
      success: true,
      queued: false,
      status: 'completed',
      routesConsidered: 0,
      channelsInserted: 0,
      channelsRemoved: 0,
      changed: false,
    })
    const { result } = renderHook(() => useRebuildRoutes(), {
      wrapper: createWrapper(),
    })

    await act(() => result.current.mutateAsync({ refreshModels: true }))

    await waitFor(() => {
      expect(mockToastWarning).toHaveBeenCalledTimes(1)
    })
    expect(mockToastSuccess).not.toHaveBeenCalled()
    expect(mockToastInfo).not.toHaveBeenCalled()
  })

  it('hints that channels come from account models when nothing changed', async () => {
    mockRebuildRoutes.mockResolvedValue({
      success: true,
      queued: false,
      status: 'completed',
      routesConsidered: 2,
      channelsInserted: 0,
      channelsRemoved: 0,
      changed: false,
    })
    const { result } = renderHook(() => useRebuildRoutes(), {
      wrapper: createWrapper(),
    })

    await act(() => result.current.mutateAsync({ refreshModels: true }))

    await waitFor(() => {
      expect(mockToastInfo).toHaveBeenCalledTimes(1)
    })
    expect(mockToastSuccess).not.toHaveBeenCalled()
    expect(mockToastWarning).not.toHaveBeenCalled()
  })

  it('sends refreshModels and wait through to the API call', async () => {
    mockRebuildRoutes.mockResolvedValue({
      success: true,
      queued: false,
      status: 'completed',
      routesConsidered: 1,
      channelsInserted: 1,
      channelsRemoved: 0,
      changed: true,
    })
    const { result } = renderHook(() => useRebuildRoutes(), {
      wrapper: createWrapper(),
    })

    await act(() =>
      result.current.mutateAsync({ refreshModels: false, wait: true })
    )

    expect(mockRebuildRoutes).toHaveBeenCalledWith(false, true)
  })
})
