import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'
import { api } from '@/lib/api'

import { RequestMetrics } from '../request-metrics'

vi.mock('@/lib/api', () => ({ api: { getProxyLogsMeta: vi.fn() } }))
vi.mock('@tanstack/react-router', () => ({
  Link: (props: {
    to: string
    search?: Record<string, string>
    children: ReactNode
  }) => (
    <a href={`${props.to}?${new URLSearchParams(props.search)}`}>
      {props.children}
    </a>
  ),
}))

beforeEach(() => {
  vi.mocked(api.getProxyLogsMeta).mockReset()
})
afterEach(cleanup)

function renderMetrics() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <RequestMetrics />
    </QueryClientProvider>
  )
}

describe('operator request metrics', () => {
  it('uses unfiltered aggregate counts and links failures to the exact fetched window', async () => {
    vi.mocked(api.getProxyLogsMeta).mockResolvedValue({
      clientOptions: [],
      sites: [],
      summary: {
        totalCount: 100,
        successCount: 97,
        failedCount: 3,
        totalCost: 1.25,
        totalTokensAll: 12000,
      },
    })
    renderMetrics()
    expect(await screen.findByText('97.0%')).toBeInTheDocument()
    const params = vi.mocked(api.getProxyLogsMeta).mock.calls[0]?.[0]
    if (!params?.from || !params.to) throw new Error('Missing request window')
    expect(params).not.toHaveProperty('siteId')
    expect(params).not.toHaveProperty('status')
    expect(Date.parse(params.to) - Date.parse(params.from)).toBe(86_400_000)
    const link = screen.getByRole('link', { name: /Success rate/ })
    const href = link.getAttribute('href')
    if (!href) throw new Error('Missing drill-down link')
    const target = new URL(href, 'http://localhost')
    expect(target.searchParams.get('status')).toBe('failed')
    expect(target.searchParams.get('from')).toBe(params.from)
    expect(target.searchParams.get('to')).toBe(params.to)
    expect(screen.getByText('3 failed · open failure logs')).toBeInTheDocument()
  })

  it('does not describe a no-traffic period as 100 percent success', async () => {
    vi.mocked(api.getProxyLogsMeta).mockResolvedValue({
      clientOptions: [],
      sites: [],
      summary: {
        totalCount: 0,
        successCount: 0,
        failedCount: 0,
        totalCost: 0,
        totalTokensAll: 0,
      },
    })
    renderMetrics()
    expect(
      await screen.findByText('No recorded requests in this period')
    ).toBeInTheDocument()
    expect(screen.queryByText('100.0%')).not.toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: /Success rate/ })
    ).toHaveTextContent('—')
  })

  it('reports query failure without inventing a zero-cost healthy result', async () => {
    vi.mocked(api.getProxyLogsMeta).mockRejectedValue(
      new Error('aggregate unavailable')
    )
    renderMetrics()
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'aggregate unavailable'
    )
    expect(
      screen.getByRole('link', { name: /Estimated call cost/ })
    ).toHaveTextContent('—')
    expect(screen.queryByText('$0.00')).not.toBeInTheDocument()
  })
})
