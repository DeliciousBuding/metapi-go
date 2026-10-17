import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'
import { api } from '@/lib/api'
import { formatDateTime } from '@/lib/format'

import { reportFixture } from '../../__tests__/report-fixture'
import { RequestMetrics } from '../request-metrics'
import { RequestTrend } from '../request-trend'

vi.mock('@/lib/api', () => ({ api: { getOverviewReport: vi.fn() } }))
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
  vi.mocked(api.getOverviewReport).mockReset()
})
afterEach(cleanup)

function renderMetrics(withTrend = false) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <RequestMetrics />
      {withTrend && <RequestTrend />}
    </QueryClientProvider>
  )
}

describe('operator request metrics', () => {
  it('uses persisted gateway outcomes rather than the successful last attempt and preserves the coverage boundary', async () => {
    vi.mocked(api.getOverviewReport).mockResolvedValue(
      reportFixture({
        requestSummary: {
          totalCount: 10,
          successCount: 10,
          failedCount: 0,
          legacyAttemptCount: 0,
          partial: false,
          partialReasons: [],
        },
        gatewayRequestSummary: {
          totalCount: 5,
          completedCount: 2,
          failedCount: 1,
          cancelledCount: 1,
          rejectedCount: 1,
          coverageStartedAt: '2026-10-16T00:00:00Z',
        },
        gatewayRequestPoints: [],
      })
    )
    renderMetrics(true)
    expect(await screen.findByText('40.0%')).toBeInTheDocument()
    expect(screen.queryByText('100.0%')).not.toBeInTheDocument()
    expect(
      screen.getByText(/1 failed · 1 cancelled · 1 rejected/)
    ).toBeInTheDocument()
    expect(
      screen.getByText(/Request records available since/)
    ).toHaveTextContent(formatDateTime('2026-10-16T00:00:00Z', 'en'))
    expect(screen.queryByText(/2026-10-16T00:00:00Z/)).not.toBeInTheDocument()
    expect(
      screen.getAllByText(/Completion is based on protocol outcomes/)
    ).toHaveLength(1)
    const link = screen.getByRole('link', { name: /Request completion rate/ })
    expect(link.getAttribute('href')).toContain('view=requests')
    expect(link.getAttribute('href')).not.toContain('status=failed')
  })
  it('uses unfiltered aggregate counts and links failures to the exact fetched window', async () => {
    vi.mocked(api.getOverviewReport).mockResolvedValue(
      reportFixture({
        summary: {
          totalCount: 15,
          successCount: 12,
          failedCount: 3,
          totalCost: 1.25,
          totalTokensAll: 12000,
          averageLatencyMs: null,
        },
        requestSummary: {
          totalCount: 10,
          successCount: 9,
          failedCount: 1,
          legacyAttemptCount: 2,
          partial: true,
          partialReasons: ['requestIdMissing'],
        },
      })
    )
    renderMetrics()
    expect(await screen.findByText('90.0%')).toBeInTheDocument()
    expect(api.getOverviewReport).toHaveBeenCalledWith('7d')
    const params = reportFixture().window
    const link = screen.getByRole('link', { name: /Last-attempt success rate/ })
    const href = link.getAttribute('href')
    if (!href) throw new Error('Missing drill-down link')
    const target = new URL(href, 'http://localhost')
    expect(target.searchParams.get('status')).toBe('failed')
    expect(target.searchParams.get('from')).toBe(params.from)
    expect(target.searchParams.get('to')).toBe(params.to)
    expect(
      screen.getByText('1 failed last attempts · inspect attempt logs')
    ).toBeInTheDocument()
    expect(screen.getByText('15')).toBeInTheDocument()
  })

  it('does not describe a no-traffic period as 100 percent success', async () => {
    vi.mocked(api.getOverviewReport).mockResolvedValue(
      reportFixture({
        summary: {
          totalCount: 0,
          successCount: 0,
          failedCount: 0,
          totalCost: 0,
          totalTokensAll: 0,
          averageLatencyMs: null,
        },
        requestSummary: {
          totalCount: 0,
          successCount: 0,
          failedCount: 0,
          legacyAttemptCount: 0,
          partial: false,
          partialReasons: [],
        },
      })
    )
    renderMetrics()
    expect(
      await screen.findByText(
        'No request-level records with requestId in this period'
      )
    ).toBeInTheDocument()
    expect(screen.queryByText('100.0%')).not.toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: /Last-attempt success rate/ })
    ).toHaveTextContent('—')
  })

  it('reports query failure without inventing a zero-cost healthy result', async () => {
    vi.mocked(api.getOverviewReport).mockRejectedValue(
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
