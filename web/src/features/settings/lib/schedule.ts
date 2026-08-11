// metapi-go/features/settings/lib — ScheduleSpec v1 mapping.
//
// The backend keeps the legacy `*_cron` field as the source of truth and
// exposes/accepts a semantic ScheduleSpec (version 1) alongside it. This
// module holds the pure mapping logic: cron → spec (lossy-friendly: anything
// unmappable becomes `custom` with the original expression verbatim), and
// spec → cron (returns the original expression unchanged when the semantic
// meaning did not change, otherwise a canonical 5-field cron). `window` has
// no deterministic cron equivalent — it stays a spec and is only supported
// for the checkin job (backend uses random-in-window scheduling).

import type { ScheduleSpecV1 } from '@/lib/api'

export const SCHEDULE_SPEC_VERSION = 1 as const

export function cronToSchedule(
  cron: string | null | undefined,
): ScheduleSpecV1 {
  const raw = cron?.trim() ?? ''
  if (!raw) {
    return { version: 1, kind: 'custom', cron: '' }
  }
  const fields = raw.split(/\s+/).filter(Boolean)
  let f = fields
  // 6-field cron with a zero seconds field maps to the same 5-field form.
  if (f.length === 6 && f[0] === '0') {
    f = f.slice(1)
  }
  if (f.length !== 5) {
    return { version: 1, kind: 'custom', cron: raw }
  }
  const [minute, hour, dom, mon, dow] = f
  if (dom !== '*' || mon !== '*' || dow !== '*') {
    return { version: 1, kind: 'custom', cron: raw }
  }
  const minuteNum = Number(minute)
  if (minute === '*' || !Number.isInteger(minuteNum) || minuteNum < 0 || minuteNum > 59) {
    return { version: 1, kind: 'custom', cron: raw }
  }
  const intervalMatch = /^\*$|^\*\/(\d+)$/.exec(hour)
  if (intervalMatch) {
    const everyHours = intervalMatch[1] ? Number(intervalMatch[1]) : 1
    if (everyHours >= 1 && everyHours <= 24) {
      return { version: 1, kind: 'interval', everyHours }
    }
  }
  const hourNum = Number(hour)
  if (Number.isInteger(hourNum) && hourNum >= 0 && hourNum <= 23) {
    return {
      version: 1,
      kind: 'daily',
      time: `${String(hourNum).padStart(2, '0')}:${String(minuteNum).padStart(2, '0')}`,
    }
  }
  return { version: 1, kind: 'custom', cron: raw }
}

/** Parse a 5-field cron (no seconds) into a canonical daily/interval cron. */
function canonicalCron(spec: ScheduleSpecV1): string {
  switch (spec.kind) {
    case 'daily': {
      const [hh = '00', mm = '00'] = spec.time.split(':')
      return `${Number(mm)} ${Number(hh)} * * *`
    }
    case 'interval':
      return `0 */${spec.everyHours} * * *`
    default:
      return ''
  }
}

export function scheduleEqual(a: ScheduleSpecV1, b: ScheduleSpecV1): boolean {
  if (a.version !== b.version || a.kind !== b.kind) {
    return false
  }
  switch (a.kind) {
    case 'daily':
      return b.kind === 'daily' && a.time === b.time
    case 'interval':
      return b.kind === 'interval' && a.everyHours === b.everyHours
    case 'window':
      return (
        b.kind === 'window'
        && a.windowStart === b.windowStart
        && a.windowEnd === b.windowEnd
      )
    case 'custom':
      return b.kind === 'custom' && a.cron === b.cron
  }
}

/**
 * spec → cron. Returns `undefined` for `window` (no deterministic cron).
 * When `originalCron` is given and still expresses the same semantics, the
 * original bytes are returned so unchanged schedules never drift.
 */
export function scheduleToCron(
  spec: ScheduleSpecV1 | undefined,
  originalCron?: string,
): string | undefined {
  if (!spec) {
    return undefined
  }
  if (spec.kind === 'custom') {
    return spec.cron
  }
  if (spec.kind === 'window') {
    return undefined
  }
  const canonical = canonicalCron(spec)
  if (originalCron) {
    const original = cronToSchedule(originalCron)
    if (scheduleEqual(original, spec)) {
      return originalCron
    }
  }
  return canonical
}

/** Map a ScheduleSpec to the legacy `checkinScheduleMode` value. */
export function specToLegacyMode(
  spec: ScheduleSpecV1 | undefined,
): 'cron' | 'interval' | 'window' {
  if (spec?.kind === 'interval') {
    return 'interval'
  }
  if (spec?.kind === 'window') {
    return 'window'
  }
  return 'cron'
}

/** Build a ScheduleSpec from the legacy runtime fields (GET shape). */
export function scheduleFromLegacy(opts: {
  cron?: string
  mode?: string
  intervalHours?: number
  windowStart?: string
  windowEnd?: string
}): ScheduleSpecV1 {
  if (opts.mode === 'window') {
    return {
      version: 1,
      kind: 'window',
      windowStart: opts.windowStart || '00:00',
      windowEnd: opts.windowEnd || '23:59',
    }
  }
  const fromCron = cronToSchedule(opts.cron)
  if (fromCron.kind !== 'custom') {
    return fromCron
  }
  if (opts.mode === 'interval' && opts.intervalHours) {
    return { version: 1, kind: 'interval', everyHours: opts.intervalHours }
  }
  return fromCron
}
