// Behavior test for the chart axis currency formatter: precision shrinks as
// magnitude grows so a tick never outgrows the fixed YAxis width ($160, not
// $160.000; $1.2K, not $1234.56) while sub-cent costs stay legible ($0.003).

import { describe, expect, it } from 'vitest'

import { EM_DASH } from '@/lib/format'

import {
  formatChartCurrency,
  makeChartCurrencyAxisFormatter,
} from '../charts-currency'

describe('formatChartCurrency', () => {
  it('renders the zero tick as $0.000 (not $0.000000)', () => {
    expect(formatChartCurrency(0)).toBe('$0.000')
    expect(formatChartCurrency(0)).not.toBe('$0.000000')
  })

  it('shrinks precision as magnitude grows so ticks fit the fixed axis width', () => {
    expect(formatChartCurrency(0.5)).toBe('$0.500')
    expect(formatChartCurrency(1)).toBe('$1.00')
    expect(formatChartCurrency(4.2)).toBe('$4.20')
    expect(formatChartCurrency(160)).toBe('$160')
    expect(formatChartCurrency(1234.56)).toBe('$1.2K')
  })

  it('keeps sub-cent per-call costs legible', () => {
    expect(formatChartCurrency(0.0025)).toBe('$0.003')
  })

  it('renders an em dash for non-finite values', () => {
    expect(formatChartCurrency(Number.NaN)).toBe(EM_DASH)
    expect(formatChartCurrency(Number.POSITIVE_INFINITY)).toBe(EM_DASH)
  })

  it('formats negative budgets with the same precision', () => {
    expect(formatChartCurrency(-0.4)).toBe('$-0.400')
  })
})

describe('makeChartCurrencyAxisFormatter', () => {
  it('picks ONE precision per axis from the maximum (no mixed formats)', () => {
    const fmt = makeChartCurrencyAxisFormatter(160)
    const ticks = [0, 40, 80, 120, 160].map(fmt)
    // Every tick integer-only — the "$80.00 next to $160" mix is the bug.
    for (const tick of ticks) expect(tick).toMatch(/^\$[\d]+$/)
    expect(fmt(160)).toBe('$160')
  })

  it('keeps 3 decimals uniformly on a sub-dollar axis', () => {
    const fmt = makeChartCurrencyAxisFormatter(0.5)
    expect(fmt(0)).toBe('$0.000')
    expect(fmt(0.5)).toBe('$0.500')
  })

  it('compacts thousand-plus axes and renders a plain $0 zero tick', () => {
    const fmt = makeChartCurrencyAxisFormatter(4200)
    expect(fmt(0)).toBe('$0')
    expect(fmt(1500)).toBe('$1.5K')
    expect(fmt(4200)).toBe('$4.2K')
  })

  it('renders two decimals for mid-range axes', () => {
    const fmt = makeChartCurrencyAxisFormatter(8)
    expect(fmt(0)).toBe('$0.00')
    expect(fmt(2.5)).toBe('$2.50')
  })
})
