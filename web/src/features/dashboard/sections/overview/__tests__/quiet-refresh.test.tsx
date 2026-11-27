import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'
import { api } from '@/lib/api'

import { OverviewSection } from '../overview-section'
import { reportFixture } from './report-fixture'

vi.mock('@/lib/api', () => ({
  api: {
    getDashboardSnapshot: vi.fn(),
    getOverviewReport: vi.fn(),
    getSchedulerStatus: vi.fn(),
    probeModelsNow: vi.fn(),
    getDownstreamApiKeys: vi.fn(),
  },
}))

vi.mock('@/features/token-routes', () => ({
  useRoutes: () => ({ data: [] }),
}))

vi.mock('@tanstack/react-router', () => ({
  Link: ({ to, children }: { to?: string; children: ReactNode }) => (
    <a href={to}>{children}</a>
  ),
}))

vi.mock('@/features/dashboard/components/announcement-banner', () => ({
  AnnouncementBanner: () => null,
}))
vi.mock('../components/request-metrics', () => ({
  RequestMetrics: () => <div data-testid='metrics-dom'>Metrics stable</div>,
}))
vi.mock('../components/request-trend', () => ({ RequestTrend: () => null }))
vi.mock('../components/attention-panel', () => ({ AttentionPanel: () => null }))
vi.mock('../components/upstream-health-panel', () => ({
  UpstreamHealthPanel: () => null,
}))
vi.mock('../components/model-usage-panel', () => ({
  ModelUsagePanel: () => null,
}))

beforeEach(() => {
  vi.mocked(api.getOverviewReport)
    .mockReset()
    .mockResolvedValue(reportFixture())
  vi.mocked(api.getDashboardSnapshot)
    .mockReset()
    .mockResolvedValue({ siteCount: 1, totalAccounts: 1 })
  vi.mocked(api.getSchedulerStatus)
    .mockReset()
    .mockResolvedValue({ items: [], generatedAt: '' })
  vi.mocked(api.getDownstreamApiKeys)
    .mockReset()
    .mockResolvedValue({ items: [] })
})

afterEach(() => {
  vi.useRealTimers()
  cleanup()
})

describe('OverviewSection quiet refresh', () => {
  it('retains existing DOM, delays spin by 200ms, shows freshness text, and hides freshness on error', async () => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={client}>
        <OverviewSection />
      </QueryClientProvider>
    )

    expect(await screen.findByTestId('quiet-refresh-status')).toHaveTextContent(
      'Updated just now'
    )
    const metricsDom = screen.getByTestId('metrics-dom')

    vi.useFakeTimers()
    let resolveNext!: (value: ReturnType<typeof reportFixture>) => void
    vi.mocked(api.getOverviewReport).mockReturnValueOnce(
      new Promise((resolve) => {
        resolveNext = resolve
      })
    )

    const refreshBtn = screen.getByRole('button', { name: 'Refresh report' })
    fireEvent.click(refreshBtn)
    await act(async () => {
      vi.advanceTimersByTime(0)
    })
    expect(screen.getByTestId('metrics-dom')).toBe(metricsDom)
    expect(refreshBtn.querySelector('svg')).not.toHaveClass('animate-spin')

    await act(async () => {
      vi.advanceTimersByTime(250)
    })
    expect(refreshBtn.querySelector('svg')).toHaveClass('animate-spin')

    await act(async () => {
      resolveNext(reportFixture())
    })
    await act(async () => {
      vi.advanceTimersByTime(0)
    })
    expect(refreshBtn.querySelector('svg')).not.toHaveClass('animate-spin')
    expect(screen.getByTestId('metrics-dom')).toBe(metricsDom)

    vi.mocked(api.getOverviewReport).mockRejectedValueOnce(new Error('down'))
    fireEvent.click(refreshBtn)
    await act(async () => {
      vi.advanceTimersByTime(0)
    })
    await act(async () => {
      await Promise.resolve()
      vi.advanceTimersByTime(0)
    })
    expect(screen.queryByTestId('quiet-refresh-status')).not.toBeInTheDocument()
  })
})
