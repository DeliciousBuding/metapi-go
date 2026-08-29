import { afterEach, describe, expect, it, vi } from 'vitest'

// metapi-go/features/accounts — server-pagination fetcher contract tests.
import { fetchAccountsPage } from '../api'

const { getAccountsPageMock } = vi.hoisted(() => ({
  getAccountsPageMock: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  api: { getAccountsPage: getAccountsPageMock },
}))

afterEach(() => {
  getAccountsPageMock.mockReset()
})

describe('fetchAccountsPage', () => {
  it('sends 1-based page params and returns the items/total/sites envelope', async () => {
    getAccountsPageMock.mockResolvedValue({
      items: [{ id: 3, username: 'alice' }],
      total: 27,
      sites: [{ id: 1, name: 'Site' }],
      generatedAt: '2026-08-29T00:00:00Z',
    })

    const page = await fetchAccountsPage({ pageIndex: 1, pageSize: 10 })

    expect(getAccountsPageMock).toHaveBeenCalledWith({ page: 2, pageSize: 10 })
    expect(page.items).toEqual([{ id: 3, username: 'alice' }])
    expect(page.total).toBe(27)
    expect(page.sites).toEqual([{ id: 1, name: 'Site' }])
  })

  it('degrades a missing total to the returned page length', async () => {
    getAccountsPageMock.mockResolvedValue({ items: [{ id: 1 }] })

    const page = await fetchAccountsPage({ pageIndex: 0, pageSize: 10 })

    expect(page.total).toBe(1)
  })
})
