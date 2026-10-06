import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen, within } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import i18n from '@/i18n/config'
import { api, type OverviewReport } from '@/lib/api'

import { reportFixture } from '../../__tests__/report-fixture'
import { AttentionPanel } from '../attention-panel'
import { ModelUsagePanel } from '../model-usage-panel'
import { UpstreamHealthPanel } from '../upstream-health-panel'

vi.mock('@/lib/api', () => ({
  api: { getAttention: vi.fn(), getOverviewReport: vi.fn() },
}))
vi.mock('@tanstack/react-router', () => ({
  Link: ({
    to,
    search,
    children,
    ...props
  }: {
    to: string
    search?: Record<string, string>
    children: ReactNode
  }) => (
    <a {...props} href={`${to}?${new URLSearchParams(search)}`}>
      {children}
    </a>
  ),
}))
function mount(element: ReactNode) {
  return render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      {element}
    </QueryClientProvider>
  )
}
beforeEach(async () => {
  vi.resetAllMocks()
  await i18n.changeLanguage('en')
})
afterEach(cleanup)
describe('overview operational insights', () => {
  it('shares one range query across panels and supports the Chinese locale alias', async () => {
    await i18n.changeLanguage('zhCN')
    vi.mocked(api.getOverviewReport).mockResolvedValue(
      reportFixture({
        siteAvailability: [
          {
            siteId: 1,
            siteName: 'Example upstream',
            totalRequests: 1200,
            successCount: 1199,
            failedCount: 1,
            averageLatencyMs: 125,
          },
        ],
        models: [
          { model: 'example-model', cost: 1.25, calls: 1200, tokens: 24000 },
        ],
      })
    )
    mount(
      <>
        <UpstreamHealthPanel />
        <ModelUsagePanel />
      </>
    )
    expect(await screen.findByText('Example upstream')).toBeInTheDocument()
    expect(await screen.findByText('example-model')).toBeInTheDocument()
    expect(screen.getByText('$1.2500')).toBeInTheDocument()
    expect(api.getOverviewReport).toHaveBeenCalledTimes(1)
  })
  it('prioritizes failures and preserves the server window in diagnostic links', async () => {
    vi.mocked(api.getOverviewReport).mockResolvedValue(
      reportFixture({
        siteAvailability: [
          {
            siteId: 1,
            siteName: 'Idle',
            totalRequests: 0,
            successCount: 0,
            failedCount: 0,
            averageLatencyMs: null,
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
    )
    mount(<UpstreamHealthPanel />)
    await screen.findByText('Failing')
    const rows = screen.getAllByRole('row')
    expect(
      rows
        .slice(1)
        .map((row) => within(row).getAllByRole('cell')[0].textContent)
    ).toEqual(['Failing', 'Busy', 'Idle'])
    expect(within(rows[3]).getAllByText('—')).toHaveLength(2)
    const url = new URL(
      screen
        .getByRole('link', { name: 'Failing: 3 failed requests' })
        .getAttribute('href') ?? '',
      'http://localhost'
    )
    expect(Object.fromEntries(url.searchParams)).toEqual({
      siteId: '3',
      status: 'failed',
      ...reportFixture().window,
    })
  })
  it('does not call missing upstream data a healthy empty list', async () => {
    vi.mocked(api.getOverviewReport).mockResolvedValue({
      ...reportFixture(),
      siteAvailability: null,
    } as unknown as OverviewReport)
    mount(<UpstreamHealthPanel />)
    expect(await screen.findByRole('status')).toHaveTextContent(
      'Data unavailable'
    )
    expect(
      screen.queryByText('No enabled upstream sites.')
    ).not.toBeInTheDocument()
  })
  it('reports fetch errors instead of a reassuring empty state', async () => {
    vi.mocked(api.getOverviewReport).mockRejectedValue(
      new Error('report unavailable')
    )
    mount(<UpstreamHealthPanel />)
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'report unavailable'
    )
  })
  it('keeps Other non-clickable and gives real models the same all-time bounds', async () => {
    vi.mocked(api.getOverviewReport).mockResolvedValue(
      reportFixture({
        period: 'all',
        window: { to: '2026-10-06T04:00:00Z' },
        models: [
          { model: 'gpt-test', cost: 2, calls: 40, tokens: 1000 },
          { model: 'other', cost: 1, calls: 2, tokens: 200 },
        ],
      })
    )
    mount(<ModelUsagePanel period='all' />)
    const link = await screen.findByRole('link', { name: 'gpt-test' })
    const url = new URL(link.getAttribute('href') ?? '', 'http://localhost')
    expect(Object.fromEntries(url.searchParams)).toEqual({
      q: 'gpt-test',
      to: '2026-10-06T04:00:00Z',
    })
    expect(
      screen.queryByRole('link', { name: 'Other models' })
    ).not.toBeInTheDocument()
  })
  it('localizes current notices and links to their actual entity', async () => {
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
    expect(await screen.findByRole('link', { name: /Alex/ })).toHaveAttribute(
      'href',
      '/accounts?accountId=42'
    )
    expect(screen.queryByText('untranslated')).not.toBeInTheDocument()
  })
})
