// metapi-go/features/dashboard — unit tests for the VChart spec builders.
//
// The builders are pure functions (no DOM / VChart runtime), so the donut
// tooltip shape can be asserted directly: every slice must carry a text
// title (site/model name) plus labelled value rows so colour is never the
// only encoding (a11y-checklist.md §8).

import { describe, expect, it } from 'vitest'

import {
  buildModelCostSpec,
  buildSiteDistributionSpec,
  type ModelCostTooltipLabels,
  type SiteDistributionTooltipLabels,
} from '../lib/chart-specs'

const COLORS = {
  axisLabel: '#6b7280',
  grid: '#e5e7eb',
  onPrimary: '#ffffff',
  series: ['#3b82f6', '#06b6d4', '#8b5cf6', '#ec4899', '#10b981'],
  seriesSoft: ['#3b82f633'],
  seriesFaint: ['#3b82f60d'],
}

type TooltipShape = {
  title: { value: (datum: unknown) => string }
  content: Array<{ key: string; value: (datum: unknown) => string }>
}

function tooltipOf(spec: Record<string, unknown>): TooltipShape {
  const tooltip = spec.tooltip as { mark: TooltipShape }
  return tooltip.mark
}

function datumWith(extra: Record<string, unknown>): unknown {
  return { _percent_: 14.2, ...extra }
}

describe('buildSiteDistributionSpec', () => {
  const labels: SiteDistributionTooltipLabels = {
    balance: 'Balance',
    accounts: 'Accounts',
    share: 'Share',
  }

  it('maps slices into the pie values and keeps the non-colour fields', () => {
    const spec = buildSiteDistributionSpec(
      COLORS,
      '#6b7280',
      [
        {
          siteName: 'OpenAI',
          platform: 'platform-a',
          totalBalance: 16.62,
          totalSpend: 4.1,
          accountCount: 3,
        },
      ],
      labels
    )
    const values = (spec.data as Array<{ values: unknown[] }>)[0].values
    expect(values).toEqual([
      {
        siteName: 'OpenAI',
        value: 16.62,
        accountCount: 3,
        totalSpend: 4.1,
      },
    ])
  })

  it('titles the tooltip with the site name', () => {
    const spec = buildSiteDistributionSpec(
      COLORS,
      '#6b7280',
      [
        {
          siteName: 'OpenAI',
          platform: 'p',
          totalBalance: 1,
          totalSpend: 0,
          accountCount: 1,
        },
      ],
      labels
    )
    const tooltip = tooltipOf(spec)
    expect(tooltip.title.value(datumWith({ siteName: 'OpenAI' }))).toBe(
      'OpenAI'
    )
    expect(tooltip.title.value(null)).toBe('-')
  })

  it('labels the tooltip rows with the i18n keys passed in', () => {
    const spec = buildSiteDistributionSpec(COLORS, '#6b7280', [], labels)
    expect(tooltipOf(spec).content.map((row) => row.key)).toEqual([
      'Balance',
      'Accounts',
      'Share',
    ])
  })

  it('formats balance, accounts and share as text values', () => {
    const spec = buildSiteDistributionSpec(COLORS, '#6b7280', [], labels)
    const tooltip = tooltipOf(spec)
    expect(tooltip.content[0].value(datumWith({ value: 16.62 }))).toBe(
      '$16.620'
    )
    expect(tooltip.content[1].value(datumWith({ accountCount: 3 }))).toBe('3')
    expect(tooltip.content[2].value(datumWith({}))).toBe('14.2%')
  })
})

describe('buildModelCostSpec', () => {
  const labels: ModelCostTooltipLabels = {
    cost: 'Cost',
    calls: 'Requests',
    tokens: 'Tokens',
    share: 'Share',
  }

  it('maps rows into the pie values and keeps the tooltip fields', () => {
    const spec = buildModelCostSpec(
      COLORS,
      '#6b7280',
      [
        {
          model: 'deepseek-v3',
          label: 'deepseek-v3',
          cost: 0.2,
          calls: 916,
          tokens: 253141,
        },
      ],
      labels
    )
    const values = (spec.data as Array<{ values: unknown[] }>)[0].values
    expect(values).toEqual([
      { model: 'deepseek-v3', value: 0.2, calls: 916, tokens: 253141 },
    ])
  })

  it('titles the tooltip with the model name', () => {
    const spec = buildModelCostSpec(COLORS, '#6b7280', [], labels)
    expect(
      tooltipOf(spec).title.value(datumWith({ model: 'deepseek-v3' }))
    ).toBe('deepseek-v3')
    expect(tooltipOf(spec).title.value(undefined)).toBe('-')
  })

  it('labels the tooltip rows with the i18n keys passed in', () => {
    const spec = buildModelCostSpec(COLORS, '#6b7280', [], labels)
    expect(tooltipOf(spec).content.map((row) => row.key)).toEqual([
      'Cost',
      'Requests',
      'Tokens',
      'Share',
    ])
  })

  it('formats cost, calls, tokens and share as text values', () => {
    const spec = buildModelCostSpec(COLORS, '#6b7280', [], labels)
    const tooltip = tooltipOf(spec)
    expect(tooltip.content[0].value(datumWith({ value: 0.2 }))).toBe(
      '$0.200000'
    )
    expect(tooltip.content[1].value(datumWith({ calls: 916 }))).toBe('916')
    expect(tooltip.content[2].value(datumWith({ tokens: 253141 }))).toBe(
      '253,141'
    )
    expect(tooltip.content[3].value(datumWith({}))).toBe('14.2%')
  })
})
