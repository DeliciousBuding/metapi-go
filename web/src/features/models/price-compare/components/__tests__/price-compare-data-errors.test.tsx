import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import i18n from '@/i18n/config'

import { priceCompareQueryOptions } from '../../api'
import { PriceComparePage } from '../price-compare-page'

const state = vi.hoisted(() => ({ get: vi.fn(), navigate: vi.fn() }))

vi.mock('@/lib/api', () => ({
  api: { getModelPriceCompare: state.get },
}))

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => state.navigate,
  useSearch: () => ({}),
  Link: ({ children }: { children?: ReactNode }) => <a>{children}</a>,
}))

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

const item = {
  siteId: 1,
  siteName: 'Example',
  platform: 'openai',
  model: 'example-model',
  accountId: 2,
  inputPerMillion: 1,
  outputPerMillion: 2,
}

function client() {
  return new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })
}

describe('price comparison API data errors', () => {
  it.each([
    ['missing items', {}],
    ['null items', { items: null }],
    ['non-array items', { items: {} }],
    [
      'one invalid candidate among valid rows',
      { items: [item, { ...item, siteId: 'invalid' }] },
    ],
  ])(
    'rejects %s instead of returning a successful empty list',
    async (_, response) => {
      state.get.mockResolvedValue(response)
      const queryClient = client()
      try {
        await expect(
          queryClient.fetchQuery(priceCompareQueryOptions())
        ).rejects.toThrow()
      } finally {
        queryClient.clear()
      }
    }
  )

  it('keeps valid candidates and a genuine empty result', async () => {
    state.get
      .mockResolvedValueOnce({ items: [item] })
      .mockResolvedValueOnce({ items: [] })
    const queryClient = client()
    try {
      const rows = await queryClient.fetchQuery(
        priceCompareQueryOptions({ model: item.model })
      )
      expect(rows).toEqual([expect.objectContaining(item)])
      expect(
        await queryClient.fetchQuery(
          priceCompareQueryOptions({ model: 'missing' })
        )
      ).toEqual([])
    } finally {
      queryClient.clear()
    }
  })

  it.each(['en', 'zhCN'])(
    'shows an error, not empty state, and can retry in %s',
    async (language) => {
      await i18n.changeLanguage(language)
      state.get
        .mockResolvedValueOnce({
          items: [item, { ...item, siteId: 'invalid' }],
        })
        .mockResolvedValueOnce({ items: [] })
      const queryClient = client()
      try {
        render(
          <QueryClientProvider client={queryClient}>
            <PriceComparePage />
          </QueryClientProvider>
        )
        const alert = await screen.findByRole('alert')
        expect(alert).toHaveTextContent(
          i18n.t('priceCompare.page.invalidResponse')
        )
        expect(alert).not.toHaveTextContent(
          /invalid_type|expected|"path"|"items"/
        )
        expect(
          screen.queryByText(i18n.t('priceCompare.page.emptyDescription'))
        ).not.toBeInTheDocument()
        fireEvent.click(
          screen.getByRole('button', { name: i18n.t('common.retry') })
        )
        expect(
          await screen.findByText(i18n.t('priceCompare.page.emptyDescription'))
        ).toBeInTheDocument()
        expect(screen.queryByRole('alert')).not.toBeInTheDocument()
        expect(state.get).toHaveBeenCalledTimes(2)
      } finally {
        queryClient.clear()
        await i18n.changeLanguage('en')
      }
    }
  )

  it.each(['en', 'zhCN'])(
    'keeps network errors distinct from invalid data in %s',
    async (language) => {
      await i18n.changeLanguage(language)
      state.get.mockRejectedValueOnce(new Error('HTTP 503'))
      const queryClient = client()
      try {
        render(
          <QueryClientProvider client={queryClient}>
            <PriceComparePage />
          </QueryClientProvider>
        )
        const alert = await screen.findByRole('alert')
        expect(alert).toHaveTextContent(
          i18n.t('priceCompare.page.loadError', { message: 'HTTP 503' })
        )
        expect(alert).not.toHaveTextContent(
          i18n.t('priceCompare.page.invalidResponse')
        )
        expect(
          screen.queryByText(i18n.t('priceCompare.page.emptyDescription'))
        ).not.toBeInTheDocument()
      } finally {
        queryClient.clear()
        await i18n.changeLanguage('en')
      }
    }
  )
})
