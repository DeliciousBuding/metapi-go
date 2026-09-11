// metapi-go/data-table — useDebounce, package-private.
//
// The toolbar's global-filter input is the only consumer: it pushes every
// keystroke into TanStack's global filter, which re-runs filtering over the
// whole page of rows, so the value is settled before it reaches the table.
//
// Deliberately kept inside this package rather than promoted to `@/hooks` — a
// second consumer elsewhere is the signal to move it, not a reason to pre-empt
// it. Note the direction of the dependency: this waits for the value to stop
// changing. Server-side filtering wants the opposite (fire immediately, cancel
// a stale in-flight request), which is TanStack Query's job, not this hook's.

import { useEffect, useState } from 'react'

/** Returns `value` once it has stopped changing for `delay` ms. */
export function useDebounce<T>(value: T, delay: number): T {
  const [debounced, setDebounced] = useState(value)

  useEffect(() => {
    const timer = setTimeout(() => setDebounced(value), delay)
    return () => clearTimeout(timer)
  }, [value, delay])

  return debounced
}
