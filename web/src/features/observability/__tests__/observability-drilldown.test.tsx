import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { ObservabilityAutoRefreshToggle } from '../components/observability-auto-refresh-toggle'
import { ObservabilityAutoRefreshProvider } from '../context/auto-refresh-context'
import { HealthSection } from '../sections/health/health-section'
import { observabilityKeys } from '../types'

const mocks = vi.hoisted(() => ({
  useMonitorHealth: vi.fn(),
}))

vi.mock('../api', () => ({
  useMonitorHealth: mocks.useMonitorHealth,
}))

vi.mock('@tanstack/react-router', () => ({
  Link: ({
    to,
    search,
    children,
    ...rest
  }: {
    to: string
    search?: Record<string, string | number | undefined>
    children?: ReactNode
    className?: string
  }) => {
    const query = search
      ? new URLSearchParams(
          Object.entries(search)
            .filter(([, value]) => value !== undefined)
            .map(([key, value]) => [key, String(value)])
        ).toString()
      : ''
    return (
      <a href={query ? `${to}?${query}` : to} {...rest}>
        {children}
      </a>
    )
  },
}))

beforeEach(() => {
  localStorage.removeItem('metapi-go:observability:auto-refresh')
  mocks.useMonitorHealth.mockReset()
})

afterEach(() => {
  vi.useRealTimers()
  cleanup()
})

describe('ObservabilityAutoRefreshToggle quiet refresh', () => {
  it('does not spin continuously when idle and spins only when fetching exceeds 200ms', async () => {
    vi.useFakeTimers()
    const client = new QueryClient()
    render(
      <QueryClientProvider client={client}>
        <ObservabilityAutoRefreshProvider>
          <ObservabilityAutoRefreshToggle />
        </ObservabilityAutoRefreshProvider>
      </QueryClientProvider>
    )

    const refreshBtn = screen.getByRole('button', { name: 'Refresh now' })
    const icon = refreshBtn.querySelector('svg')
    expect(icon).not.toHaveClass('animate-spin')

    let resolveFetch!: (value: unknown) => void
    void client.fetchQuery({
      queryKey: observabilityKeys.monitorHealth(),
      queryFn: () =>
        new Promise((resolve) => {
          resolveFetch = resolve
        }),
    })

    await act(async () => {
      vi.advanceTimersByTime(0)
    })
    await act(async () => {
      vi.advanceTimersByTime(150)
    })
    expect(refreshBtn.querySelector('svg')).not.toHaveClass('animate-spin')

    await act(async () => {
      vi.advanceTimersByTime(100)
    })
    expect(refreshBtn.querySelector('svg')).toHaveClass('animate-spin')

    await act(async () => {
      resolveFetch({ ok: true })
    })
    await act(async () => {
      vi.advanceTimersByTime(0)
    })
    expect(refreshBtn.querySelector('svg')).not.toHaveClass('animate-spin')
    expect(screen.getByTestId('quiet-refresh-status')).toHaveTextContent(
      'Updated just now'
    )
  })
})

describe('HealthSection breakers and cooldown actions', () => {
  it('renders penalty score, live countdown, and diagnostic action links for open breakers', () => {
    const futureMs = Date.now() + 125_000
    const futureIso = new Date(Date.now() + 75_000).toISOString()
    mocks.useMonitorHealth.mockReturnValue({
      isLoading: false,
      isFetching: false,
      isError: false,
      refetch: vi.fn(),
      data: {
        generatedAt: '2026-11-27T00:00:00Z',
        runtimeHealth: {
          sitesTracked: 2,
          sitesBreakerOpen: 1,
          modelsTracked: 3,
          modelsBreakerOpen: 1,
          openBreakers: [
            {
              siteId: 4,
              model: 'gpt-4o',
              breakerLevel: 2,
              breakerUntilMs: futureMs,
              penaltyScore: 18,
            },
          ],
        },
        cooldown: {
          channelsCooling: 1,
          channelsWithFailures: 1,
          channelsRecentlyFailed: 1,
          cooling: [
            {
              channelId: 9,
              accountId: 3,
              siteId: 4,
              siteName: 'Primary',
              failCount: 5,
              cooldownUntil: futureIso,
            },
          ],
        },
        sites: { total: 2, active: 2, disabled: 0, other: 0 },
        accounts: { total: 3, active: 3, disabled: 0, other: 0 },
      },
    })

    render(<HealthSection />)

    expect(screen.getByText('L2')).toBeInTheDocument()
    expect(screen.getByText('18')).toBeInTheDocument()
    const logsLinks = screen.getAllByRole('link', { name: 'Logs' })
    expect(logsLinks[0]).toHaveAttribute(
      'href',
      '/proxy-logs?view=attempts&siteId=4&status=failed&q=gpt-4o'
    )
    expect(logsLinks[1]).toHaveAttribute(
      'href',
      '/proxy-logs?view=attempts&channelId=9&status=failed'
    )
    expect(screen.getByRole('link', { name: 'Routes' })).toHaveAttribute(
      'href',
      '/token-routes?workspaceModel=gpt-4o'
    )
    expect(screen.getAllByText(/\d{2}:\d{2} left/)).toHaveLength(2)
  })
})
