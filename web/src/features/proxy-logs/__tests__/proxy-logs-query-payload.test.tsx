// Behavior test for the useProxyLogs query payload. Mocks the shared
// axios instance (http-client) so the real `request()` transport runs end
// to end and the request URL can be inspected — asserting the latency
// bounds added to ProxyLogsQuery are serialized server-side.
//
// The original bug kept `latencyMin`/`latencyMax` OUT of the page's query
// payload (filtering client-side), so `total` (server, unfiltered) and
// `items` (client, filtered) disagreed. This test guards the type +
// transport contract: once latency is on ProxyLogsQuery, useProxyLogs
// forwards it to the request URL.

import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { ProxyLogsQuery } from '@/lib/api'

import { useProxyLogs } from '../api'

const { mockApiClientGet } = vi.hoisted(() => ({
  // capture stub for `apiClient.get(url, config)` — the JSON GET path used
  // by `request()` in lib/api/transport.ts. Resolves to an empty-but-valid
  // ProxyLogsResponse so the query succeeds and the URL is inspectable.
  mockApiClientGet: vi.fn(),
}))

vi.mock('@/lib/http-client', () => ({
  apiClient: {
    get: mockApiClientGet,
    request: vi.fn(),
    post: vi.fn(),
    put: vi.fn(),
    delete: vi.fn(),
    patch: vi.fn(),
  },
  fetchAuthenticatedResponse: vi.fn(),
  extractResponseErrorMessage: vi.fn(),
}))

const EMPTY_PROXY_LOGS_RESPONSE = {
  items: [],
  total: 0,
  page: 1,
  pageSize: 20,
  clientOptions: [],
  summary: {
    totalCount: 0,
    successCount: 0,
    failedCount: 0,
    totalCost: 0,
    totalTokensAll: 0,
  },
  sites: [],
}

function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: 0, refetchOnWindowFocus: false },
    },
  })
}

function createWrapper(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )
  }
}

beforeEach(() => {
  mockApiClientGet.mockReset()
  mockApiClientGet.mockResolvedValue({ data: EMPTY_PROXY_LOGS_RESPONSE })
})

afterEach(() => {
  vi.clearAllMocks()
})

// firstRequestedUrl returns the URL of the first apiClient.get call,
// throwing if no call was made. Avoids non-null assertions while keeping
// the assertions readable; the caller has already waitFor'd the call.
function firstRequestedUrl(): string {
  const url = mockApiClientGet.mock.calls[0]?.[0]
  if (typeof url !== 'string') {
    throw new Error('expected apiClient.get to have been called once')
  }
  return url
}

describe('useProxyLogs — latency filter payload', () => {
  it('serializes latencyMin and latencyMax into the request URL when set', async () => {
    const params: ProxyLogsQuery = {
      limit: 20,
      offset: 0,
      latencyMin: 2000,
      latencyMax: 4000,
    }
    const queryClient = createQueryClient()
    renderHook(() => useProxyLogs(params), {
      wrapper: createWrapper(queryClient),
    })

    await waitFor(() => {
      expect(mockApiClientGet).toHaveBeenCalledTimes(1)
    })

    const requestedUrl = firstRequestedUrl()
    expect(requestedUrl).toContain('/api/stats/proxy-logs')
    expect(requestedUrl).toContain('latencyMin=2000')
    expect(requestedUrl).toContain('latencyMax=4000')
  })

  it('omits latency bounds from the URL when they are null/undefined', async () => {
    const params: ProxyLogsQuery = { limit: 20, offset: 0 }
    const queryClient = createQueryClient()
    renderHook(() => useProxyLogs(params), {
      wrapper: createWrapper(queryClient),
    })

    await waitFor(() => {
      expect(mockApiClientGet).toHaveBeenCalledTimes(1)
    })

    const requestedUrl = firstRequestedUrl()
    expect(requestedUrl).not.toContain('latencyMin')
    expect(requestedUrl).not.toContain('latencyMax')
  })

  it('serializes latencyMin alone (the "Slow only" preset, latencyMax unset)', async () => {
    const params: ProxyLogsQuery = { limit: 20, offset: 0, latencyMin: 2000 }
    const queryClient = createQueryClient()
    renderHook(() => useProxyLogs(params), {
      wrapper: createWrapper(queryClient),
    })

    await waitFor(() => {
      expect(mockApiClientGet).toHaveBeenCalledTimes(1)
    })

    const requestedUrl = firstRequestedUrl()
    expect(requestedUrl).toContain('latencyMin=2000')
    // latencyMax must NOT be emitted when unset — buildQueryString drops
    // undefined/null/empty so the URL stays minimal.
    expect(requestedUrl).not.toContain('latencyMax')
  })
})

describe('useProxyLogs — channel filter payload', () => {
  it('serializes channelId into the request URL when set', async () => {
    const params: ProxyLogsQuery = { limit: 20, offset: 0, channelId: 42 }
    const queryClient = createQueryClient()
    renderHook(() => useProxyLogs(params), {
      wrapper: createWrapper(queryClient),
    })

    await waitFor(() => {
      expect(mockApiClientGet).toHaveBeenCalledTimes(1)
    })

    const requestedUrl = firstRequestedUrl()
    expect(requestedUrl).toContain('/api/stats/proxy-logs')
    expect(requestedUrl).toContain('channelId=42')
  })

  it('omits channelId from the URL when it is undefined', async () => {
    const params: ProxyLogsQuery = { limit: 20, offset: 0 }
    const queryClient = createQueryClient()
    renderHook(() => useProxyLogs(params), {
      wrapper: createWrapper(queryClient),
    })

    await waitFor(() => {
      expect(mockApiClientGet).toHaveBeenCalledTimes(1)
    })

    expect(firstRequestedUrl()).not.toContain('channelId')
  })

  it('composes channelId with the other server-side filters', async () => {
    const params: ProxyLogsQuery = {
      limit: 20,
      offset: 0,
      channelId: 7,
      siteId: 3,
      status: 'failed',
      latencyMin: 2000,
    }
    const queryClient = createQueryClient()
    renderHook(() => useProxyLogs(params), {
      wrapper: createWrapper(queryClient),
    })

    await waitFor(() => {
      expect(mockApiClientGet).toHaveBeenCalledTimes(1)
    })

    const requestedUrl = firstRequestedUrl()
    expect(requestedUrl).toContain('channelId=7')
    expect(requestedUrl).toContain('siteId=3')
    expect(requestedUrl).toContain('status=failed')
    expect(requestedUrl).toContain('latencyMin=2000')
  })
})
