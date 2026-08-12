// metapi-go features/checkin/lib — UTC date parsing & formatting helpers.
//
// The backend stores `created_at` as a naive UTC string (e.g.
// "2026-08-11 12:30:00") that may arrive without a timezone suffix. These
// helpers normalise such values to real Dates before formatting them for the
// UI, mirroring the legacy `pages/helpers/checkinLogTime` module which is
// outside the `src/` alias scope and therefore cannot be imported here.

function parseServerUtcDate(value: string | null | undefined): {
  date: Date | null
  raw: string
} {
  if (!value) return { date: null, raw: '' }

  const raw = String(value).trim()
  if (!raw) return { date: null, raw: '' }

  // Epoch milliseconds / seconds shorthand.
  if (/^\d{10,13}$/.test(raw)) {
    const asNumber = Number(raw)
    if (Number.isFinite(asNumber)) {
      const epochMs = raw.length === 13 ? asNumber : asNumber * 1000
      const date = new Date(epochMs)
      if (!Number.isNaN(date.getTime())) return { date, raw }
    }
  }

  let normalized = raw
  if (!normalized.includes('T') && normalized.includes(' ')) {
    normalized = normalized.replace(' ', 'T')
  }
  // Normalise bare offsets like +0800 → +08:00, or +08 → +08:00.
  normalized = normalized.replace(/([+-]\d{2})(\d{2})$/, '$1:$2')
  normalized = normalized.replace(/([+-]\d{2})$/, '$1:00')

  const hasTimeZone = /[zZ]$|[+-]\d{2}:\d{2}$/.test(normalized)
  if (!hasTimeZone) {
    normalized = `${normalized}Z`
  }

  const date = new Date(normalized)
  if (Number.isNaN(date.getTime())) return { date: null, raw }

  return { date, raw }
}

export function parseServerUtcDateTime(
  value: string | null | undefined
): Date | null {
  return parseServerUtcDate(value).date
}

function formatWithParts(
  value: string | null | undefined,
  options: Intl.DateTimeFormatOptions,
  locale = 'zh-CN',
  timeZone?: string
): string {
  const { date, raw } = parseServerUtcDate(value)
  if (!date) return raw || '-'
  return new Intl.DateTimeFormat(locale, {
    ...options,
    ...(timeZone ? { timeZone } : {}),
  }).format(date)
}

export function formatCheckinLogTime(
  value: string | null | undefined,
  locale?: string,
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

export function formatDateTimeMinuteLocal(
  value: string | null | undefined,
  locale = 'zh-CN',
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
      hour12: false,
    },
    locale,
    timeZone
  )
}

/**
 * Format a Date as the local `datetime-local` input value
 * (`YYYY-MM-DDTHH:mm`), the shape the date-range preset inputs expect.
 */
export function toLocalDatetimeInputValue(date: Date): string {
  const pad = (n: number) => String(n).padStart(2, '0')
  return (
    `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}` +
    `T${pad(date.getHours())}:${pad(date.getMinutes())}`
  )
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
