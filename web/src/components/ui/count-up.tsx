// metapi-go/ui — CountUp number animation (prefers-reduced-motion aware).
//
// Animates a numeric value from its previous value to the new value using
// requestAnimationFrame. First mount uses a longer ramp (1500ms); subsequent
// value updates use a shorter ramp (800ms). When the user prefers reduced
// motion the final value renders immediately with no animation.
//
// The visual animated number is aria-hidden; a screen-reader-only span always
// carries the final formatted value so assistive tech never hears in-between
// frames.

'use client'

import { useEffect, useRef, useState } from 'react'

import { cn } from '@/lib/utils'

type CountUpProps = {
  value: number
  /** Formats the animated numeric value. Defaults to locale string. */
  format?: (value: number) => string
  /** First-mount animation duration in ms. */
  initialDuration?: number
  /** Subsequent value-change animation duration in ms. */
  updateDuration?: number
  className?: string
}

const DEFAULT_INITIAL_DURATION = 1500
const DEFAULT_UPDATE_DURATION = 800

function prefersReducedMotion(): boolean {
  return (
    typeof window !== 'undefined' &&
    window.matchMedia('(prefers-reduced-motion: reduce)').matches
  )
}

const easeOutCubic = (t: number) => 1 - Math.pow(1 - t, 3)

export function CountUp({
  value,
  format = (n) => n.toLocaleString(),
  initialDuration = DEFAULT_INITIAL_DURATION,
  updateDuration = DEFAULT_UPDATE_DURATION,
  className,
}: CountUpProps) {
  const [display, setDisplay] = useState(() =>
    prefersReducedMotion() || !Number.isFinite(value) ? value : 0
  )
  const previousValueRef = useRef(0)
  const mountedRef = useRef(false)
  const frameRef = useRef<number | null>(null)

  useEffect(() => {
    const from = previousValueRef.current
    const to = value

    if (prefersReducedMotion() || !Number.isFinite(to)) {
      setDisplay(to)
      previousValueRef.current = to
      mountedRef.current = true
      return undefined
    }

    const duration = mountedRef.current ? updateDuration : initialDuration
    mountedRef.current = true
    const start = performance.now()

    const tick = (now: number) => {
      const progress = Math.min((now - start) / duration, 1)
      const current = from + (to - from) * easeOutCubic(progress)
      setDisplay(current)
      if (progress < 1) {
        frameRef.current = requestAnimationFrame(tick)
      } else {
        previousValueRef.current = to
      }
    }

    frameRef.current = requestAnimationFrame(tick)
    return () => {
      if (frameRef.current !== null) {
        cancelAnimationFrame(frameRef.current)
      }
      previousValueRef.current = to
    }
  }, [value, initialDuration, updateDuration])

  return (
    <span className={cn('tabular-nums', className)}>
      <span aria-hidden='true'>{format(display)}</span>
      <span className='sr-only'>{format(value)}</span>
    </span>
  )
}
