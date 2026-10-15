import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { useAccountTokens } from '../api'

const getAccountTokens = vi.hoisted(() => vi.fn())
vi.mock('@/lib/api', () => ({ api: { getAccountTokens } }))
afterEach(() => cleanup())

function renderTokens() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return renderHook(() => useAccountTokens(7), {
    wrapper: ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    ),
  })
}

describe('account key inventory response contract', () => {
  it.each([[], { tokens: [] }, { data: { tokens: [] } }])(
    'accepts a legitimate empty inventory (%j)',
    async (response) => {
      getAccountTokens.mockResolvedValue(response)
      const { result } = renderTokens()
      await waitFor(() => expect(result.current.isSuccess).toBe(true))
      expect(result.current.data).toEqual([])
      expect(getAccountTokens).toHaveBeenLastCalledWith(7)
    }
  )

  it.each([null, {}, { tokens: {} }, { error: 'upstream failed' }])(
    'rejects malformed data instead of showing a successful empty inventory (%j)',
    async (response) => {
      getAccountTokens.mockResolvedValue(response)
      const { result } = renderTokens()
      await waitFor(() => expect(result.current.isError).toBe(true))
      expect(result.current.data).toBeUndefined()
    }
  )
})
