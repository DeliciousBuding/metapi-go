// metapi-go/features/settings/lib — ScheduleSpec v1 mapping tests.
// Covers the four default cron expressions round-trip, unmappable → custom
// (verbatim), and the "unchanged semantics return the original bytes" rule.

import { describe, expect, it } from 'vitest'

import {
  cronToSchedule,
  scheduleEqual,
  scheduleFromLegacy,
  scheduleToCron,
  specToLegacyMode,
} from '../schedule'

describe('cronToSchedule', () => {
  it('maps daily 08:00', () => {
    expect(cronToSchedule('0 8 * * *')).toEqual({
      version: 1,
      kind: 'daily',
      time: '08:00',
    })
  })

  it('maps daily 23:58', () => {
    expect(cronToSchedule('58 23 * * *')).toEqual({
      version: 1,
      kind: 'daily',
      time: '23:58',
    })
  })

  it('maps every-hour to interval 1', () => {
    expect(cronToSchedule('0 * * * *')).toEqual({
      version: 1,
      kind: 'interval',
      everyHours: 1,
    })
  })

  it('maps every-6-hours to interval 6', () => {
    expect(cronToSchedule('0 */6 * * *')).toEqual({
      version: 1,
      kind: 'interval',
      everyHours: 6,
    })
  })

  it('maps every-24-hours to interval 24', () => {
    expect(cronToSchedule('0 */24 * * *')).toEqual({
      version: 1,
      kind: 'interval',
      everyHours: 24,
    })
  })

  it('maps a 6-field zero-seconds expression to the 5-field form', () => {
    expect(cronToSchedule('0 0 8 * * *')).toEqual({
      version: 1,
      kind: 'daily',
      time: '08:00',
    })
  })

  it('keeps weekday expressions as custom verbatim', () => {
    const cron = '0 8 * * 1-5'
    expect(cronToSchedule(cron)).toEqual({ version: 1, kind: 'custom', cron })
  })

  it('keeps non-zero minutes with wildcard hour as custom', () => {
    const cron = '*/5 * * * *'
    expect(cronToSchedule(cron)).toEqual({ version: 1, kind: 'custom', cron })
  })

  it('keeps interval over 24 hours as custom', () => {
    const cron = '0 */25 * * *'
    expect(cronToSchedule(cron)).toEqual({ version: 1, kind: 'custom', cron })
  })

  it('keeps malformed expressions as custom verbatim', () => {
    const cron = '0 8 * * * extra'
    expect(cronToSchedule(cron)).toEqual({ version: 1, kind: 'custom', cron })
  })

  it('keeps empty input as custom with empty cron', () => {
    expect(cronToSchedule('')).toEqual({ version: 1, kind: 'custom', cron: '' })
  })

  it('keeps null/undefined as custom with empty cron', () => {
    expect(cronToSchedule(null)).toEqual({ version: 1, kind: 'custom', cron: '' })
    expect(cronToSchedule(undefined)).toEqual({ version: 1, kind: 'custom', cron: '' })
  })
})

describe('scheduleToCron', () => {
  it('emits a canonical cron for daily', () => {
    expect(scheduleToCron({ version: 1, kind: 'daily', time: '08:00' })).toBe(
      '0 8 * * *',
    )
  })

  it('emits a canonical cron for interval', () => {
    expect(
      scheduleToCron({ version: 1, kind: 'interval', everyHours: 6 }),
    ).toBe('0 */6 * * *')
  })

  it('returns custom cron verbatim', () => {
    expect(
      scheduleToCron({ version: 1, kind: 'custom', cron: '0 8 * * 1-5' }),
    ).toBe('0 8 * * 1-5')
  })

  it('returns undefined for window (no deterministic cron)', () => {
    expect(
      scheduleToCron({
        version: 1,
        kind: 'window',
        windowStart: '00:00',
        windowEnd: '23:59',
      }),
    ).toBeUndefined()
  })

  it('returns undefined for missing spec', () => {
    expect(scheduleToCron(undefined)).toBeUndefined()
  })

  it('returns the original bytes when semantics are unchanged (6-field)', () => {
    const original = '0 0 8 * * *'
    expect(
      scheduleToCron({ version: 1, kind: 'daily', time: '08:00' }, original),
    ).toBe(original)
  })

  it('returns the original bytes when semantics are unchanged (interval 1)', () => {
    const original = '0 * * * *'
    expect(
      scheduleToCron({ version: 1, kind: 'interval', everyHours: 1 }, original),
    ).toBe(original)
  })

  it('returns the original bytes for interval 6', () => {
    const original = '0 */6 * * *'
    expect(
      scheduleToCron({ version: 1, kind: 'interval', everyHours: 6 }, original),
    ).toBe(original)
  })

  it('emits canonical cron when semantics changed', () => {
    expect(
      scheduleToCron({ version: 1, kind: 'daily', time: '08:00' }, '0 9 * * *'),
    ).toBe('0 8 * * *')
  })
})

