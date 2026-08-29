// metapi-go/features/models — server-pagination fetcher contract tests.
import { afterEach, describe, expect, it, vi } from 'vitest'

const { getModelsMarketplaceMock } = vi.hoisted(() => ({
  getModelsMarketplaceMock: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  api: { getModelsMarketplace: getModelsMarketplaceMock },
}))

import { fetchModelsPage } from '../api'

afterEach(() => {
  getModelsMarketplaceMock.mockReset()
})

describe('fetchModelsPage', () => {
  it('sends 1-based page params and returns items with the true total', async () => {
    getModelsMarketplaceMock.mockResolvedValue({
      items: [{ name: 'gpt-5.5' }],
      total: 240,
      page: 2,
      pageSize: 20,
      meta: {},
    })

    const page = await fetchModelsPage({
      pageIndex: 1,
      pageSize: 20,
      includePricing: true,
    })

    expect(getModelsMarketplaceMock).toHaveBeenCalledWith({
      page: 2,
      pageSize: 20,
      includePricing: true,
    })
    expect(page.items).toEqual([{ name: 'gpt-5.5' }])
    expect(page.total).toBe(240)
  })

  it('degrades a missing total to the returned page length', async () => {
    getModelsMarketplaceMock.mockResolvedValue({ items: [{ name: 'gemini-pro' }] })

    const page = await fetchModelsPage({
      pageIndex: 0,
      pageSize: 20,
      includePricing: false,
    })

    expect(page.total).toBe(1)
  })
})
