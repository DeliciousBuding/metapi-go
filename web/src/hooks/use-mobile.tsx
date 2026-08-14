// metapi-go/hooks — use-mobile rewritten with useSyncExternalStore.
//
// The previous implementation seeded `useState` with `undefined`, so the first
// client render always returned `false` (the `!!isMobile` coercion) and an
// effect patched it after paint — producing a one-frame desktop-sidebar flash
// for mobile users. useSyncExternalStore reads the real matchMedia value
// synchronously during render, so the first client render is already correct
// with no post-paint correction.
//
// The store lives at module scope so every component observes the same source.
// matchMedia is resolved lazily (on first getSnapshot/subscribe) so SSR and
// non-browser builds don't touch `window`. `getServerSnapshot` returns `false`
// to keep server output stable and hydration-consistent; React then
// synchronously re-renders with the real client value.

import * as React from 'react'

const MOBILE_BREAKPOINT = 768
// `max-width: ${MOBILE_BREAKPOINT - 1}px` mirrors the original `innerWidth <
// MOBILE_BREAKPOINT` check, so a 768px viewport stays classified as desktop
// (no behavior change — only the first-render flash is fixed).
const MOBILE_MEDIA_QUERY = `(max-width: ${MOBILE_BREAKPOINT - 1}px)`

type Listener = () => void

interface MobileStore {
  subscribe: (listener: Listener) => () => void
  getSnapshot: () => boolean
  getServerSnapshot: () => boolean
}

const isMobileStore: MobileStore = (() => {
  const listeners = new Set<Listener>()
  // Resolved lazily on the client; null on the server.
  let mediaQueryList: MediaQueryList | null = null
  let cachedSnapshot = false

  function resolveMediaQueryList(): MediaQueryList | null {
    if (
      typeof window === 'undefined' ||
      typeof window.matchMedia !== 'function'
    ) {
      return null
    }
    // Idempotent: the `=== null` guard means repeated calls (including React
    // strict-mode double renders) only attach one change listener.
    if (mediaQueryList === null) {
      mediaQueryList = window.matchMedia(MOBILE_MEDIA_QUERY)
      cachedSnapshot = mediaQueryList.matches
      mediaQueryList.addEventListener('change', handleChange)
    }
    return mediaQueryList
  }

  function handleChange(event: MediaQueryListEvent) {
    cachedSnapshot = event.matches
    for (const listener of listeners) {
      listener()
    }
  }

  return {
    subscribe(listener) {
      listeners.add(listener)
      // First subscribe on the client resolves matchMedia so the change
      // listener is attached even if getSnapshot hasn't run yet.
      resolveMediaQueryList()
      return () => {
        listeners.delete(listener)
      }
    },
    getSnapshot() {
      // Resolving here (not just in subscribe) is what makes the first client
      // render return the real value — useSyncExternalStore calls getSnapshot
      // during render, before subscribe runs in a passive effect.
      resolveMediaQueryList()
      return cachedSnapshot
    },
    getServerSnapshot() {
      return false
    },
  }
})()

export function useIsMobile(): boolean {
  return React.useSyncExternalStore(
    isMobileStore.subscribe,
    isMobileStore.getSnapshot,
    isMobileStore.getServerSnapshot
  )
}
