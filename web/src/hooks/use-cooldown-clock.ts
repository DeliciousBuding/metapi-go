import { useEffect, useState } from 'react'

function parseDeadlineMs(value: string | number | null | undefined): number {
  if (value === null || value === undefined || value === '') return NaN
  if (typeof value === 'number') return Number.isFinite(value) ? value : NaN
  return Date.parse(value)
}

/**
 * Single shared 1-second clock for a list/panel containing cooling items.
 * Never starts a timer when no deadline lies in the future.
 */
export function useCooldownClock(
  deadlines: readonly (string | number | null | undefined)[]
): number {
  const [now, setNow] = useState(() => Date.now())
  const hasFutureDeadline = deadlines.some((deadline) => {
    const ts = parseDeadlineMs(deadline)
    return Number.isFinite(ts) && ts > now
  })

  useEffect(() => {
    if (!hasFutureDeadline) return
    const timer = window.setInterval(() => {
      setNow(Date.now())
    }, 1000)
    return () => {
      window.clearInterval(timer)
    }
  }, [hasFutureDeadline])

  return now
}

/**
 * Formats remaining cooldown duration as MM:SS (or HH:MM:SS when >= 1 hour).
 * Returns null if the deadline is missing, invalid, or already expired.
 */
export function formatRemainingCooldown(
  until: string | number | null | undefined,
  now: number
): string | null {
  const ts = parseDeadlineMs(until)
  if (!Number.isFinite(ts)) return null
  const diffMs = ts - now
  if (diffMs <= 0) return null
  const totalSeconds = Math.ceil(diffMs / 1000)
  const hours = Math.floor(totalSeconds / 3600)
  const minutes = Math.floor((totalSeconds % 3600) / 60)
  const seconds = totalSeconds % 60
  const pad = (value: number) => String(value).padStart(2, '0')
  if (hours > 0) {
    return `${pad(hours)}:${pad(minutes)}:${pad(seconds)}`
  }
  return `${pad(minutes)}:${pad(seconds)}`
}
