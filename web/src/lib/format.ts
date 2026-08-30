// metapi-go/lib/format — shared display formatters.
// Pure functions, no i18n runtime deps (locale-aware formatters take an
// explicit BCP-47 locale argument instead). Consolidates the per-feature
// formatters (dashboard formatInt/formatRatio, models formatPrice/
// formatLatency/formatSuccessRate) into one vocabulary so number/price/
// latency/token display stays consistent across the admin console.

export const EM_DASH = '—'

/**
 * Format an integer with locale grouping; returns "—" for null/NaN. Pass a
 * BCP-47 `locale` for explicit grouping (e.g. the active i18n language);
 * omitted, the browser default locale applies.
 */
export function formatInt(
  value: number | null | undefined,
  locale?: string
): string {
  if (value === null || value === undefined || !Number.isFinite(value)) {
    return EM_DASH
  }
  return value.toLocaleString(locale)
}

/**
 * Currency convention: every money field the backend returns — balance,
 * quota, used, todayIncome, unitCost, estimatedCost, and the per-million
 * pricing rates — is denominated in US dollars (USD). NewAPI/OneAPI-family
 * upstreams expose integer `quota` (1 USD = 500000 quota; Veloera uses
 * 1 USD = 1000000 quota); the platform adapters normalize to USD at the
 * boundary, so the frontend never renders raw quota or RMB. `$` is a
 * currency symbol, not translatable interface copy (see docs/api.md).
 */
export const USD_SYMBOL = '$'

/**
 * Format a USD amount (balance / cost / spend / quota): "$" + locale
 * grouping + fixed fraction digits (default 2). Returns "—" for
 * null/undefined/NaN so missing money is never rendered as $0.
 */
