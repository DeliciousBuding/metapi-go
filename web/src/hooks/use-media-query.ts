// metapi-go/hooks — use-media-query ported from newapi. AGPL header stripped.
// Exports useMediaQuery — consumed by select.tsx (via @/hooks barrel).

import { useSyncExternalStore } from 'react'

/**
 * React hook for responsive media queries
 * @param query - CSS media query string (e.g., "(max-width: 640px)")
 * @returns boolean indicating if the query matches
 */
export function useMediaQuery(query: string): boolean {
  return useSyncExternalStore(
    (onStoreChange) => {
      if (typeof window === 'undefined') {
        return () => {}
      }

      const media = window.matchMedia(query)
      media.addEventListener('change', onStoreChange)
      return () => media.removeEventListener('change', onStoreChange)
    },
    () => {
      if (typeof window !== 'undefined') {
        return window.matchMedia(query).matches
      }
      return false
    },
    () => {
      return false
    }
  )
}
