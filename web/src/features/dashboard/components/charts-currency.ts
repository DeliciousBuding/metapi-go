import { EM_DASH, USD_SYMBOL } from '@/lib/format'

/**
 * Adaptive currency formatting for chart axes/tooltips. Precision shrinks as
 * magnitude grows: sub-cent per-call costs keep 3 decimals ($0.002), while
 * three-figure ticks drop decimals entirely ($160) and four-figure ticks
 * compact ($1.2K) — the fixed-width Y axis can never clip a label. (The old
 * uniform-precision rule produced "$0.000000"-style zero ticks and, later,
 * clipped "$160.000" labels.)
 */
/**
 * Axis tick formatter for one chart: precision is chosen ONCE from the axis
 * maximum so every tick on the axis shares a format (mixed precision on one
 * axis reads as a rounding bug), while the band itself guarantees the label
 * fits the fixed YAxis width.
 */
export function makeChartCurrencyAxisFormatter(
  maxAbs: number
): (value: number) => string {
  if (!Number.isFinite(maxAbs)) return formatChartCurrency
  if (maxAbs >= 1000) {
    return (value) =>
      value === 0
        ? `${USD_SYMBOL}0`
        : `${USD_SYMBOL}${(value / 1000).toFixed(1)}K`
  }
  if (maxAbs >= 100) {
    return (value) => `${USD_SYMBOL}${value.toFixed(0)}`
  }
  if (maxAbs >= 1) {
    return (value) => `${USD_SYMBOL}${value.toFixed(2)}`
  }
  return (value) => `${USD_SYMBOL}${value.toFixed(3)}`
}

export function formatChartCurrency(value: number): string {
  if (!Number.isFinite(value)) return EM_DASH
  const magnitude = Math.abs(value)
  // Axis ticks are read at a glance, not audited — precision budget shrinks
  // as magnitude grows so the label never outgrows the fixed YAxis width
  // (a "$160.000" tick at width=48 lost its leading characters, issue: the
  // round-2 design review's clipped-axis finding).
  if (magnitude >= 1000) return `${USD_SYMBOL}${(value / 1000).toFixed(1)}K`
  if (magnitude >= 100) return `${USD_SYMBOL}${value.toFixed(0)}`
  if (magnitude >= 1) return `${USD_SYMBOL}${value.toFixed(2)}`
  return `${USD_SYMBOL}${value.toFixed(3)}`
}
