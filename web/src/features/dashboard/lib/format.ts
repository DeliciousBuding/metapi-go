// metapi-go/features/dashboard/lib — display formatters for dashboard
// metrics. Pure functions, no deps on i18n runtime (callers pass the locale-
// derived number format where needed). Kept tiny so the 4 sections share one
// formatting vocabulary instead of each inlining `${(x).toFixed(1)}%`.

/** Compact a large count: 1234 → "1.2K", 1_500_000 → "1.5M". */
export function formatCompactNumber(value: number): string {
  const absoluteValue = Math.abs(value)
  if (absoluteValue >= 1_000_000) {
    return `${(value / 1_000_000).toFixed(1)}M`
  }
  if (absoluteValue >= 1_000) {
    return `${(value / 1_000).toFixed(1)}K`
  }
  return String(value)
}

/** Format an integer with locale grouping; returns "—" for null/NaN. */
export function formatInt(value: number | null | undefined): string {
  if (value === null || value === undefined || !Number.isFinite(value)) {
    return '—'
  }
  return value.toLocaleString()
}

/** Format a 0..1 ratio as a percentage: 0.853 → "85.3%". */
export function formatPercent(
  rate: number | null | undefined,
  fractionDigits = 1,
): string {
  if (rate === null || rate === undefined || !Number.isFinite(rate)) {
    return '—'
  }
  return `${(rate * 100).toFixed(fractionDigits)}%`
}

/** Format a numerator/denominator pair as a percentage: 85, 100 → "85.0%". */
export function formatRatio(
  numerator: number,
  denominator: number,
  fractionDigits = 1,
): string {
  if (!denominator) return '—'
  return formatPercent(numerator / denominator, fractionDigits)
}

/** Format a latency in milliseconds: 123 → "123ms", 1500 → "1.50s". */
export function formatLatencyMs(
  milliseconds: number | null | undefined,
): string {
  if (milliseconds === null || milliseconds === undefined) return '—'
  if (!Number.isFinite(milliseconds)) return '—'
  if (milliseconds < 1000) return `${Math.round(milliseconds)}ms`
  return `${(milliseconds / 1000).toFixed(2)}s`
}

/** Format a currency-ish value with 2 decimals and a $ sign. */
export function formatCost(value: number | null | undefined): string {
  if (value === null || value === undefined || !Number.isFinite(value)) {
    return '—'
  }
  return `$${value.toFixed(2)}`
}
