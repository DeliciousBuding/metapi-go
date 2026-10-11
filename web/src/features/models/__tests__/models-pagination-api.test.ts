import { afterEach, describe, expect, it, vi } from 'vitest'

// metapi-go/features/models — server-driven page fetcher contract tests.
import { fetchModelsPage, type ModelsPageParams } from '../api'

const { getModelsMarketplaceMock } = vi.hoisted(() => ({
  getModelsMarketplaceMock: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  api: { getModelsMarketplace: getModelsMarketplaceMock },
}))

afterEach(() => {
  getModelsMarketplaceMock.mockReset()
})

const baseParams: ModelsPageParams = {
  pageIndex: 1,
  pageSize: 20,
  includePricing: true,
  q: '',
  capabilities: [],
  endpointTypes: [],
  sourceKinds: [],
  sort: '',
}

describe('fetchModelsPage', () => {
  it('sends 1-based page params and returns items, the true total, and facets', async () => {
    const facets = {
      endpointTypes: [{ value: 'openai', count: 200 }],
      capabilities: [],
      sourceKinds: [{ value: 'account', count: 180 }],
    }
    getModelsMarketplaceMock.mockResolvedValue({
      items: [{ name: 'gpt-5.5' }],
      total: 240,
      page: 2,
      pageSize: 20,
      facets,
      facetScope: 'filteredExcludingSelf',
      meta: {},
    })

    const page = await fetchModelsPage(baseParams)

    expect(getModelsMarketplaceMock).toHaveBeenCalledWith({
      page: 2,
      pageSize: 20,
      includePricing: true,
      q: undefined,
      capability: undefined,
      endpointType: undefined,
      sourceKind: undefined,
      sort: undefined,
    })
    expect(page.items).toEqual([{ name: 'gpt-5.5' }])
    expect(page.total).toBe(240)
    expect(page.facets).toEqual(facets)
  })

  it('forwards the URL-owned query contract to the server as CSV params', async () => {
    getModelsMarketplaceMock.mockResolvedValue({ items: [], total: 0 })

    await fetchModelsPage({
      ...baseParams,
      q: 'claude',
      capabilities: ['vision', 'reasoning'],
      endpointTypes: ['anthropic'],
      sourceKinds: ['account', 'oauth'],
      sort: 'supplyCount:desc',
    })

    expect(getModelsMarketplaceMock).toHaveBeenCalledWith({
      page: 2,
      pageSize: 20,
      includePricing: true,
      q: 'claude',
      capability: 'vision,reasoning',
      endpointType: 'anthropic',
      sourceKind: 'account,oauth',
      sort: 'supplyCount:desc',
    })
  })

  it('degrades a missing total to the returned page length and null facets', async () => {
    getModelsMarketplaceMock.mockResolvedValue({
      items: [{ name: 'gemini-pro' }],
    })

    const page = await fetchModelsPage({
      ...baseParams,
      pageIndex: 0,
      includePricing: false,
    })

    expect(page.total).toBe(1)
    expect(page.facets).toBeNull()
  })
})
