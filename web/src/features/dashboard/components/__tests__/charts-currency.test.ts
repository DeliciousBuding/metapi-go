// Behavior test for the chart axis currency formatter (wave 7 observability):
// the old <1 branch rendered "$0.000000" on the zero tick beside "$1.000" —
// mixed precision read as a rounding bug. All bands now render 3 decimals
// (2 above $1000) so an axis reads one format throughout.

import { describe, expect, it } from 'vitest'

import { EM_DASH } from '@/lib/format'

import { formatChartCurrency } from '../charts'

describe('formatChartCurrency', () => {
  it('renders the zero tick as $0.000 (not $0.000000)', () => {
    expect(formatChartCurrency(0)).toBe('$0.000')
    expect(formatChartCurrency(0)).not.toBe('$0.000000')
  })

  it('keeps one precision per magnitude band', () => {
    expect(formatChartCurrency(0.5)).toBe('$0.500')
    expect(formatChartCurrency(1)).toBe('$1.000')
    expect(formatChartCurrency(4.2)).toBe('$4.200')
    expect(formatChartCurrency(1234.56)).toBe('$1234.56')
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
