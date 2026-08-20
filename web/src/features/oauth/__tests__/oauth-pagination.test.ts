// Behavior test for the OAuth connections server-side pagination fetcher
// (issue #889 perf section). The list used to request `limit:1000` and
// silently truncate larger fleets; it now fetches one limit/offset page at a
// time and derives the pager total from the backend's real `total`.
//
// `fetchOAuthConnectionsPage` is the shared fetcher consumed by both the
// `useOAuthConnections` hook and the route loader, so the offset math + the
// honest-total degradation are asserted here in isolation (network stubbed).

import { beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import {
  fetchOAuthConnectionsPage,
  oauthConnectionsPageQueryKey,
} from '../api'

vi.mock('@/lib/api', () => ({
  api: {
    getOAuthConnections: vi.fn(),
  },
}))

const mockGetConnections = vi.mocked(api.getOAuthConnections)

beforeEach(() => {
  mockGetConnections.mockReset()
})

describe('fetchOAuthConnectionsPage — limit/offset math', () => {
  it('requests limit=pageSize and offset=page*pageSize', async () => {
    mockGetConnections.mockResolvedValue({
      items: [],
      total: 0,
      limit: 20,
      offset: 40,
    })

    await fetchOAuthConnectionsPage({ page: 2, pageSize: 20 })

    expect(mockGetConnections).toHaveBeenCalledWith({ limit: 20, offset: 40 })
  })

  it('computes offset zero for the first page', async () => {
    mockGetConnections.mockResolvedValue({
      items: [],
      total: 0,
      limit: 50,
      offset: 0,
    })

    await fetchOAuthConnectionsPage({ page: 0, pageSize: 50 })

    expect(mockGetConnections).toHaveBeenCalledWith({ limit: 50, offset: 0 })
  })
})

describe('fetchOAuthConnectionsPage — total handling', () => {
  it('returns the backend total and items so the pager reflects the fleet', async () => {
    mockGetConnections.mockResolvedValue({
      items: [{ accountId: 1 }, { accountId: 2 }] as never,
      total: 137,
      limit: 2,
      offset: 0,
    })

    const page = await fetchOAuthConnectionsPage({ page: 0, pageSize: 2 })

    expect(page.total).toBe(137)
    expect(page.items).toHaveLength(2)
  })

  it('degrades to the page length when total is missing (never invents one)', async () => {
    mockGetConnections.mockResolvedValue({
      items: [{ accountId: 1 }, { accountId: 2 }, { accountId: 3 }] as never,
      total: Number.NaN,
      limit: 3,
      offset: 0,
    })

    const page = await fetchOAuthConnectionsPage({ page: 0, pageSize: 3 })

    expect(page.total).toBe(3)
  })

  it('returns an empty items array when the envelope has none', async () => {
    mockGetConnections.mockResolvedValue({
      items: undefined,
      total: 0,
      limit: 20,
      offset: 0,
    } as never)

    const page = await fetchOAuthConnectionsPage({ page: 0, pageSize: 20 })

    expect(page.items).toEqual([])
    expect(page.total).toBe(0)
  })
})

describe('oauthConnectionsPageQueryKey', () => {
  it('nests the page params under the shared connections prefix', () => {
    expect(
      oauthConnectionsPageQueryKey({ page: 1, pageSize: 20 })
    ).toEqual(['oauth', 'connections', { page: 1, pageSize: 20 }])
  })

  it('stays under the connections prefix so mutations invalidate all pages', () => {
    const key = oauthConnectionsPageQueryKey({ page: 3, pageSize: 50 })
    expect(key.slice(0, 2)).toEqual(['oauth', 'connections'])
  })
})
