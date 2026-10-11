import { useEffect, useRef, useState } from 'react'

export function useScopeSelectionFocus() {
  const listRef = useRef<HTMLDivElement>(null)
  const [focusRequest, setFocusRequest] = useState(0)
  useEffect(() => {
    if (focusRequest === 0) return
    const option = listRef.current?.querySelector<HTMLElement>(
      'input:checked, [role="checkbox"][aria-checked="true"], [data-unresolved-ref]'
    )
    option?.scrollIntoView?.({ block: 'nearest' })
    option?.focus()
  }, [focusRequest])
  return { listRef, focusSelected: () => setFocusRequest((value) => value + 1) }
}
