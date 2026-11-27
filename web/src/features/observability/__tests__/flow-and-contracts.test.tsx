import '@testing-library/jest-dom/vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import i18n from '@/i18n/config'
import type { TrafficFlowResponse } from '@/lib/api'

import { FlowSection } from '../sections/flow/flow-section'
import { HealthSection } from '../sections/health/health-section'
import { OverviewSection } from '../sections/overview/overview-section'

const mocks = vi.hoisted(() => ({
  useTrafficFlow: vi.fn(),
  useMonitorHealth: vi.fn(),
  useSlowRequests: vi.fn(),
  useUsageHeatmap: vi.fn(),
}))

vi.mock('../api', () => ({
  useTrafficFlow: mocks.useTrafficFlow,
  useMonitorHealth: mocks.useMonitorHealth,
  useSlowRequests: mocks.useSlowRequests,
  useUsageHeatmap: mocks.useUsageHeatmap,
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
    'aria-label'?: string
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

const SAMPLE_FLOW: TrafficFlowResponse = {
  period: '24h',
  window: {
    from: '2026-11-26T00:00:00Z',
    to: '2026-11-27T00:00:00Z',
  },
  totalRequests: 150,
  totalFailed: 6,
  totalTokens: 98_400,
  totalCost: 2.3456,
  edges: [
    {
      downstreamKeyId: 11,
      downstreamKeyName: 'prod-app',
      model: 'gpt-5.5',
      siteId: 4,
      siteName: 'Primary Relay',
      channelId: 21,
      requests: 100,
      failedCount: 6,
      totalTokens: 60_000,
      totalCost: 1.8,
      averageLatencyMs: 640,
    },
    {
      downstreamKeyId: 12,
      downstreamKeyName: 'staging-bot',
      model: 'claude-sonnet-5-5',
      siteId: 5,
      siteName: 'Secondary Relay',
      channelId: 22,
      requests: 50,
      failedCount: 0,
      totalTokens: 38_400,
      totalCost: 0.5456,
      averageLatencyMs: 410,
    },
  ],
}

beforeEach(async () => {
  await i18n.changeLanguage('en')
  mocks.useTrafficFlow.mockReset()
  mocks.useMonitorHealth.mockReset()
  mocks.useSlowRequests.mockReset()
  mocks.useUsageHeatmap.mockReset()
})

afterEach(() => {
  cleanup()
})

describe('FlowSection', () => {
  it('renders three columns, sr-only table, node selection highlight, and deep links', () => {
    mocks.useTrafficFlow.mockReturnValue({
      data: SAMPLE_FLOW,
      isLoading: false,
      isFetching: false,
      isError: false,
      refetch: vi.fn(),
    })

    render(<FlowSection />)

    // Summary strip values
    expect(screen.getByText('150')).toBeInTheDocument()
    expect(screen.getByText('$2.3456')).toBeInTheDocument()

    // Nodes are native pressable buttons
    const keyButton = screen.getByRole('button', { name: /prod-app/ })
    const otherKeyButton = screen.getByRole('button', { name: /staging-bot/ })
    expect(keyButton).toHaveAttribute('aria-pressed', 'false')

    // Click to select downstream key node
    fireEvent.click(keyButton)
    expect(keyButton).toHaveAttribute('aria-pressed', 'true')
    expect(otherKeyButton).toHaveClass('opacity-35')

    const logsLink = screen.getByRole('link', { name: /Go to logs/ })
    expect(logsLink).toHaveAttribute(
      'href',
      '/proxy-logs?view=attempts&from=2026-11-26T00%3A00%3A00Z&to=2026-11-27T00%3A00%3A00Z&downstreamKeyId=11'
    )

    // Selecting an upstream node exposes both logs and channel deep links
    const upstreamButton = screen.getByRole('button', {
      name: /Primary Relay/,
    })
    fireEvent.click(upstreamButton)
    expect(upstreamButton).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByRole('link', { name: /Go to logs/ })).toHaveAttribute(
      'href',
      '/proxy-logs?view=attempts&from=2026-11-26T00%3A00%3A00Z&to=2026-11-27T00%3A00%3A00Z&siteId=4&channelId=21'
    )
    expect(screen.getByRole('link', { name: /Go to channel/ })).toHaveAttribute(
      'href',
      '/channels?channelId=21'
    )

    // Accessible sr-only table exposes all edge rows
    const table = screen.getByRole('table', { name: 'Traffic flow topology' })
    expect(within(table).getAllByRole('row')).toHaveLength(3)
  })

  it('switches period, metric, and failed-only filter cleanly', () => {
    mocks.useTrafficFlow.mockReturnValue({
      data: SAMPLE_FLOW,
      isLoading: false,
      isFetching: false,
      isError: false,
      refetch: vi.fn(),
    })

    render(<FlowSection />)

    fireEvent.click(screen.getByRole('button', { name: 'Last 7 days' }))
    expect(mocks.useTrafficFlow).toHaveBeenLastCalledWith({
      period: '7d',
      topN: 20,
    })

    fireEvent.click(screen.getByRole('button', { name: 'Estimated cost' }))
    expect(screen.getByRole('button', { name: /prod-app/ })).toHaveTextContent(
      '$1.8000'
    )

    // Toggle failed-only hides clean staging-bot path
    fireEvent.click(screen.getByRole('switch', { name: 'Failed paths only' }))
    expect(
      screen.queryByRole('button', { name: /staging-bot/ })
    ).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: /prod-app/ })).toBeInTheDocument()
  })

  it('distinguishes empty state from error state', () => {
    mocks.useTrafficFlow.mockReturnValue({
      data: {
        ...SAMPLE_FLOW,
        totalRequests: 0,
        totalFailed: 0,
        totalTokens: 0,
        totalCost: 0,
        edges: [],
      },
      isLoading: false,
      isFetching: false,
      isError: false,
      refetch: vi.fn(),
    })

    const { rerender } = render(<FlowSection />)
    expect(
      screen.getByText('No traffic flow edges recorded in this window.')
    ).toBeInTheDocument()

    const refetch = vi.fn()
    mocks.useTrafficFlow.mockReturnValue({
      data: undefined,
      isLoading: false,
      isFetching: false,
      isError: true,
      refetch,
    })

    rerender(<FlowSection />)
    expect(
      screen.getByText('Failed to load traffic flow data.')
    ).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(refetch).toHaveBeenCalledTimes(1)
  })
})

