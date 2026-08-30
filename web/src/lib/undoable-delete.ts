// metapi-go/lib — undoable delete (S7 删除+undo 档).
//
// Gmail-style deferred delete: the row leaves the list immediately
// (optimistic cache update) and a toast offers 撤销 for a short window;
// the real DELETE fires only when the toast auto-closes or is swiped away.
// Clicking 撤销 restores the exact cache snapshot — nothing was ever
// deleted server-side, so undo is always safe. If the commit fails, the
// snapshot is restored and the error toast explains why.
//
// Use this for leaf single-row deletes (announcements, redirects, catalog
// sources, routes, keys, tokens). Cascading deletes (site → accounts,
// account → tokens) and bulk/irreversible ops keep their confirm dialogs —
// those are the 批量确认 / typed-confirm tiers, not this one.

import { useQueryClient } from '@tanstack/react-query'
import { useCallback } from 'react'

import { toast } from '@/lib/toast'

/** Default undo window. Long enough to notice, short enough to not stall. */
const UNDO_WINDOW_MS = 6000

export type UndoableDeleteParams<TCache, TItem> = {
  /** The row being deleted (passed to removeFromCache + deleteFn). */
  item: TItem
  /** List query whose cached payload is optimistically updated. */
  queryKey: readonly unknown[]
  /** Pure: return the cache payload without the item. */
  removeFromCache: (data: TCache, item: TItem) => TCache
  /** The real DELETE (fires only after the undo window closes). */
  deleteFn: (item: TItem) => Promise<unknown>
  /** Toast text — the caller's localized "deleted" message. */
  title: string
  /** Undo action label (t('common.undo')). */
  undoLabel: string
  /** Failure toast text shown when the deferred DELETE rejects. */
  errorTitle: string | ((error: unknown) => string)
  /**
   * Extra query keys to invalidate after the commit lands (e.g. a parent
   * list that derives counts from the deleted row). The primary queryKey is
   * always invalidated.
   */
  alsoInvalidate?: Array<readonly unknown[]>
  /** Overrides the 6s window (tests). */
  windowMs?: number
}

export function useUndoableDelete() {
  const queryClient = useQueryClient()

  // Stable identity so callers can drop the trigger into memo dep arrays
  // (column defs, row-action maps) without re-creating them every render.
  return useCallback(
    function triggerUndoableDelete<TCache, TItem>(
      params: UndoableDeleteParams<TCache, TItem>
    ): void {
      const {
        item,
        queryKey,
        removeFromCache,
        deleteFn,
        title,
        undoLabel,
        errorTitle,
        alsoInvalidate = [],
        windowMs = UNDO_WINDOW_MS,
      } = params

      const snapshot = queryClient.getQueryData<TCache>(queryKey)
      queryClient.setQueryData<TCache>(queryKey, (current) =>
        current === undefined ? current : removeFromCache(current, item)
      )

      let settled = false

      const undo = (toastId: string | number) => {
        if (settled) return
        settled = true
        if (snapshot !== undefined) {
          queryClient.setQueryData(queryKey, snapshot)
        }
        toast.dismiss(toastId)
      }

      const commit = () => {
        if (settled) return
        settled = true
        deleteFn(item)
          .then(() => {
            void queryClient.invalidateQueries({ queryKey })
            for (const extra of alsoInvalidate) {
              void queryClient.invalidateQueries({ queryKey: extra })
            }
          })
          .catch((error: unknown) => {
            // Restore the row and say so — a silent resurrect would read as
            // a ghost; the error toast owns the explanation.
            if (snapshot !== undefined) {
              queryClient.setQueryData(queryKey, snapshot)
            }
            toast.error(
              typeof errorTitle === 'function' ? errorTitle(error) : errorTitle
            )
          })
      }

      const toastId = toast.message(title, {
        duration: windowMs,
        action: {
          label: undoLabel,
          onClick: () => undo(toastId),
        },
        onAutoClose: commit,
        // Swipe/× dismiss = accepting the delete (Gmail semantics).
        onDismiss: commit,
      })
    },
    [queryClient]
  )
}
