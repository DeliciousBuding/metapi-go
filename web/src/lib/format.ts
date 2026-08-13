// metapi-go/lib/format — shared display formatters.
// Pure functions, no i18n runtime deps. Consolidates the per-feature
// formatters (dashboard formatInt/formatRatio, models formatPrice/
// formatLatency/formatSuccessRate) into one vocabulary so number/price/
// latency/token display stays consistent across the admin console.

const EM_DASH = '—'

/** Format an integer with locale grouping; returns "—" for null/NaN. */
export function formatInt(value: number | null | undefined): string {
  if (value === null || value === undefined || !Number.isFinite(value)) {
    return EM_DASH
  }
  return value.toLocaleString()
}

/** Format a 0..1 ratio as a percentage: 0.853 → "85.3%". */
export function formatPercent(
  rate: number | null | undefined,
  fractionDigits = 1
): string {
  if (rate === null || rate === undefined || !Number.isFinite(rate)) {
    return EM_DASH
  }
  return `${(rate * 100).toFixed(fractionDigits)}%`
}

/** Format a numerator/denominator pair as a percentage: 85, 100 → "85.0%". */
export function formatRatio(
  numerator: number,
  denominator: number,
  fractionDigits = 1
): string {
  if (!denominator) return EM_DASH
  return formatPercent(numerator / denominator, fractionDigits)
}

/** Format a per-million USD price with adaptive precision. */
export function formatPrice(price: number | null | undefined): string {
  if (price === null || price === undefined || !Number.isFinite(price)) {
    return EM_DASH
  }
  if (price === 0) return '0'
  if (price < 0.01) return price.toFixed(4)
  return price.toFixed(2)
}

/** Format latency in milliseconds. */
export function formatLatency(latency: number | null | undefined): string {
  if (latency === null || latency === undefined || !Number.isFinite(latency)) {
    return EM_DASH
  }
  return `${Math.round(latency)}ms`
}

/** Format an already-percentage (0-100) value without re-scaling. */
export function formatSuccessRate(rate: number | null | undefined): string {
  if (rate === null || rate === undefined || !Number.isFinite(rate)) {
    return EM_DASH
  }
  return `${Math.round(rate)}%`
}

/** Compact a large number: 1234 → "1.2K", 3456789 → "3.5M". */
export function formatCompactNumber(value: number | null | undefined): string {
  if (value === null || value === undefined || !Number.isFinite(value)) {
    return EM_DASH
  }
  return new Intl.NumberFormat(undefined, {
    notation: 'compact',
    maximumFractionDigits: 1,
  }).format(value)
}

/** Format a token count with compact notation for large values. */
export function formatTokens(value: number | null | undefined): string {
  return formatCompactNumber(value)
}
