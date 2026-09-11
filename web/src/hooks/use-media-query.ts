// metapi-go/hooks — useMediaQuery: subscribe a component to a CSS media query.
//
// `useSyncExternalStore` rather than state + effect so the first render already
// reports the real match: an effect-seeded hook renders one frame of the wrong
// layout, which is exactly the sidebar flash `use-mobile.tsx` was rewritten to
// remove. The two hooks stay separate on purpose — `use-mobile.tsx` observes
// one fixed breakpoint through a module-scope store shared by the app chrome,
// while this one takes an arbitrary query and owns one `MediaQueryList` per
// subscriber. Sole call site: the table→card-list switch in
// `components/data-table/layout/data-table-page.tsx` (640px, deliberately
// narrower than the 767px sidebar threshold — see `lib/breakpoints.ts`).
//
// No `typeof window` guards: metapi-go is a browser-only SPA (Rsbuild static
// output embedded in the Go binary, no SSR and no prerender), so `window`
// always exists at render time.

import { useSyncExternalStore } from 'react'

/** Whether `query` currently matches. Re-renders the caller when it flips. */
export function useMediaQuery(query: string): boolean {
  return useSyncExternalStore(
    (onStoreChange) => {
      const media = window.matchMedia(query)
      media.addEventListener('change', onStoreChange)
      return () => media.removeEventListener('change', onStoreChange)
    },
    () => window.matchMedia(query).matches,
    // Required by the hook signature; never read, since nothing hydrates.
    () => false
  )
}
