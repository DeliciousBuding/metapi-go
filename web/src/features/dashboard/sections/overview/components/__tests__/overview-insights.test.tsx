import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen, within } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import i18n from '@/i18n/config'
import { api } from '@/lib/api'

import { AttentionPanel } from '../attention-panel'
import { ModelUsagePanel } from '../model-usage-panel'
import { UpstreamHealthPanel } from '../upstream-health-panel'

vi.mock('@/lib/api', () => ({
  api: {
    getAttention: vi.fn(),
    getDashboardSnapshot: vi.fn(),
    getModelCostDistribution: vi.fn(),
  },
}))
vi.mock('@tanstack/react-router', () => ({
  Link: ({
    to,
    search,
    params,
    children,
    ...props
  }: {
    to: string
    search?: Record<string, unknown>
    params?: Record<string, string>
    children: ReactNode
  }) => {
    const path = Object.entries(params ?? {}).reduce(
      (url, [key, value]) => url.replace(`$${key}`, value),
      to
    )
    return (
      <a
        {...props}
        href={
          path +
          (search
            ? '?' +
              new URLSearchParams(
                Object.entries(search).map(([key, value]) => [
                  key,
                  String(value),
                ])
              ).toString()
            : '')
        }
      >
        {children}
      </a>
    )
  },
}))

function mount(element: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>{element}</QueryClientProvider>
  )
}
beforeEach(async () => {
  vi.resetAllMocks()
  await i18n.changeLanguage('en')
})
afterEach(cleanup)

describe('overview operational insights', () => {
  it('prioritizes failing sites, then traffic, and anchors log links to the reported window', async () => {
    vi.mocked(api.getDashboardSnapshot).mockResolvedValue({
      generatedAt: '2026-10-06T04:00:00Z',
      siteAvailability: [
        {
          siteId: 1,
          siteName: 'Idle',
          totalRequests: 0,
          successCount: 0,
          failedCount: 0,
          averageLatencyMs: 0,
        },
        {
          siteId: 2,
          siteName: 'Busy',
          totalRequests: 100,
          successCount: 99,
          failedCount: 1,
          averageLatencyMs: 125,
        },
        {
          siteId: 3,
          siteName: 'Failing',
          totalRequests: 10,
          successCount: 7,
          failedCount: 3,
          averageLatencyMs: 50,
        },
      ],
    })
    mount(<UpstreamHealthPanel />)
    await screen.findByText('Failing')
    const rows = screen.getAllByRole('row')
    expect(
      rows
        .slice(1)
        .map((row) => within(row).getAllByRole('cell')[0].textContent)
    ).toEqual(['Failing', 'Busy', 'Idle'])
    expect(within(rows[3]).getAllByText('—')).toHaveLength(2)
    expect(screen.queryByText('100.0%')).not.toBeInTheDocument()
    const href =
      screen
        .getByRole('link', { name: 'Failing: 3 failed requests' })
        .getAttribute('href') ?? ''
    const url = new URL(href, 'http://localhost')
    expect(Object.fromEntries(url.searchParams)).toEqual({
      siteId: '3',
      status: 'failed',
      from: '2026-10-05T04:00:00.000Z',
      to: '2026-10-06T04:00:00.000Z',
    })
    expect(api.getDashboardSnapshot).toHaveBeenCalledWith({ view: 'insights' })
  })
  it('does not turn missing upstream metrics into an empty healthy list', async () => {
    vi.mocked(api.getDashboardSnapshot).mockResolvedValue({
      siteAvailability: null,
      dashboardStatus: { status: 'partial', failed: ['siteAvailability'] },
    })
    mount(<UpstreamHealthPanel />)
    expect(await screen.findByRole('status')).toHaveTextContent(
      'Data unavailable'
    )
    expect(
      screen.queryByText('No enabled upstream sites.')
    ).not.toBeInTheDocument()
  })
  it('keeps a partial response visible as partial', async () => {
    vi.mocked(api.getDashboardSnapshot).mockResolvedValue({
      siteAvailability: [],
      dashboardStatus: { status: 'partial', failed: ['proxy24h'] },
    })
    mount(<UpstreamHealthPanel />)
    expect(await screen.findByRole('status')).toHaveTextContent(
      'Some dashboard metrics are unavailable'
    )
  })
  it('reports fetch failures instead of a reassuring empty state', async () => {
    vi.mocked(api.getAttention).mockRejectedValue(new Error('offline'))
    mount(<AttentionPanel />)
    expect(await screen.findByRole('alert')).toHaveTextContent('offline')
    expect(
      screen.queryByText('No account or operational notices.')
    ).not.toBeInTheDocument()
  })
  it('localizes attention and resolves an actionable account link', async () => {
    vi.mocked(api.getAttention).mockResolvedValue({
      total: 1,
      items: [
        {
          severity: 'warning',
          category: 'expired_account',
          label: 'untranslated',
          target: '/accounts?accountId=42',
          params: { username: 'Alex' },
        },
      ],
    })
    mount(<AttentionPanel />)
    const link = await screen.findByRole('link', { name: /Alex/ })
    expect(link).toHaveAttribute('href', '/accounts?accountId=42')
    expect(screen.queryByText('untranslated')).not.toBeInTheDocument()
    expect(api.getAttention).toHaveBeenCalledWith(6)
  })
  it('shows model totals and links actual models but not the aggregated other bucket', async () => {
    vi.mocked(api.getModelCostDistribution).mockResolvedValue({
      days: 7,
      topN: 5,
      since: '2026-09-29T00:00:00Z',
      totals: { cost: 3, calls: 42, tokens: 1200 },
      items: [
        {
          model: 'gpt-test',
          label: 'gpt-test',
          calls: 40,
          tokens: 1000,
          cost: 2,
        },
        {
          model: 'other',
          label: 'Other models',
          calls: 2,
          tokens: 200,
          cost: 1,
        },
      ],
    })
    mount(<ModelUsagePanel />)
    const link = await screen.findByRole('link', { name: 'gpt-test' })
    expect(
      new URL(
        link.getAttribute('href') ?? '',
        'http://localhost'
      ).searchParams.get('q')
    ).toBe('gpt-test')
    expect(
      screen.queryByRole('link', { name: 'Other models' })
    ).not.toBeInTheDocument()
    expect(screen.getByText('1,200')).toBeInTheDocument()
    expect(api.getModelCostDistribution).toHaveBeenCalledWith(7, 5)
  })
})
