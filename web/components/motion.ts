/**
 * prefers-reduced-motion (WCAG 2.3.3) — canvas animations (VChart specs)
 * cannot be disabled via CSS media queries; chart specs must gate on this
 * function. Same pattern as TokenRoutes' local helper (centralized so every
 * chart shares one implementation).
 */
export function prefersReducedMotion(): boolean {
  return typeof globalThis.matchMedia === 'function'
    && globalThis.matchMedia('(prefers-reduced-motion: reduce)').matches;
}
