import { describe, expect, it, vi } from 'vitest'

// The hook imports `@/context/theme-provider`, which is outside the relative
// import graph vitest resolves here. Stub it so the hook module loads
// cleanly — we only assert the static fallback constant, not the hook.
vi.mock('@/context/theme-provider', () => ({
  useTheme: () => ({ resolvedTheme: 'light' }),
}))

import { CHART_COLORS_FALLBACK } from '../hooks/use-chart-colors'

// The VChart canvas cannot resolve CSS var() references, so useChartColors
// resolves theme tokens to concrete hex strings. Under jsdom (and any
// environment where getComputedStyle returns "" for the custom properties)
// the hook falls back to CHART_COLORS_FALLBACK — this constant is therefore
// the documented contract for the no-CSS / unstyled path. We test it as a
// pure-data surface; the hook's rAF + MutationObserver re-read loop is
// integration-tested via the dashboard page snapshots.

describe('CHART_COLORS_FALLBACK', () => {
  it('exposes the documented axis / grid / onPrimary hex values', () => {
    expect(CHART_COLORS_FALLBACK.axisLabel).toBe('#6b7280')
    expect(CHART_COLORS_FALLBACK.grid).toBe('#e5e7eb')
    expect(CHART_COLORS_FALLBACK.onPrimary).toBe('#ffffff')
  })

  it('ships a 5-entry ordinal series palette', () => {
    expect(CHART_COLORS_FALLBACK.series).toHaveLength(5)
    expect(CHART_COLORS_FALLBACK.series).toEqual([
      '#3b82f6',
      '#06b6d4',
      '#8b5cf6',
      '#ec4899',
      '#10b981',
    ])
  })

  it('derives seriesSoft as each series colour + "33" (~20% alpha)', () => {
    expect(CHART_COLORS_FALLBACK.seriesSoft).toHaveLength(5)
    CHART_COLORS_FALLBACK.series.forEach((color, index) => {
      expect(CHART_COLORS_FALLBACK.seriesSoft[index]).toBe(`${color}33`)
    })
  })

  it('derives seriesFaint as each series colour + "0d" (~5% alpha)', () => {
    expect(CHART_COLORS_FALLBACK.seriesFaint).toHaveLength(5)
    CHART_COLORS_FALLBACK.series.forEach((color, index) => {
      expect(CHART_COLORS_FALLBACK.seriesFaint[index]).toBe(`${color}0d`)
    })
  })

  it('keeps the soft / faint arrays aligned with the series array by index', () => {
    expect(CHART_COLORS_FALLBACK.seriesSoft.length).toBe(
      CHART_COLORS_FALLBACK.series.length
    )
    expect(CHART_COLORS_FALLBACK.seriesFaint.length).toBe(
      CHART_COLORS_FALLBACK.series.length
    )
  })
})