describe('scheduleEqual', () => {
  it('matches identical daily specs', () => {
    expect(
      scheduleEqual(
        { version: 1, kind: 'daily', time: '08:00' },
        { version: 1, kind: 'daily', time: '08:00' },
      ),
    ).toBe(true)
  })

  it('rejects differing daily times', () => {
    expect(
      scheduleEqual(
        { version: 1, kind: 'daily', time: '08:00' },
        { version: 1, kind: 'daily', time: '09:00' },
      ),
    ).toBe(false)
  })

  it('rejects different kinds', () => {
    expect(
      scheduleEqual(
        { version: 1, kind: 'daily', time: '08:00' },
        { version: 1, kind: 'interval', everyHours: 6 },
      ),
    ).toBe(false)
  })

  it('matches identical window specs', () => {
    expect(
      scheduleEqual(
        { version: 1, kind: 'window', windowStart: '00:00', windowEnd: '08:00' },
        { version: 1, kind: 'window', windowStart: '00:00', windowEnd: '08:00' },
      ),
    ).toBe(true)
  })
})

describe('specToLegacyMode', () => {
  it('maps interval to interval', () => {
    expect(
      specToLegacyMode({ version: 1, kind: 'interval', everyHours: 6 }),
    ).toBe('interval')
  })

  it('maps window to window', () => {
    expect(
      specToLegacyMode({
        version: 1,
        kind: 'window',
        windowStart: '00:00',
        windowEnd: '23:59',
      }),
    ).toBe('window')
  })

  it('maps daily and custom to cron', () => {
    expect(specToLegacyMode({ version: 1, kind: 'daily', time: '08:00' })).toBe('cron')
    expect(specToLegacyMode({ version: 1, kind: 'custom', cron: 'x' })).toBe('cron')
  })

  it('maps undefined to cron', () => {
    expect(specToLegacyMode(undefined)).toBe('cron')
  })
})

describe('scheduleFromLegacy', () => {
  it('builds a window spec from legacy mode + boundaries', () => {
    expect(
      scheduleFromLegacy({
        mode: 'window',
        windowStart: '02:00',
        windowEnd: '06:00',
      }),
    ).toEqual({ version: 1, kind: 'window', windowStart: '02:00', windowEnd: '06:00' })
  })

  it('defaults window boundaries when absent', () => {
    expect(scheduleFromLegacy({ mode: 'window' })).toEqual({
      version: 1,
      kind: 'window',
      windowStart: '00:00',
      windowEnd: '23:59',
    })
  })

  it('derives the spec from cron', () => {
    expect(scheduleFromLegacy({ cron: '0 8 * * *' })).toEqual({
      version: 1,
      kind: 'daily',
      time: '08:00',
    })
  })

  it('falls back to interval hours when mode is interval and cron is unmappable', () => {
    const spec = scheduleFromLegacy({
      cron: '0 8 * * 1-5',
      mode: 'interval',
      intervalHours: 6,
    })
    expect(spec).toEqual({ version: 1, kind: 'interval', everyHours: 6 })
  })
})
