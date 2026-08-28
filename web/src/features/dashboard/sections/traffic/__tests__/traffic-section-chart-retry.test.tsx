// Regression test: a failed traffic chart showed only an error message with
// no way to retry. ChartError must offer a Retry button that refetches the
// owning query (audit #1029 batch B).
import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import type { ReactElement, ReactNode } from 'react'
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

vi.mock('../../components/chart-shell', () => ({
  ChartShell: ({ children }: { children?: ReactNode }) => <div>{children}</div>,
}))

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
  mockIncomeOutcome.mockRejectedValue(new Error('income boom'))
  mockSiteTrend.mockRejectedValue(new Error('trend boom'))
  mockSiteDistribution.mockRejectedValue(new Error('distribution boom'))
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

describe('TrafficSection chart error retry', () => {
  it('renders a Retry button per failed chart', async () => {
    renderSection()

    expect(
      await screen.findAllByText('Failed to load income/spend data.')
    ).toHaveLength(3)
    expect(screen.getAllByRole('button', { name: 'Retry' })).toHaveLength(3)
  })

  it('refetches the owning query when Retry is clicked', async () => {
    renderSection()

    const retryButtons = await screen.findAllByRole('button', { name: 'Retry' })
    mockIncomeOutcome.mockResolvedValue({
      days: 30,
      points: [],
      summary: { totalIncome: 0, totalOutcome: 0, net: 0 },
    })
    fireEvent.click(retryButtons[0] as HTMLElement)

    await waitFor(() => {
      expect(mockIncomeOutcome).toHaveBeenCalledTimes(2)
    })
  })
})
