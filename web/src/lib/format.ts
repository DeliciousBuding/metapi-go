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
function formatPercent(
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
export function formatLatency(
  latency: number | null | undefined,
  options?: {
    /** Render values >= 1000ms as seconds instead of milliseconds. */
    autoSeconds?: boolean
    /** Fraction digits for the seconds form (default 1). */
    secondsDigits?: number
    /** When the seconds value reaches this, drop all fraction digits. */
    wholeSecondsThreshold?: number
    /** Insert a space between number and unit ("1.3 s" vs "1.3s"). */
    spaced?: boolean
  }
): string {
  if (latency === null || latency === undefined || !Number.isFinite(latency)) {
    return EM_DASH
  }
  const separator = options?.spaced ? ' ' : ''
  if (options?.autoSeconds && latency >= 1000) {
    const seconds = latency / 1000
    const digits =
      options?.wholeSecondsThreshold !== undefined &&
      seconds >= options.wholeSecondsThreshold
        ? 0
        : (options?.secondsDigits ?? 1)
    return `${seconds.toFixed(digits)}${separator}s`
  }
  return `${Math.round(latency)}${separator}ms`
}

/** Format an already-percentage (0-100) value without re-scaling. */
export function formatSuccessRate(rate: number | null | undefined): string {
  if (rate === null || rate === undefined || !Number.isFinite(rate)) {
    return EM_DASH
  }
  return `${Math.round(rate)}%`
}

const SECONDS_PER_MINUTE = 60
const SECONDS_PER_HOUR = 60 * SECONDS_PER_MINUTE
const SECONDS_PER_DAY = 24 * SECONDS_PER_HOUR
// Past 7 days a relative unit stops being actionable — surface an absolute
// date so a stale alert still reads clearly instead of "8 days ago".
const ABSOLUTE_THRESHOLD_SECONDS = 7 * SECONDS_PER_DAY

function toTimestamp(
  value: string | number | Date | null | undefined
): number | null {
  if (value === null || value === undefined || value === '') return null
  const timestamp =
    typeof value === 'number' ? value : new Date(value).getTime()
  return Number.isFinite(timestamp) ? timestamp : null
}

/**
 * Format a timestamp as a localized relative time ("just now", "2 minutes
 * ago", "3 hours ago", "2 days ago"). Items older than 7 days fall back to
 * a localized absolute date. Returns an empty string for null / empty /
 * invalid input so the caller can conditionally render without surfacing
 * "ago" with no date. Uses `Intl.RelativeTimeFormat` with `numeric: 'auto'`
 * so the output localizes with the active BCP-47 locale (e.g. "2 minutes
 * ago" in en, "2 分钟前" in zh-CN) without needing new translation keys.
 * The `locale` MUST be a BCP-47 tag — pass i18next's `i18n.language` through
 * `toBcp47()` first (`zhCN` would throw `RangeError`).
 */
export function formatRelativeTime(
  value: string | number | Date | null | undefined,
  locale: string
): string {
  const timestamp = toTimestamp(value)
  if (timestamp === null) return ''
  const deltaSeconds = Math.round((timestamp - Date.now()) / 1000)
  const absDeltaSeconds = Math.abs(deltaSeconds)
  if (absDeltaSeconds >= ABSOLUTE_THRESHOLD_SECONDS) {
    return new Intl.DateTimeFormat(locale, {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
    }).format(timestamp)
  }
  const relativeTime = new Intl.RelativeTimeFormat(locale, {
    numeric: 'auto',
  })
  if (absDeltaSeconds < SECONDS_PER_MINUTE) {
    return relativeTime.format(deltaSeconds, 'second')
  }
  if (absDeltaSeconds < SECONDS_PER_HOUR) {
    return relativeTime.format(
      Math.round(deltaSeconds / SECONDS_PER_MINUTE),
      'minute'
    )
  }
  if (absDeltaSeconds < SECONDS_PER_DAY) {
    return relativeTime.format(
      Math.round(deltaSeconds / SECONDS_PER_HOUR),
      'hour'
    )
  }
  return relativeTime.format(Math.round(deltaSeconds / SECONDS_PER_DAY), 'day')
}

/**
 * Format a timestamp as a localized absolute date+time for a `title`
 * tooltip (e.g. "Jan 15, 2026, 10:00 AM"). Returns an empty string for
 * null / empty / invalid input so the caller can omit the attribute. Uses
 * `Intl.DateTimeFormat` so the output localizes with the active BCP-47
 * locale. The `locale` MUST be a BCP-47 tag (pass `i18n.language` through
 * `toBcp47()` first).
 */
export function formatAbsoluteDateTime(
  value: string | number | Date | null | undefined,
  locale: string
): string {
  const timestamp = toTimestamp(value)
  if (timestamp === null) return ''
  return new Intl.DateTimeFormat(locale, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(timestamp)
}
