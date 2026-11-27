import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { toBcp47 } from '@/i18n/languages'

import { makeChartCurrencyAxisFormatter } from './charts-currency'

/**
 * Uniform currency tick formatter for one axis: the band is chosen once from
 * the largest numeric value in the pivoted rows so every tick on the axis
 * shares a precision (mixed precision on one axis reads as a rounding bug).
 */
export function useCurrencyAxisTicks(
  rows: ReadonlyArray<object>
): (value: number) => string {
  return useMemo(() => {
    let max = 0
    for (const row of rows) {
      for (const value of Object.values(row)) {
        if (typeof value === 'number' && Math.abs(value) > max) {
          max = Math.abs(value)
        }
      }
    }
    return makeChartCurrencyAxisFormatter(max)
  }, [rows])
}

// ---------------------------------------------------------------------------
// Locale-aware date axis/tooltip helpers. The stats endpoints emit plain
// `YYYY-MM-DD` day strings; parse the calendar parts directly so the tick
// never shifts a day across timezones (a bare `new Date('2026-08-21')` is
// UTC midnight and renders as the previous evening in negative offsets).
// ---------------------------------------------------------------------------

function parseDayLabel(value: string | number): Date | null {
  const text = String(value)
  const calendarMatch = /^(\d{4})-(\d{2})-(\d{2})/.exec(text)
  if (calendarMatch) {
    const date = new Date(
      Number(calendarMatch[1]),
      Number(calendarMatch[2]) - 1,
      Number(calendarMatch[3])
    )
    return Number.isNaN(date.getTime()) ? null : date
  }
  const parsed = new Date(text)
  return Number.isNaN(parsed.getTime()) ? null : parsed
}

/** Hook returning a locale-aware "Aug 21" tick formatter for date axes. */
export function useDateTickFormatter(): (value: string | number) => string {
  const { i18n } = useTranslation()
  const locale = toBcp47(i18n.language || 'en')
  // Intl.DateTimeFormat construction is expensive; memoize per locale so the
  // axis re-render only reuses the formatter instead of re-parsing options.
  return useMemo(() => {
    const formatter = new Intl.DateTimeFormat(locale, {
      month: 'short',
      day: 'numeric',
    })
    return (value: string | number): string => {
      const date = parseDayLabel(value)
      if (!date) return String(value)
      return formatter.format(date)
    }
  }, [locale])
}

/** Hook returning a locale-aware "Aug 21, 2026" formatter for tooltip headers. */
export function useDateTooltipFormatter(): (value: string | number) => string {
  const { i18n } = useTranslation()
  const locale = toBcp47(i18n.language || 'en')
  return useMemo(() => {
    const formatter = new Intl.DateTimeFormat(locale, {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
    })
    return (value: string | number): string => {
      const date = parseDayLabel(value)
      if (!date) return String(value)
      return formatter.format(date)
    }
  }, [locale])
}
