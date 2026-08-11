// metapi-go/features/dashboard/lib — display formatters for dashboard metrics.
// Pure functions, no deps on i18n runtime. Kept tiny so the 4 sections share
// one formatting vocabulary instead of each inlining `${(x).toFixed(1)}%`.

/** Format an integer with locale grouping; returns "—" for null/NaN. */
export function formatInt(value: number | null | undefined): string {
  if (value === null || value === undefined || !Number.isFinite(value)) {
    return '—'
  }
  return value.toLocaleString()
}

/** Format a 0..1 ratio as a percentage: 0.853 → "85.3%". */
function formatPercent(
  rate: number | null | undefined,
  fractionDigits = 1
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
  fractionDigits = 1
): string {
  if (!denominator) return '—'
  return formatPercent(numerator / denominator, fractionDigits)
}
