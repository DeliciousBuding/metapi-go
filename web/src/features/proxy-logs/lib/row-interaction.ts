import type { KeyboardEvent } from 'react'

const INTERACTIVE_TARGET_SELECTOR =
  'button, a, [role="menuitem"], input, select, textarea, [contenteditable="true"]'

export function isInteractiveRowTarget(target: EventTarget | null): boolean {
  return (
    target instanceof Element &&
    target.closest(INTERACTIVE_TARGET_SELECTOR) !== null
  )
}

export function handleProxyLogRowKeyDown(
  event: KeyboardEvent<HTMLElement>,
  onActivate: () => void
) {
  if (event.key !== 'Enter' && event.key !== ' ') return
  if (isInteractiveRowTarget(event.target)) return

  event.preventDefault()
  onActivate()
}
