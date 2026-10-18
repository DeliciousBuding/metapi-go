import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import i18n from '@/i18n/config'

import { currentPriceSchema, priceCompareResponseSchema } from '../../types'
import { AccountModelPrice, CurrentPrices } from '../current-prices'

const state = vi.hoisted(() => ({ refresh: vi.fn() }))
vi.mock('@/lib/api', () => ({ api: { refreshAccountPricing: state.refresh } }))
afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

const rates = {
  input: 1.5,
  output: 7.5,
  cacheRead: 0.15,
  cacheWrite: 1.875,
  perRequest: null,
}
const row = currentPriceSchema.parse({
  accountId: 1,
  siteName: 'Synthetic relay',
  username: 'demo',
  model: 'synthetic-model',
  fetchedAt: '2026-10-18T02:00:00Z',
  lastError: '',
  pricing: {
    model: 'synthetic-model',
    quotaType: 0,
    modelRatio: 1.5,
    completionRatio: 5,
    cacheRatio: 0.1,
    cacheWriteRatio: 1.25,
    pricePerRequest: null,
    groups: ['all'],
    billingMode: '',
    billingExpression: '',
  },
  groups: [
    {
      group: 'special',
      description: 'Special group',
      multiplier: 0.5,
      rates,
      relative: {
        input: 0.5,
        output: 0.5,
        cacheRead: 0.5,
        cacheWrite: 0.5,
        perRequest: null,
      },
      sampleCost: 0.009,
    },
  ],
  official: null,
})

describe('published prices', () => {
  it('shows each component and the effective multiplier in Chinese', async () => {
    await i18n.changeLanguage('zhCN')
    render(<AccountModelPrice row={row} />)
    expect(screen.getByText('有效分组倍率')).toBeInTheDocument()
    expect(screen.getByText('$1.5000')).toBeInTheDocument()
    expect(screen.getByText('$7.5000')).toBeInTheDocument()
    expect(screen.getByText('$0.1500')).toBeInTheDocument()
    expect(screen.getByText('$1.8750')).toBeInTheDocument()
    expect(screen.getByText('$0.009000')).toBeInTheDocument()
    await i18n.changeLanguage('en')
  })
  it('keeps free prices visible, unknown ratios blank, and stale snapshots labeled', () => {
    render(
      <AccountModelPrice
        row={{
          ...row,
          lastError: 'upstream_error',
          groups: [
            {
              ...row.groups[0]!,
              multiplier: null,
              rates: { ...rates, input: 0, output: null },
              relative: { ...rates, input: null, output: null },
              sampleCost: null,
            },
          ],
        }}
      />
    )
    expect(screen.getByText('$0.0000')).toBeInTheDocument()
    expect(screen.getByText(/latest refresh failed/)).toBeInTheDocument()
    expect(screen.getAllByText('—').length).toBeGreaterThan(1)
  })
  it('renders conditional billing as text, and keeps per-call units separate', () => {
    const { rerender } = render(
      <AccountModelPrice
        row={{
          ...row,
          pricing: {
            ...row.pricing,
            billingMode: 'tiered_expr',
            billingExpression: 'tokens > 1000 ? 2 : 1',
          },
          groups: [],
        }}
      />
    )
    expect(screen.getByText('tokens > 1000 ? 2 : 1')).toBeInTheDocument()
    rerender(
      <AccountModelPrice
        row={{
          ...row,
          pricing: { ...row.pricing, quotaType: 1, pricePerRequest: 0.05 },
          groups: [
            {
              ...row.groups[0]!,
              rates: {
                input: null,
                output: null,
                cacheRead: null,
                cacheWrite: null,
                perRequest: 0.025,
              },
              sampleCost: 0.025,
            },
          ],
        }}
      />
    )
    expect(screen.getByText(/USD \/ call/)).toHaveTextContent('$0.0250')
    expect(screen.queryByText('vs official')).not.toBeInTheDocument()
  })
  it('refreshes only the explicitly selected account and keeps failure visible', async () => {
    state.refresh.mockRejectedValue(new Error('network failure'))
    const client = new QueryClient({
      defaultOptions: { mutations: { retry: false } },
    })
    const data = priceCompareResponseSchema.parse({
      items: [],
      currentPrices: [row],
      officialPrices: {},
      pricingAccounts: [
        {
          accountId: 1,
          siteName: 'Synthetic relay',
          username: 'demo',
          platform: 'new-api',
          fetchedAt: row.fetchedAt,
          attemptedAt: row.fetchedAt,
          lastError: '',
          supported: true,
        },
      ],
    })
    render(
      <QueryClientProvider client={client}>
        <CurrentPrices data={data} />
      </QueryClientProvider>
    )
    expect(state.refresh).not.toHaveBeenCalled()
    fireEvent.click(
      screen.getByRole('button', { name: 'Pricing sources (1 accounts)' })
    )
    const source = screen.getByRole('listitem')
    fireEvent.click(
      within(source).getByRole('button', {
        name: 'Refresh prices for Synthetic relay',
      })
    )
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Price refresh failed'
    )
    expect(state.refresh).toHaveBeenCalledExactlyOnceWith(1)
    expect(screen.getByText('$1.5000')).toBeInTheDocument()
    client.clear()
  })
})
