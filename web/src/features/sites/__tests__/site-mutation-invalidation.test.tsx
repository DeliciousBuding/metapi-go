import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, renderHook } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, expect, it, vi } from 'vitest'

import { accountQueryKeys } from '@/features/accounts'
import { channelsKeys } from '@/features/channels'
import { routeQueryKeys } from '@/features/token-routes'

import { useBatchUpdateSites, useDeleteSite, useUpdateSite } from '../api'

vi.mock('@/lib/api', () => ({
  api: {
    updateSite: vi.fn().mockResolvedValue({ id: 1, status: 'disabled' }),
    deleteSite: vi.fn().mockResolvedValue({ success: true }),
    batchUpdateSites: vi
      .fn()
      .mockResolvedValue({ successIds: [1], failedItems: [] }),
  },
}))

afterEach(cleanup)

function setup() {
  const client = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  })
  const invalidate = vi.spyOn(client, 'invalidateQueries')
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  )
  return { invalidate, wrapper }
}

it('refreshes inherited availability after a site status change', async () => {
  const { invalidate, wrapper } = setup()
  const { result } = renderHook(() => useUpdateSite(), { wrapper })
  await act(() =>
    result.current.mutateAsync({ id: 1, payload: { status: 'disabled' } })
  )
  for (const key of [
    accountQueryKeys.all,
    channelsKeys.all,
    routeQueryKeys.all,
  ]) {
    expect(invalidate).toHaveBeenCalledWith({ queryKey: key })
  }
})

it('refreshes dependent lists after delete and batch status changes', async () => {
  const { invalidate, wrapper } = setup()
  const { result } = renderHook(
    () => ({ remove: useDeleteSite(), batch: useBatchUpdateSites() }),
    { wrapper }
  )
  await act(() => result.current.remove.mutateAsync(1))
  for (const key of [
    accountQueryKeys.all,
    channelsKeys.all,
    routeQueryKeys.all,
  ]) {
    expect(invalidate).toHaveBeenCalledWith({ queryKey: key })
  }
  invalidate.mockClear()
  await act(() =>
    result.current.batch.mutateAsync({ ids: [1], action: 'disable' })
  )
  for (const key of [
    accountQueryKeys.all,
    channelsKeys.all,
    routeQueryKeys.all,
  ]) {
    expect(invalidate).toHaveBeenCalledWith({ queryKey: key })
  }
})

it('does not refresh routing availability for an unrelated pin update', async () => {
  const { invalidate, wrapper } = setup()
  const { result } = renderHook(() => useUpdateSite(), { wrapper })
  await act(() =>
    result.current.mutateAsync({ id: 1, payload: { isPinned: true } })
  )
  expect(invalidate).not.toHaveBeenCalledWith({ queryKey: channelsKeys.all })
})