export function formatCurrency(
  value: number | null | undefined,
  options?: {
    /** Fraction digits (default 2; proxy-log costs use 4). */
    fractionDigits?: number
    /** BCP-47 locale for grouping/decimal separators (default: browser). */
    locale?: string
  }
): string {
  if (value === null || value === undefined || !Number.isFinite(value)) {
    return EM_DASH
  }
  const fractionDigits = options?.fractionDigits ?? 2
  const sign = value < 0 ? '-' : ''
  return `${sign}${USD_SYMBOL}${Math.abs(value).toLocaleString(
    options?.locale,
    {
      minimumFractionDigits: fractionDigits,
      maximumFractionDigits: fractionDigits,
    }
  )}`
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

/**
 * Format a per-million USD price/rate. Adaptive precision by default so zero
 * and sub-cent rates stay readable ($0, $0.0050, $2.50); pass `fractionDigits`
 * for a fixed decimal count (price-compare / route pricing keep 4 decimals to
 * preserve tiny cross-site differences).
 */
export function formatPrice(
  price: number | null | undefined,
  options?: {
    /** Fixed fraction digits; when omitted, adaptive (4 for <0.01 else 2). */
    fractionDigits?: number
    /** BCP-47 locale for grouping/decimal separators (default: browser). */
    locale?: string
  }
): string {
  if (price === null || price === undefined || !Number.isFinite(price)) {
    return EM_DASH
  }
  const digits = options?.fractionDigits
  if (digits !== undefined) {
    const sign = price < 0 ? '-' : ''
    return `${sign}${USD_SYMBOL}${Math.abs(price).toLocaleString(
      options?.locale,
      {
        minimumFractionDigits: digits,
        maximumFractionDigits: digits,
      }
    )}`
  }
  if (price === 0) return `${USD_SYMBOL}0`
  if (price < 0.01) return `${USD_SYMBOL}${price.toFixed(4)}`
  return `${USD_SYMBOL}${price.toFixed(2)}`
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

/** Format an already-percentage (0-100) value with one decimal: 99.95 → "100.0%". */
export function formatSuccessRate(rate: number | null | undefined): string {
  if (rate === null || rate === undefined || !Number.isFinite(rate)) {
    return EM_DASH
  }
  return `${rate.toFixed(1)}%`
}

const SECONDS_PER_MINUTE = 60
const SECONDS_PER_HOUR = 60 * SECONDS_PER_MINUTE
const SECONDS_PER_DAY = 24 * SECONDS_PER_HOUR
// Past 7 days a relative unit stops being actionable — surface an absolute
// date so a stale alert still reads clearly instead of "8 days ago".
const ABSOLUTE_THRESHOLD_SECONDS = 7 * SECONDS_PER_DAY

/**
 * Normalize a server timestamp to epoch milliseconds.
 *
 * Accepts epoch numbers, Dates, and ISO-like strings. The backend stores
 * `created_at` as naive UTC strings ("2026-08-11 12:30:00", no timezone
 * suffix); those are interpreted as UTC (not browser-local) so rendered
 * times never shift by the viewer's offset. Bare offsets (+0800 / +08) are
 * normalized, and 10/13-digit epoch strings are supported.
 *
 * Exported so feature modules (e.g. features/checkin) reuse this exact
 * parsing stack instead of re-implementing it.
 */
export function toTimestamp(
  value: string | number | Date | null | undefined
): number | null {
  if (value === null || value === undefined || value === '') return null
  if (value instanceof Date) {
    const dateMs = value.getTime()
    return Number.isFinite(dateMs) ? dateMs : null
  }
  if (typeof value === 'number') {
    return Number.isFinite(value) ? value : null
  }
  const raw = String(value).trim()
  if (!raw) return null
  if (/^\d{10,13}$/.test(raw)) {
    const asNumber = Number(raw)
    return raw.length === 13 ? asNumber : asNumber * 1000
  }
  let normalized = raw
  if (!normalized.includes('T') && normalized.includes(' ')) {
    normalized = normalized.replace(' ', 'T')
  }
  normalized = normalized.replace(/([+-]\d{2})(\d{2})$/, '$1:$2')
  normalized = normalized.replace(/([+-]\d{2})$/, '$1:00')
  const hasTimeZone = /[zZ]$|[+-]\d{2}:\d{2}$/.test(normalized)
  if (!hasTimeZone) {
    normalized = `${normalized}Z`
  }
  const timestamp = new Date(normalized).getTime()
  return Number.isFinite(timestamp) ? timestamp : null
}

/**
 * Shared core for the absolute date+time formatters: parse `value` via
 * `toTimestamp` and render it with `Intl.DateTimeFormat` using `options`,
 * optionally pinned to an IANA `timeZone` (viewer-local when omitted).
 * Returns "—" for null / empty / invalid input — raw strings must never
 * reach the render tree.
 */
function formatWithParts(
  value: string | number | Date | null | undefined,
  options: Intl.DateTimeFormatOptions,
  locale: string,
  timeZone?: string
): string {
  const timestamp = toTimestamp(value)
  if (timestamp === null) return EM_DASH
  return new Intl.DateTimeFormat(locale, {
    ...options,
    ...(timeZone ? { timeZone } : {}),
  }).format(timestamp)
}

/**
 * Format a timestamp as a localized absolute date+time WITH seconds
 * (e.g. "2026-08-21 15:04:05" in zh-CN). Returns "—" for null / empty /
 * invalid input — raw ISO strings must never reach the render tree.
 * The `locale` MUST be a BCP-47 tag — pass i18next's `i18n.language`
 * through `toBcp47()` first (`zhCN` would throw `RangeError`).
 * The optional `timeZone` pins rendering to an IANA zone (e.g. "UTC");
 * when omitted the viewer's local zone is used.
 */
export function formatDateTime(
  value: string | number | Date | null | undefined,
  locale: string,
  timeZone?: string
): string {
  return formatWithParts(
    value,
    {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      hour12: false,
    },
    locale,
    timeZone
  )
}

/**
 * Format a timestamp as a localized time-of-day with seconds ("15:04:05").
 * Returns "—" for null / empty / invalid input. The `locale` MUST be a
 * BCP-47 tag (pass `i18n.language` through `toBcp47()` first).
 */
export function formatTimeOfDay(
  value: string | number | Date | null | undefined,
  locale: string
): string {
  const timestamp = toTimestamp(value)
  if (timestamp === null) return EM_DASH
  return new Intl.DateTimeFormat(locale, {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  }).format(timestamp)
}

/**
 * Format a timestamp as a compact localized date for tight list cells
 * ("Aug 21"; the year is appended when it differs from the current year so
 * stale rows stay unambiguous). Returns "—" for null / empty / invalid
 * input. The `locale` MUST be a BCP-47 tag.
 */
export function formatShortDate(
  value: string | number | Date | null | undefined,
  locale: string
): string {
  const timestamp = toTimestamp(value)
  if (timestamp === null) return EM_DASH
  const sameYear =
    new Date(timestamp).getFullYear() === new Date().getFullYear()
  return new Intl.DateTimeFormat(locale, {
    month: 'short',
    day: 'numeric',
    ...(sameYear ? {} : { year: 'numeric' }),
  }).format(timestamp)
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
 * Detail line for log-table timestamps (the muted second line under the time
 * of day). Within the relative window: "8月23日 · 3 天前" (short date +
 * relative). Past the window, formatRelativeTime already renders an absolute
 * date, so the short date is dropped — otherwise the line duplicates the day
 * ("8月23日 · 2026年8月23日"). Empty string for null/invalid input.
 */
export function formatLogDateDetail(
  value: string | number | Date | null | undefined,
  locale: string
): string {
  const timestamp = toTimestamp(value)
  if (timestamp === null) return ''
  const ageSeconds = Math.abs(Math.round((timestamp - Date.now()) / 1000))
  if (ageSeconds >= ABSOLUTE_THRESHOLD_SECONDS) {
    return formatRelativeTime(value, locale)
  }
  return `${formatShortDate(value, locale)} · ${formatRelativeTime(value, locale)}`
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
