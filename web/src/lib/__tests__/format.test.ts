// metapi-go/lib/__tests__ — shared display formatter contract tests.
// The admin console converges all number/currency/date/latency rendering on
// lib/format; these tests pin the shared vocabulary (EM_DASH placeholders,
// currency symbol placement, naive-UTC parsing, locale-aware date/time).

import { describe, expect, it } from 'vitest'

import {
  EM_DASH,
  formatCurrency,
  formatDateTime,
  formatInt,
  formatLatency,
  formatPrice,
  formatRelativeTime,
  formatShortDate,
  formatSuccessRate,
  formatTimeOfDay,
} from '../format'

// Renders the epoch with the same Intl options a formatter under test uses,
// so assertions verify parsing semantics (naive-UTC → epoch) without baking
// in the CI machine's timezone.
function renderInLocalZone(
  epochMs: number,
  options: Intl.DateTimeFormatOptions,
  locale = 'en-US'
): string {
  return new Intl.DateTimeFormat(locale, options).format(epochMs)
}

describe('formatInt', () => {
  it('groups thousands', () => {
    expect(formatInt(1234567)).toBe('1,234,567')
  })

  it('renders counts of zero as 0', () => {
    expect(formatInt(0)).toBe('0')
  })

  it('renders null / undefined / NaN as an em dash', () => {
    expect(formatInt(null)).toBe(EM_DASH)
    expect(formatInt(undefined)).toBe(EM_DASH)
    expect(formatInt(Number.NaN)).toBe(EM_DASH)
  })
})

describe('formatCurrency', () => {
  it('renders $ + grouping + 2 decimals by default', () => {
    expect(formatCurrency(1234567.891)).toBe('$1,234,567.89')
    expect(formatCurrency(0)).toBe('$0.00')
  })

  it('supports custom fraction digits (proxy-log costs keep 4)', () => {
    expect(formatCurrency(0.123456, { fractionDigits: 4 })).toBe('$0.1235')
  })

  it('keeps the sign before the currency symbol', () => {
    expect(formatCurrency(-12.5)).toBe('-$12.50')
  })

  it('renders null / undefined / NaN as an em dash (never invents $0)', () => {
    expect(formatCurrency(null)).toBe(EM_DASH)
    expect(formatCurrency(undefined)).toBe(EM_DASH)
    expect(formatCurrency(Number.NaN)).toBe(EM_DASH)
  })
})

describe('formatPrice', () => {
  it('includes the currency symbol at every precision tier', () => {
    expect(formatPrice(0)).toBe('$0')
    expect(formatPrice(0.005)).toBe('$0.0050')
    expect(formatPrice(2.5)).toBe('$2.50')
  })

  it('supports a fixed decimal count for cross-site price comparison', () => {
    expect(formatPrice(2.5, { fractionDigits: 4 })).toBe('$2.5000')
    expect(formatPrice(0.5123, { fractionDigits: 4 })).toBe('$0.5123')
    expect(formatPrice(1234.5, { fractionDigits: 4 })).toBe('$1,234.5000')
  })

  it('renders null / undefined / NaN as an em dash', () => {
    expect(formatPrice(null)).toBe(EM_DASH)
    expect(formatPrice(undefined)).toBe(EM_DASH)
  })
})

describe('formatSuccessRate', () => {
  it('keeps one decimal instead of rounding away it', () => {
    expect(formatSuccessRate(99.95)).toBe('100.0%')
    expect(formatSuccessRate(85.34)).toBe('85.3%')
    expect(formatSuccessRate(0)).toBe('0.0%')
  })

  it('renders null / undefined / NaN as an em dash', () => {
    expect(formatSuccessRate(null)).toBe(EM_DASH)
    expect(formatSuccessRate(undefined)).toBe(EM_DASH)
  })
})

