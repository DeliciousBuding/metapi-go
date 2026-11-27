import { useEffect, useState } from 'react'

/**
 * Delays showing a spinner until an active fetch exceeds `delayMs` (default 200ms)
 * so fast quiet polls and manual refreshes never flash a jittery spin.
 */
export function useDelayedSpin(isFetching: boolean, delayMs = 200): boolean {
  const [spinning, setSpinning] = useState(false)

  useEffect(() => {
    if (!isFetching) {
      setSpinning(false)
      return
    }
    const timer = window.setTimeout(() => {
      setSpinning(true)
    }, delayMs)
    return () => {
      window.clearTimeout(timer)
    }
  }, [isFetching, delayMs])

  return spinning
}

/**
 * Tracks a coarse clock (every 10s) while `updatedAt` is valid and there is
 * no active query error so relative freshness labels stay current without
 * per-second re-renders.
 */
export function useRelativeFreshnessNow(
  updatedAt: number,
  hasError = false
): number | null {
  const [now, setNow] = useState(() => Date.now())

  useEffect(() => {
    if (hasError || !updatedAt || updatedAt <= 0) return
    setNow(Date.now())
    const timer = window.setInterval(() => {
      setNow(Date.now())
    }, 10_000)
    return () => {
      window.clearInterval(timer)
    }
  }, [updatedAt, hasError])

  if (hasError || !updatedAt || updatedAt <= 0) return null
  return now
}
