// Dashboard charts must carry an accessible name (W19-T2 P2-p). The
// ChartContainer figure semantics are pinned in ui/__tests__/chart-a11y; this
// locks the other half of the contract — that every real chart surface passes
// an explicit aria-label, because five of the six are multi-series or
// label-less configs that the config-label fallback cannot name.
import '@testing-library/jest-dom/vitest'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'

import '@/i18n/config'

import {
  IncomeOutcomeChart,
  LatencyHistogramChart,
  LatencyTrendChart,
  ModelCostChart,
  SiteDistributionChart,
  SiteTrendChart,
} from '../charts'

// recharts' ResponsiveObserver instantiates a ResizeObserver to measure the
// wrapper; jsdom ships none, so stub the constructor.
class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
globalThis.ResizeObserver =
  ResizeObserverStub as unknown as typeof ResizeObserver

afterEach(() => cleanup())

describe('dashboard charts carry an explicit accessible name', () => {
  it('names the income vs spend chart', () => {
    render(
      <IncomeOutcomeChart
        data={[
          { day: '2026-08-01', type: 'income', value: 10 },
          { day: '2026-08-01', type: 'outcome', value: 4 },
        ]}
      />
    )
    expect(
      screen.getByRole('figure', { name: 'Daily income vs spend' })
    ).toBeInTheDocument()
  })

  it('names the per-site trend chart', () => {
    render(
      <SiteTrendChart
        data={[{ date: '2026-08-01', site: 'Alpha', spend: 3, calls: 40 }]}
      />
    )
    expect(
      screen.getByRole('figure', { name: 'Daily requests by site' })
    ).toBeInTheDocument()
  })

  it('names the site balance distribution donut', () => {
    render(
      <SiteDistributionChart
        data={[
          {
            siteName: 'Alpha',
            platform: 'newapi',
            totalBalance: 100,
            totalSpend: 10,
            accountCount: 2,
          },
        ]}
        labels={{ balance: 'Balance', accounts: 'Accounts', share: 'Share' }}
      />
    )
    expect(
      screen.getByRole('figure', { name: 'Balance distribution by site' })
    ).toBeInTheDocument()
  })

  it('names the latency histogram (its single series label is empty)', () => {
    render(<LatencyHistogramChart data={[{ label: '<1s', count: 12 }]} />)
    expect(
      screen.getByRole('figure', { name: 'Requests by latency bucket' })
    ).toBeInTheDocument()
  })

  it('names the latency trend chart', () => {
    render(
      <LatencyTrendChart
        data={[{ date: '2026-08-01', metric: 'avg', latency: 120 }]}
        avgLabel='Average'
        p95Label='p95'
      />
    )
    expect(
      screen.getByRole('figure', { name: 'Latency trend (avg and p95)' })
    ).toBeInTheDocument()
  })

  it('names the model cost donut', () => {
    render(
      <ModelCostChart
        data={[{ model: 'gpt-5', label: '', cost: 9, calls: 30, tokens: 500 }]}
        labels={{
          cost: 'Cost',
          calls: 'Calls',
          tokens: 'Tokens',
          share: 'Share',
        }}
      />
    )
    expect(
      screen.getByRole('figure', { name: 'Cost share by model' })
    ).toBeInTheDocument()
  })
})
