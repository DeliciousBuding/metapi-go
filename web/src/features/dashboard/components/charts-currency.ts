import { EM_DASH } from '@/lib/format'

/**
 * Adaptive currency formatting for chart axes/tooltips. One precision per
 * magnitude band — the 6-decimal branch is gone because it rendered the zero
 * tick as "$0.000000" next to "$1.000" on the same axis (mixed precision
 * reads as a rounding bug). 3 decimals stays legible for sub-cent per-call
 * costs ($0.002) and keeps every tick on an axis in the same format.
 */
export function formatChartCurrency(value: number): string {
  if (!Number.isFinite(value)) return EM_DASH
  const magnitude = Math.abs(value)
  if (magnitude >= 1000) return `$${value.toFixed(2)}`
  return `$${value.toFixed(3)}`
}