describe('HealthSection new contract fields & legacy fallback', () => {
  it('renders breaker siteName with #siteId fallback and cooldown reason code + expandable summary', () => {
    const futureMs = Date.now() + 90_000
    const futureIso = new Date(Date.now() + 90_000).toISOString()
    const longReason =
      'HTTP 429 Too Many Requests from upstream provider quota guard: rate limit exceeded for organization tier-3 across regional gateway nodes'

    mocks.useMonitorHealth.mockReturnValue({
      isLoading: false,
      isFetching: false,
      isError: false,
      refetch: vi.fn(),
      data: {
        generatedAt: '2026-11-27T00:00:00Z',
        runtimeHealth: {
          sitesTracked: 2,
          sitesBreakerOpen: 2,
          modelsTracked: 2,
          modelsBreakerOpen: 2,
          openBreakers: [
            {
              siteId: 4,
              siteName: 'Primary Relay',
              model: 'gpt-5.5',
              breakerLevel: 2,
              breakerUntilMs: futureMs,
              penaltyScore: 18,
            },
            {
              siteId: 7,
              siteName: '',
              model: 'claude-sonnet-5-5',
              breakerLevel: 1,
              breakerUntilMs: futureMs,
              penaltyScore: 6,
            },
          ],
        },
        cooldown: {
          channelsCooling: 2,
          channelsWithFailures: 2,
          channelsRecentlyFailed: 2,
          cooling: [
            {
              channelId: 9,
              accountId: 3,
              siteId: 4,
              siteName: 'Primary Relay',
              failCount: 5,
              cooldownUntil: futureIso,
              cooldownReasonCode: 'rate_limited',
              cooldownReason: longReason,
            },
            {
              channelId: 10,
              accountId: 4,
              siteId: 7,
              siteName: 'Legacy Site',
              failCount: 2,
              cooldownUntil: futureIso,
            },
          ],
        },
        sites: { total: 2, active: 2, disabled: 0, other: 0 },
        accounts: { total: 2, active: 2, disabled: 0, other: 0 },
      },
    })

    render(<HealthSection />)

    // Named breaker shows siteName + #siteId; empty siteName falls back to #7
    const siteLinkNamed = screen.getByRole('link', {
      name: /Primary Relay.*#4/,
    })
    expect(siteLinkNamed).toHaveAttribute('href', '/sites?edit=4')
    const siteLinkFallback = screen.getByRole('link', { name: '#7' })
    expect(siteLinkFallback).toHaveAttribute('href', '/sites?edit=7')

    // Cooldown reason code + expandable summary
    expect(screen.getByText('Rate limited')).toBeInTheDocument()
    expect(screen.getByText(longReason)).toBeInTheDocument()
    const expandBtn = screen.getByRole('button', { name: 'Show full text' })
    fireEvent.click(expandBtn)
    expect(
      screen.getByRole('button', { name: 'Show less' })
    ).toBeInTheDocument()
  })
})

describe('OverviewSection slow requests gatewayRequestId & nullable TTFB', () => {
  it('links to request view when gatewayRequestId is present, falls back to attempts otherwise, and distinguishes 0 ms vs null TTFB', () => {
    mocks.useSlowRequests.mockReturnValue({
      isLoading: false,
      isFetching: false,
      isError: false,
      refetch: vi.fn(),
      data: {
        items: [
          {
            id: 1,
            model: 'gpt-5.5',
            status: 'success',
            latencyMs: 3200,
            firstByteLatencyMs: 0,
            firstOutputLatencyMs: 850,
            httpStatus: 200,
            requestId: 'req-1',
            gatewayRequestId: 'gw-req-99',
            accountId: 1,
            siteId: 2,
            siteName: 'Alpha',
            createdAt: '2026-11-27T00:00:00Z',
          },
          {
            id: 2,
            model: 'claude-sonnet-5-5',
            status: 'failed',
            latencyMs: 5100,
            firstByteLatencyMs: null,
            firstOutputLatencyMs: null,
            httpStatus: 504,
            requestId: 'req-2',
            accountId: 1,
            siteId: 3,
            siteName: 'Beta',
            createdAt: '2026-11-27T00:00:00Z',
          },
        ],
      },
    })
    mocks.useUsageHeatmap.mockReturnValue({
      isLoading: false,
      isFetching: false,
      isError: false,
      refetch: vi.fn(),
      data: { cells: [] },
    })

    render(<OverviewSection />)

    const links = screen.getAllByRole('link', { name: 'View in logs' })
    expect(links[0]).toHaveAttribute(
      'href',
      '/proxy-logs?view=requests&requestId=gw-req-99'
    )
    expect(links[1]).toHaveAttribute(
      'href',
      '/proxy-logs?view=attempts&q=req-2&siteId=3'
    )

    // 0 ms is measured; null renders em-dash
    expect(screen.getByText('0 ms')).toBeInTheDocument()
    expect(screen.getByText('First out 850 ms')).toBeInTheDocument()
    const rows = screen.getAllByRole('row')
    expect(within(rows[2]).getByText('—')).toBeInTheDocument()
  })
})
