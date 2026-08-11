import { describe, expect, it } from 'vitest'

import {
  formatCheckinLogTime,
  formatDateLocal,
  formatDateTimeMinuteLocal,
  toLocalDatetimeInputValue,
} from './checkinLogTime.js'

describe('formatCheckinLogTime', () => {
  it('formats sqlite UTC timestamp as local display time', () => {
    const formatted = formatCheckinLogTime(
      '2026-02-25 03:51:58',
      'en-US',
      'UTC'
    )
    expect(formatted).toBe('02/25/2026, 03:51:58')
  })

  it('keeps original value for invalid timestamp', () => {
    expect(formatCheckinLogTime('invalid-time', 'en-US', 'UTC')).toBe(
      'invalid-time'
    )
  })

  it('formats local date with minute precision helpers', () => {
    expect(formatDateLocal('2026-02-25 03:51:58', 'en-US', 'UTC')).toBe(
      '02/25/2026'
    )
    expect(
      formatDateTimeMinuteLocal('2026-02-25 03:51:58', 'en-US', 'UTC')
    ).toBe('02/25/2026, 03:51')
  })

  it('supports postgres-style timezone offset without colon', () => {
    expect(
      formatDateTimeMinuteLocal('2026-03-05 20:14:39+08', 'en-US', 'UTC')
    ).toBe('03/05/2026, 12:14')
    expect(
      formatDateTimeMinuteLocal('2026-03-05T20:14:39+0800', 'en-US', 'UTC')
    ).toBe('03/05/2026, 12:14')
  })

  it('supports unix timestamp strings', () => {
    expect(formatDateTimeMinuteLocal('1709640000', 'en-US', 'UTC')).toBe(
      '03/05/2024, 12:00'
    )
  })
})

describe('toLocalDatetimeInputValue', () => {
  it('renders YYYY-MM-DDTHH:mm for a known local instant', () => {
    const d = new Date(2026, 6, 30, 9, 5) // local 2026-07-30 09:05
    expect(toLocalDatetimeInputValue(d)).toBe('2026-07-30T09:05')
  })

  it('zero-pads single-digit fields', () => {
    const d = new Date(2026, 0, 1, 2, 3) // local 2026-01-01 02:03
    expect(toLocalDatetimeInputValue(d)).toBe('2026-01-01T02:03')
  })
})
