import { describe, expect, it } from 'vitest'

import {
  formatCheckinLogTime,
  localDatetimeInputToEpochMs,
} from '../lib/checkin-time'

// ---------------------------------------------------------------------------
// formatCheckinLogTime
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

  it('renders the shared em-dash placeholder for an unparseable value', () => {
    // Contract decision (2026-08-23): the checkin formatter now forwards to
    // the shared `@/lib/format` stack, whose contract is "invalid input never
    // reaches the render tree" → "—". The previous raw-string passthrough was
    // retired. Adopting lib's contract has the smaller blast radius: the only
    // live callers render server-generated `created_at` timestamps (always
    // valid), whereas keeping passthrough would have required loosening
    // `formatDateTime` for every consumer codebase-wide.
    expect(formatCheckinLogTime('not-a-date', 'en-US')).toBe('—')
  })

  it('returns an em dash placeholder for nullish input', () => {
    expect(formatCheckinLogTime(null, 'en-US')).toBe('—')
    expect(formatCheckinLogTime('', 'en-US')).toBe('—')
  })
})

// ---------------------------------------------------------------------------
// datetime-local input helpers
// ---------------------------------------------------------------------------

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