describe('formatLatency', () => {
  it('rounds milliseconds by default', () => {
    expect(formatLatency(849.6)).toBe('850ms')
  })

  it('promotes >= 1000ms to seconds with autoSeconds', () => {
    expect(formatLatency(12345, { autoSeconds: true })).toBe('12.3s')
    expect(formatLatency(12345, { autoSeconds: true, spaced: true })).toBe(
      '12.3 s'
    )
  })

  it('drops fraction digits past the whole-seconds threshold', () => {
    expect(
      formatLatency(105_000, {
        autoSeconds: true,
        spaced: true,
        secondsDigits: 2,
        wholeSecondsThreshold: 100,
      })
    ).toBe('105 s')
  })

  it('renders null / undefined / NaN as an em dash', () => {
    expect(formatLatency(null)).toBe(EM_DASH)
    expect(formatLatency(undefined)).toBe(EM_DASH)
  })
})

// The backend stores created_at as naive UTC ("2026-02-25 03:51:58"); the
// shared formatters must treat it as UTC, not browser-local. The epoch below
// is Date.UTC(2026, 1, 25, 3, 51, 58).
const NAIVE_UTC_EPOCH_MS = Date.UTC(2026, 1, 25, 3, 51, 58)

describe('formatDateTime', () => {
  it('formats a naive UTC string with seconds (locale-aware)', () => {
    const formatted = formatDateTime('2026-02-25 03:51:58', 'en-US')
    expect(formatted).toBe(
      renderInLocalZone(NAIVE_UTC_EPOCH_MS, {
        year: 'numeric',
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit',
        hour12: false,
      })
    )
  })

  it('accepts epoch milliseconds and ISO strings with offsets', () => {
    const offsetEpochMs = Date.UTC(2026, 2, 5, 12, 14, 39)
    expect(formatDateTime(1772712879000, 'en-US')).toBe(
      formatDateTime(offsetEpochMs, 'en-US')
    )
    expect(formatDateTime('2026-03-05T20:14:39+08:00', 'en-US')).toBe(
      formatDateTime(offsetEpochMs, 'en-US')
    )
  })

  it('renders null / empty / invalid input as an em dash', () => {
    expect(formatDateTime(null, 'en-US')).toBe(EM_DASH)
    expect(formatDateTime('', 'en-US')).toBe(EM_DASH)
    expect(formatDateTime('not-a-date', 'en-US')).toBe(EM_DASH)
  })
})

describe('formatTimeOfDay', () => {
  it('renders only the clock part with seconds', () => {
    const formatted = formatTimeOfDay('2026-02-25 03:51:58', 'en-US')
    expect(formatted).toBe(
      renderInLocalZone(NAIVE_UTC_EPOCH_MS, {
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit',
        hour12: false,
      })
    )
  })

  it('renders invalid input as an em dash', () => {
    expect(formatTimeOfDay('nope', 'en-US')).toBe(EM_DASH)
  })
})

describe('formatShortDate', () => {
  it('renders month + day without the year inside the current year', () => {
    const januaryFirstThisYear = new Date(new Date().getFullYear(), 0, 1, 12)
    const formatted = formatShortDate(januaryFirstThisYear, 'en-US')
    expect(formatted).toBe('Jan 1')
  })

  it('appends the year for dates outside the current year', () => {
    const formatted = formatShortDate(
      new Date(new Date().getFullYear() - 1, 7, 21, 12),
      'en-US'
    )
    expect(formatted).toBe(`Aug 21, ${new Date().getFullYear() - 1}`)
  })

  it('renders invalid input as an em dash', () => {
    expect(formatShortDate(null, 'en-US')).toBe(EM_DASH)
  })
})

describe('formatRelativeTime', () => {
  it('localizes recent timestamps without new translation keys', () => {
    const twoHoursAgo = new Date(Date.now() - 2 * 60 * 60 * 1000)
    expect(formatRelativeTime(twoHoursAgo, 'en-US')).toBe('2 hours ago')
    expect(formatRelativeTime(twoHoursAgo, 'zh-CN')).toBe('2小时前')
  })

  it('returns an empty string for missing input', () => {
    expect(formatRelativeTime(null, 'en-US')).toBe('')
    expect(formatRelativeTime('', 'en-US')).toBe('')
  })
})
