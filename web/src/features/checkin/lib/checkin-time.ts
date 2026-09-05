// metapi-go features/checkin/lib — check-in log time helpers.
//
// Parsing and formatting are delegated to the shared stack in `@/lib/format`
// (`toTimestamp` + `formatDateTime`) so the naive
// UTC `created_at` values ("2026-08-11 12:30:00", no timezone suffix) are
// interpreted exactly like every other timestamp in the admin console. This
// module previously carried its own duplicate parse/format stack; it now only
// keeps the check-in-specific `datetime-local` input helpers.

import { formatDateTime } from '@/lib/format'

/**
 * Format a check-in log timestamp with seconds. Thin forwarder to
 * `formatDateTime`; unparseable input renders the shared "—" placeholder
 * (the lib contract — the previous raw-string passthrough was retired so
 * invalid data never leaks into the render tree).
 */
export function formatCheckinLogTime(
  value: string | null | undefined,
  locale: string,
  timeZone?: string
): string {
  return formatDateTime(value, locale, timeZone)
}

/**
 * Convert a `datetime-local` input value (`YYYY-MM-DDTHH:mm`, interpreted as
 * local) to epoch milliseconds. Returns `null` for empty/invalid input.
 * Used to compare against `created_at` when filtering the log window.
 */
export function localDatetimeInputToEpochMs(
  value: string | null | undefined,
  endOfDay = false
): number | null {
  if (!value) return null
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return null
  if (endOfDay) {
    date.setSeconds(59, 999)
  }
  return date.getTime()
}

/**
 * Convert a `datetime-local` input value (local) to a UTC RFC3339 string
 * *without* milliseconds (`YYYY-MM-DDTHH:mm:ssZ`). The checkin backend stores
 * `created_at` as a naive UTC RFC3339 string, so a lexicographic comparison
 * is correct — but only when the bound omits milliseconds, otherwise a stored
 * value at the same second sorts after the bound (`.Z` vs `Z`). Returns
 * `undefined` for empty/invalid input so callers can skip the server param.
 *
 * `endOfDay = true` pins the seconds to 59 so the upper bound includes the
 * whole minute selected by the minute-precision `datetime-local` input.
 */
export function localDatetimeInputToUtcRfc3339(
  value: string | null | undefined,
  endOfDay = false
): string | undefined {
  const epochMs = localDatetimeInputToEpochMs(value, endOfDay)
  if (epochMs === null) return undefined
  return new Date(epochMs).toISOString().replace(/\.\d{3}Z$/, 'Z')
}
