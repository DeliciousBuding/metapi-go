import { useEffect } from 'react'

function isEditableKeyboardTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  const tag = target.tagName
  if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return true
  if (target.isContentEditable) return true
  const role = target.getAttribute('role')
  if (
    role === 'combobox' ||
    role === 'listbox' ||
    role === 'slider' ||
    role === 'textbox'
  ) {
    return true
  }
  return false
}

export function useDetailSheetKeyboardNav(options: {
  open: boolean
  hasPrev?: boolean
  hasNext?: boolean
  onPrev?: () => void
  onNext?: () => void
}) {
  const { open, hasPrev, hasNext, onPrev, onNext } = options

  useEffect(() => {
    if (!open || (!onPrev && !onNext)) return

    function handleKeyDown(event: KeyboardEvent) {
      if (
        event.metaKey ||
        event.ctrlKey ||
        event.altKey ||
        event.shiftKey ||
        event.isComposing ||
        event.keyCode === 229
      ) {
        return
      }
      if (isEditableKeyboardTarget(event.target)) return
      if (event.key === 'ArrowUp' && hasPrev && onPrev) {
        event.preventDefault()
        onPrev()
      } else if (event.key === 'ArrowDown' && hasNext && onNext) {
        event.preventDefault()
        onNext()
      }
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => {
      window.removeEventListener('keydown', handleKeyDown)
    }
  }, [open, hasPrev, hasNext, onPrev, onNext])
}
