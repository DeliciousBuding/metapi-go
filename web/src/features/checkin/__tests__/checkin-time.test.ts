import { describe, expect, it } from 'vitest'

import {
  formatCheckinLogTime,
  formatDateTimeMinuteLocal,
  localDatetimeInputToEpochMs,
  parseServerUtcDateTime,
  toLocalDatetimeInputValue,
} from '../lib/checkin-time'

// ---------------------------------------------------------------------------
// parseServerUtcDateTime
// ---------------------------------------------------------------------------

describe('parseServerUtcDateTime', () => {
  it('returns null for empty / nullish input', () => {
    expect(parseServerUtcDateTime(null)).toBeNull()
    expect(parseServerUtcDateTime(undefined)).toBeNull()
    expect(parseServerUtcDateTime('')).toBeNull()
    expect(parseServerUtcDateTime('   ')).toBeNull()
  })

  it('parses a naive UTC string by appending the Z suffix', () => {
    const date = parseServerUtcDateTime('2026-02-25 03:51:58')
    expect(date).not.toBeNull()
    expect(date?.toISOString()).toBe('2026-02-25T03:51:58.000Z')
  })

  it('parses an ISO string that already carries a Z', () => {
    const date = parseServerUtcDateTime('2026-02-25T03:51:58Z')
    expect(date?.toISOString()).toBe('2026-02-25T03:51:58.000Z')
  })

  it('normalises a bare +0800 offset to +08:00', () => {
    const date = parseServerUtcDateTime('2026-03-05T20:14:39+0800')
    expect(date?.toISOString()).toBe('2026-03-05T12:14:39.000Z')
  })

  it('normalises a bare +08 offset to +08:00 and a space separator to T', () => {
    const date = parseServerUtcDateTime('2026-03-05 20:14:39+08')
    expect(date?.toISOString()).toBe('2026-03-05T12:14:39.000Z')
  })

  it('parses 10-digit epoch seconds', () => {
    const date = parseServerUtcDateTime('1709640000')
    expect(date?.toISOString()).toBe('2024-03-05T12:00:00.000Z')
  })

  it('parses 13-digit epoch milliseconds', () => {
    const date = parseServerUtcDateTime('1709640000000')
    expect(date?.toISOString()).toBe('2024-03-05T12:00:00.000Z')
  })

  it('returns null for unparseable strings', () => {
    expect(parseServerUtcDateTime('not-a-date')).toBeNull()
    expect(parseServerUtcDateTime('2026-13-45')).toBeNull()
  })
})

// ---------------------------------------------------------------------------
// formatCheckinLogTime / formatDateTimeMinuteLocal
// ---------------------------------------------------------------------------

describe('formatCheckinLogTime', () => {
  it('formats a UTC timestamp with full precision in the requested locale', () => {
    const formatted = formatCheckinLogTime(
      '2026-02-25 03:51:58',
      'en-US',
      'UTC'
    )
    // Intl punctuation can drift across ICU versions, so assert the stable
    // numeric components instead of an exact string.
    expect(formatted).toContain('2026')
    expect(formatted).toContain('02')
    expect(formatted).toContain('25')
    expect(formatted).toContain('03:51:58')
  })

  it('renders in the caller-provided locale (no hardcoded default)', () => {
    const zhFormatted = formatCheckinLogTime(
      '2026-02-25 03:51:58',
      'zh-CN',
      'UTC'
    )
    const enFormatted = formatCheckinLogTime(
      '2026-02-25 03:51:58',
      'en-US',
      'UTC'
    )
    expect(zhFormatted).toContain('2026')
    expect(zhFormatted).toContain('03:51:58')
    // The locale actually drives the output: zh-CN leads with the year
    // ("2026-02-25 …") while en-US leads with month/day.
    expect(zhFormatted).not.toBe(enFormatted)
    expect(zhFormatted.startsWith('2026')).toBe(true)
  })

  it('returns the raw trimmed input for an unparseable value', () => {
    expect(formatCheckinLogTime('not-a-date', 'en-US')).toBe('not-a-date')
  })

  it('returns an em dash placeholder for nullish input', () => {
    expect(formatCheckinLogTime(null, 'en-US')).toBe('—')
    expect(formatCheckinLogTime('', 'en-US')).toBe('—')
  })
})

describe('formatDateTimeMinuteLocal', () => {
  it('formats to minute precision', () => {
    const formatted = formatDateTimeMinuteLocal(
      '2026-02-25 03:51:58',
      'en-US',
      'UTC'
    )
    expect(formatted).toContain('2026')
    expect(formatted).toContain('03:51')
    // seconds are dropped at minute precision
    expect(formatted).not.toContain('03:51:58')
  })
})

// ---------------------------------------------------------------------------
// datetime-local input helpers
// ---------------------------------------------------------------------------

describe('toLocalDatetimeInputValue', () => {
  it('formats a local Date as YYYY-MM-DDTHH:mm', () => {
    // new Date(2026, 6, 30, 9, 5) is 30 July 2026, 09:05 local.
    const value = toLocalDatetimeInputValue(new Date(2026, 6, 30, 9, 5))
    expect(value).toBe('2026-07-30T09:05')
  })

  it('zero-pads month / day / hour / minute', () => {
    const value = toLocalDatetimeInputValue(new Date(2026, 0, 5, 3, 7))
    expect(value).toBe('2026-01-05T03:07')
  })
})

describe('localDatetimeInputToEpochMs', () => {
  it('returns a positive epoch ms for a valid datetime-local value', () => {
    const epochMs = localDatetimeInputToEpochMs('2026-07-30T09:05')
    expect(epochMs).not.toBeNull()
    expect(typeof epochMs).toBe('number')
    expect(epochMs ?? 0).toBeGreaterThan(0)
  })

  it('extends to the end of the minute when endOfDay is true', () => {
    const start = localDatetimeInputToEpochMs('2026-07-30T09:05') ?? 0
    const end = localDatetimeInputToEpochMs('2026-07-30T09:05', true) ?? 0
    // ~59.999s later, timezone-independent.
    expect(end - start).toBeCloseTo(59_999, -2)
  })

  it('returns null for empty / invalid input', () => {
    expect(localDatetimeInputToEpochMs(null)).toBeNull()
    expect(localDatetimeInputToEpochMs('')).toBeNull()
    expect(localDatetimeInputToEpochMs('not-a-date')).toBeNull()
  })
})
