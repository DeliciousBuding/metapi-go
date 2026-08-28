// S10 (#1035): the model-cost donut exposes an sr-only data summary table
// built from the already-loaded cost rows (series x key points: cost,
// requests, tokens, share). Latency charts keep text-only empty states.
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

import { ModelsSection } from '../models-section'

const { mockModelCost, mockHistogram, mockTrend } = vi.hoisted(() => ({
  mockModelCost: vi.fn(),
  mockHistogram: vi.fn(),
  mockTrend: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  api: {
    getModelCostDistribution: mockModelCost,
    getLatencyHistogram: mockHistogram,
    getLatencyTrend: mockTrend,
  },
}))

// Charts render recharts SVG; the summary is a plain DOM table. Stub the
// charts so the assertions target the summary layer only.
vi.mock('../../components/charts', () => ({
  LatencyHistogramChart: () => null,
  LatencyTrendChart: () => null,
  ModelCostChart: () => null,
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
  mockModelCost.mockReset()
  mockHistogram.mockReset()
  mockTrend.mockReset()
  mockModelCost.mockResolvedValue({
    items: [
      { model: 'gpt-x', label: 'GPT-X', cost: 12, calls: 300, tokens: 45000 },
      { model: 'claude-y', label: '', cost: 8, calls: 100, tokens: 15000 },
    ],
  })
  mockHistogram.mockResolvedValue({ buckets: [] })
  mockTrend.mockResolvedValue({ points: [] })
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
        <ModelsSection />
      </QueryClientProvider>
    ) as ReactElement
  )
}

describe('ModelsSection cost data summary (S10)', () => {
  it('exposes an sr-only summary table for the cost donut', async () => {
    renderSection()

    const table = await screen.findByRole('table', {
      name: 'Cost distribution',
    })
    expect(table).toHaveClass('sr-only')
  })

  it('lists per-model cost, requests, tokens and share', async () => {
    renderSection()

    const table = await screen.findByRole('table', {
      name: 'Cost distribution',
    })
    expect(table).toHaveTextContent('GPT-X')
    // Empty label falls back to the raw model id.
    expect(table).toHaveTextContent('claude-y')
    expect(table).toHaveTextContent('$12.000')
    expect(table).toHaveTextContent('300')
    expect(table).toHaveTextContent('45,000')
    expect(table).toHaveTextContent('60.0%')
    expect(table).toHaveTextContent('40.0%')
  })

  it('renders no summary table while the cost query is failing', async () => {
    mockModelCost.mockRejectedValue(new Error('cost boom'))
    renderSection()

    await screen.findAllByText('Failed to load model / latency data.')
    expect(screen.queryByRole('table')).toBeNull()
  })
})
