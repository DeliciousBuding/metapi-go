// API adapter contract for server-side channels pagination and the
// independent fleet-wide error summary. These tests pin the exact request
// parameters (page is one-based at the transport boundary) and response
// parsing so the page can switch from the legacy whole-list transfer to a
// bounded server page without silently breaking the backend contract.

import { beforeEach, describe, expect, it, vi } from 'vitest'

import { fetchChannelsPage, getChannelsErrorSummary } from '../api'

const apiState = vi.hoisted(() => ({
  getChannels: vi.fn(),
  getChannelsErrorSummary: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  api: apiState,
}))

beforeEach(() => {
  apiState.getChannels.mockReset()
  apiState.getChannelsErrorSummary.mockReset()
})

describe('fetchChannelsPage', () => {
  it('maps a zero-based table page to a one-based transport page', async () => {
    apiState.getChannels.mockResolvedValue({
      items: [{ id: 21, name: 'channel-21' }],
      total: 21,
    })

    const result = await fetchChannelsPage({
      pageIndex: 1,
      pageSize: 50,
      status: 'cooldown',
    })

    expect(apiState.getChannels).toHaveBeenCalledWith({
      page: 2,
      pageSize: 50,
      status: 'cooldown',
    })
    expect(result).toEqual({
      items: [{ id: 21, name: 'channel-21' }],
      total: 21,
    })
  })

  it('omits the status parameter when no status is scoped', async () => {
    apiState.getChannels.mockResolvedValue({ items: [], total: 0 })

    await fetchChannelsPage({ pageIndex: 0, pageSize: 20 })

    expect(apiState.getChannels).toHaveBeenCalledWith({
      page: 1,
      pageSize: 20,
    })
  })

  it('fails explicitly on a malformed page envelope', async () => {
    apiState.getChannels.mockResolvedValue({ items: 'not-an-array' })

    await expect(
      fetchChannelsPage({ pageIndex: 0, pageSize: 20 })
    ).rejects.toThrow('Invalid channels page response')
  })
})

describe('getChannelsErrorSummary', () => {
  it('passes through the backend count and status breakdown', async () => {
    const payload = {
      total: 12,
      errorCount: 4,
      byStatus: {
        enabled: 8,
        cooldown: 3,
        breaker_open: 1,
        manually_disabled: 0,
      },
    }
    apiState.getChannelsErrorSummary.mockResolvedValue(payload)

    await expect(getChannelsErrorSummary()).resolves.toEqual(payload)
  })

  it('fails explicitly on a malformed summary', async () => {
    apiState.getChannelsErrorSummary.mockResolvedValue({ total: 12 })

    await expect(getChannelsErrorSummary()).rejects.toThrow(
      'Invalid channels error summary response'
    )
  })
})
