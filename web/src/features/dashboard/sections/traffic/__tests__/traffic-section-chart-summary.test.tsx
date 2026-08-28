// S10 (#1035): traffic charts expose sr-only data summary tables built from
// the already-loaded query data, so screen-reader users get the key series
// values instead of opaque recharts SVG. Uses the real ChartShell so the
// summary slot is exercised end-to-end (charts themselves stay stubbed).
import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen } from '@testing-library/react'
import type { ReactElement } from 'react'
import {
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from 'vitest'

import '@/i18n/config'

import { TrafficSection } from '../traffic-section'

const { mockIncomeOutcome, mockSiteTrend, mockSiteDistribution } = vi.hoisted(
  () => ({
    mockIncomeOutcome: vi.fn(),
    mockSiteTrend: vi.fn(),
    mockSiteDistribution: vi.fn(),
  })
)

vi.mock('@/lib/api', () => ({
  api: {
    getBalanceIncomeOutcome: mockIncomeOutcome,
    getSiteTrend: mockSiteTrend,
    getSiteDistribution: mockSiteDistribution,
  },
}))

// Charts render recharts SVG; summaries are plain DOM tables. Stub the
// charts so the assertions target the summary layer only.
vi.mock('../../components/charts', () => ({
  IncomeOutcomeChart: () => null,
  SiteDistributionChart: () => null,
  SiteTrendChart: () => null,
}))

beforeAll(() => {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: vi.fn().mockImplementation((query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  })
})

beforeEach(() => {
  mockIncomeOutcome.mockReset()
  mockSiteTrend.mockReset()
  mockSiteDistribution.mockReset()
  mockIncomeOutcome.mockResolvedValue({
    days: 2,
    points: [
      { day: '2026-08-27', income: 12, outcome: 8, net: 4 },
      { day: '2026-08-28', income: 6, outcome: 2, net: 4 },
    ],
    summary: { totalIncome: 18, totalOutcome: 10, net: 8 },
  })
  mockSiteTrend.mockResolvedValue({
    trend: [
      { date: '2026-08-27', sites: { 'site-a': { spend: 3, calls: 100 } } },
      { date: '2026-08-28', sites: { 'site-a': { spend: 2, calls: 50 } } },
    ],
  })
  mockSiteDistribution.mockResolvedValue({
    distribution: [
      {
        siteId: 1,
        siteName: 'site-a',
        platform: 'openai',
        totalBalance: 40,
        totalSpend: 10,
        accountCount: 2,
      },
    ],
  })
})

afterEach(() => cleanup())

function renderSection() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })
  return render(
    (
      <QueryClientProvider client={queryClient}>
        <TrafficSection />
      </QueryClientProvider>
    ) as ReactElement
  )
}

describe('TrafficSection chart data summaries (S10)', () => {
  it('exposes one sr-only summary table per loaded chart', async () => {
    renderSection()

    expect(
      await screen.findByRole('table', { name: 'Income vs spend' })
    ).toHaveClass('sr-only')
    expect(
      await screen.findByRole('table', { name: 'Site trend' })
    ).toHaveClass('sr-only')
    expect(
      await screen.findByRole('table', { name: 'Site distribution' })
    ).toHaveClass('sr-only')
  })

  it('summarizes income/outcome totals plus the latest day', async () => {
    renderSection()

    const table = await screen.findByRole('table', { name: 'Income vs spend' })
    expect(table).toHaveTextContent('Income')
    expect(table).toHaveTextContent('Outcome')
    expect(table).toHaveTextContent('Net')
    expect(table).toHaveTextContent('$18.000') // 30d total income
    expect(table).toHaveTextContent('$6.000') // latest-day income
    expect(table).toHaveTextContent('$8.000') // 30d net
  })

  it('aggregates per-site spend and calls for the trend summary', async () => {
    renderSection()

    const table = await screen.findByRole('table', { name: 'Site trend' })
    expect(table).toHaveTextContent('site-a')
    expect(table).toHaveTextContent('$5.000') // 3 + 2 spend
    expect(table).toHaveTextContent('150') // 100 + 50 calls
  })

  it('lists balance share per site in the distribution summary', async () => {
    renderSection()

    const table = await screen.findByRole('table', {
      name: 'Site distribution',
    })
    expect(table).toHaveTextContent('site-a')
    expect(table).toHaveTextContent('$40.000') // balance
    expect(table).toHaveTextContent('$10.000') // spend
    expect(table).toHaveTextContent('100.0%') // sole-site share
  })

  it('renders no summary tables while the charts are failing', async () => {
    mockIncomeOutcome.mockRejectedValue(new Error('income boom'))
    mockSiteTrend.mockRejectedValue(new Error('trend boom'))
    mockSiteDistribution.mockRejectedValue(new Error('distribution boom'))
    renderSection()

    await screen.findAllByText('Failed to load income/spend data.')
    expect(screen.queryByRole('table')).toBeNull()
  })
})
