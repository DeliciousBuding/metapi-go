// metapi-go/hooks — useScrollFade: mark a horizontally scrollable container
// with data-fade-left / data-fade-right attributes so CSS can render edge
// fades ONLY where more content exists (the flat-design affordance for
// "this table scrolls"). Without it, wide tables silently clip their last
// columns at narrow widths (round-2 tablet review: 8+ routes at 768px).

import { useEffect, useRef } from 'react'

/**
 * Ref for the scroll container. Sets `data-fade-left` / `data-fade-right`
 * (`'true'`/`'false'`) on mount, scroll, and any size change of the
 * container or its content (column toggles change scrollWidth without a
 * container resize, so the first child is observed too).
 */
export function useScrollFade<T extends HTMLElement>() {
  const ref = useRef<T>(null)

  useEffect(() => {
    const el = ref.current
    if (!el) return
    const update = () => {
      const { scrollLeft, scrollWidth, clientWidth } = el
      el.dataset.fadeLeft = String(scrollLeft > 1)
      el.dataset.fadeRight = String(scrollLeft + clientWidth < scrollWidth - 1)
    }
    update()
    el.addEventListener('scroll', update, { passive: true })
    // ResizeObserver is absent under jsdom (unit tests render the table but
    // never layout); the scroll listener still covers real browsers there.
    if (typeof ResizeObserver === 'undefined') {
      return () => el.removeEventListener('scroll', update)
    }
    const observer = new ResizeObserver(update)
    observer.observe(el)
    if (el.firstElementChild) observer.observe(el.firstElementChild)
    return () => {
      el.removeEventListener('scroll', update)
      observer.disconnect()
    }
  }, [])

  return ref
}
