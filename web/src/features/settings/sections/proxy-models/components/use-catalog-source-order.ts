import {
  useRef,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
  type PointerEvent as ReactPointerEvent,
} from 'react'

import type { CatalogSource } from '@/lib/api'

export function useCatalogSourceOrder(
  sources: CatalogSource[],
  disabled: boolean,
  onReorder: (move: { sourceId: number; targetIndex: number }) => void
) {
  const rowRefs = useRef<Array<HTMLTableRowElement | null>>([])
  const dragState = useRef<{
    index: number
    mode: 'pointer' | 'keyboard'
  } | null>(null)
  const overIndexRef = useRef<number | null>(null)
  const [dragIndex, setDragIndex] = useState<number | null>(null)
  const [overIndex, setOverIndex] = useState<number | null>(null)
  const [keyboardGrabIndex, setKeyboardGrabIndex] = useState<number | null>(
    null
  )

  // Pointer path: the row handle captures the pointer and
  // tracks the row under the cursor; on release the drop index is committed
  // as an absolute sortOrder. Storing the live target in a ref keeps the
  // pointerup handler race-free even when the last pointermove and pointerup
  // land in the same frame.
  // Keyboard path (WCAG 2.1.1 alternative, APG keyboard-sortable pattern):
  // Enter/Space grabs the focused handle, ArrowUp/ArrowDown move the drop
  // highlight, Space/Enter drops, Escape cancels. The same endDrag() commit
  // finishes both paths.
  function findRowIndexAt(clientY: number): number {
    const rows = rowRefs.current
    for (let i = 0; i < rows.length; i++) {
      const row = rows[i]
      if (!row) continue
      if (clientY < row.getBoundingClientRect().bottom) return i
    }
    return Math.max(rows.length - 1, 0)
  }

  function onDragHandlePointerDown(
    event: ReactPointerEvent<HTMLButtonElement>,
    index: number
  ) {
    if (disabled) return
    // A pointer drag takes over from an active keyboard grab; clear the
    // keyboard-specific visual state so a click-without-move cannot commit
    // the stale keyboard target.
    if (dragState.current?.mode === 'keyboard') {
      setKeyboardGrabIndex(null)
      setDragIndex(null)
      setOverIndex(null)
      overIndexRef.current = null
    }
    dragState.current = { index, mode: 'pointer' }
    event.currentTarget.setPointerCapture(event.pointerId)
  }

  function onDragHandlePointerMove(
    event: ReactPointerEvent<HTMLButtonElement>
  ) {
    const state = dragState.current
    if (!state) return
    const next = findRowIndexAt(event.clientY)
    setDragIndex(state.index)
    setOverIndex(next)
    overIndexRef.current = next
  }

  function onDragHandleKeyDown(
    event: ReactKeyboardEvent<HTMLButtonElement>,
    index: number
  ) {
    const state = dragState.current
    const inKeyboardGrab = state?.mode === 'keyboard'
    if (!inKeyboardGrab) {
      if (event.key === ' ' || event.key === 'Enter') {
        event.preventDefault()
        dragState.current = { index, mode: 'keyboard' }
        setKeyboardGrabIndex(index)
        setDragIndex(index)
        setOverIndex(index)
        overIndexRef.current = index
      }
      return
    }
    // Grab in progress: the focused handle owns the interaction. Move only
    // the drop highlight; the row order commits once on drop.
    const pos = overIndexRef.current
    if (pos === null) return
    event.preventDefault()
    switch (event.key) {
      case 'ArrowDown': {
        const next = Math.min(pos + 1, sources.length - 1)
        overIndexRef.current = next
        setOverIndex(next)
        break
      }
      case 'ArrowUp': {
        const next = Math.max(pos - 1, 0)
        overIndexRef.current = next
        setOverIndex(next)
        break
      }
      case ' ':
      case 'Enter':
        endDrag(true)
        break
      case 'Escape':
        endDrag(false)
        break
      default:
        break
    }
  }

  function endDrag(commit: boolean) {
    const state = dragState.current
    if (!state) return
    dragState.current = null
    setDragIndex(null)
    setOverIndex(null)
    setKeyboardGrabIndex(null)
    const targetIndex = overIndexRef.current
    overIndexRef.current = null
    if (commit && targetIndex !== null && targetIndex !== state.index) {
      onReorder({ sourceId: sources[state.index].id, targetIndex })
    }
  }

  return {
    rowRefs,
    dragIndex,
    overIndex,
    handleProps: (index: number) => ({
      disabled,
      onPointerDown: (event: ReactPointerEvent<HTMLButtonElement>) =>
        onDragHandlePointerDown(event, index),
      onPointerMove: onDragHandlePointerMove,
      onPointerUp: () => endDrag(true),
      onPointerCancel: () => endDrag(false),
      onKeyDown: (event: ReactKeyboardEvent<HTMLButtonElement>) =>
        onDragHandleKeyDown(event, index),
      onBlur: () => {
        if (dragState.current?.mode === 'keyboard') endDrag(false)
      },
      'aria-pressed': keyboardGrabIndex === index,
    }),
  }
}
