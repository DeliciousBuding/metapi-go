import { useMutation, useQueryClient } from '@tanstack/react-query'
import { isAxiosError } from 'axios'
import { useCallback, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { api } from '@/lib/api'
import type {
  UpstreamDeletionPreview,
  UpstreamEntityKind,
} from '@/lib/api/upstream-lifecycle'
import { assertBusinessOk } from '@/lib/assert-business-ok'
import { toast } from '@/lib/toast'

import { UpstreamDeleteDialog } from './upstream-deletion-dialog'

export type UpstreamDeletionTarget = {
  kind: UpstreamEntityKind
  id: number
  name: string
}
type DeletionOperation = {
  target: UpstreamDeletionTarget
  preview: UpstreamDeletionPreview
  onDeleted?: () => void
}
type LeafHandler = (
  preview: UpstreamDeletionPreview,
  commit: () => Promise<unknown>
) => void
type DeletionSession = {
  target: UpstreamDeletionTarget
  onLeaf: LeafHandler
  onDeleted?: () => void
  preview?: UpstreamDeletionPreview
  error?: 'previewFailed' | 'deleteFailed' | 'changed'
  review: number
  reviewRequired?: boolean
}

function revisedPreview(error: unknown, target: UpstreamDeletionTarget) {
  if (
    !isAxiosError<{ preview?: UpstreamDeletionPreview }>(error) ||
    error.response?.status !== 409
  ) {
    return undefined
  }
  const preview = error.response.data.preview
  if (
    preview?.kind !== target.kind ||
    preview.id !== target.id ||
    !preview.revision ||
    typeof preview.requiresCascade !== 'boolean' ||
    !preview.counts ||
    !Array.isArray(preview.affectedRouteIds)
  ) {
    return undefined
  }
  return preview
}

/** Preview first. Leaf callers retain their undo UI by scheduling commit;
 * that same commit reopens review if the closure changes during the undo window. */
export function useUpstreamDeletion() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [session, setSession] = useState<DeletionSession | null>(null)
  const active = useRef(false)
  const deleting = useRef(false)
  const { mutateAsync: previewDeletion, isPending: previewPending } =
    useMutation({
      mutationFn: (target: UpstreamDeletionTarget) =>
        api.previewUpstreamDeletion(target.kind, target.id),
      retry: false,
    })
  const { mutateAsync: deleteEntity, isPending: deletePending } = useMutation({
    mutationFn: (operation: DeletionOperation) =>
      api.deleteUpstreamEntity(operation.target.kind, operation.target.id, {
        cascade: operation.preview.requiresCascade,
        expectedRevision: operation.preview.revision,
      }),
    retry: false,
  })

  const commit = useCallback(
    async (operation: DeletionOperation, fromDialog: boolean) => {
      if (deleting.current) {
        throw new Error(t('common.upstreamDeletion.pending'))
      }
      deleting.current = true
      try {
        const result = await deleteEntity(operation)
        assertBusinessOk(result, 'common.upstreamDeletion.deleteFailed')
        for (const key of [
          'imported-upstreams',
          'routes',
          'channels',
          'downstream-keys',
        ]) {
          void queryClient.invalidateQueries({ queryKey: [key] })
        }
        if (fromDialog) {
          setSession(null)
          active.current = false
          toast.success(
            t('common.upstreamDeletion.deleted', {
              name: operation.target.name,
            })
          )
        }
        operation.onDeleted?.()
        return result
      } catch (error) {
        const preview = revisedPreview(error, operation.target)
        const conflict = isAxiosError(error) && error.response?.status === 409
        if (fromDialog || conflict) {
          active.current = true
          setSession((current) => ({
            target: operation.target,
            onLeaf: () => {},
            onDeleted: operation.onDeleted,
            preview: conflict ? preview : operation.preview,
            error: preview ? 'changed' : 'deleteFailed',
            review: (current?.review ?? 0) + (conflict ? 1 : 0),
            reviewRequired: true,
          }))
        }
        throw error
      } finally {
        deleting.current = false
      }
    },
    [deleteEntity, queryClient, t]
  )

  const loadPreview = useCallback(
    async (next: DeletionSession, allowLeaf: boolean) => {
      setSession({ ...next, preview: undefined, error: undefined })
      try {
        const preview = await previewDeletion(next.target)
        if (!preview.requiresCascade && allowLeaf) {
          active.current = false
          setSession(null)
          next.onLeaf(preview, () =>
            commit(
              { target: next.target, preview, onDeleted: next.onDeleted },
              false
            )
          )
        } else {
          setSession({
            ...next,
            preview,
            error: undefined,
            review: next.review + 1,
          })
        }
      } catch {
        setSession({ ...next, preview: undefined, error: 'previewFailed' })
      }
    },
    [commit, previewDeletion]
  )

  const requestDeletion = useCallback(
    async (
      target: UpstreamDeletionTarget,
      onLeaf: LeafHandler,
      onDeleted?: () => void
    ): Promise<void> => {
      if (active.current || deleting.current) return
      active.current = true
      await loadPreview({ target, onLeaf, onDeleted, review: 0 }, true)
    },
    [loadPreview]
  )

  const isPending = previewPending || deletePending
  const dialog = session ? (
    <UpstreamDeleteDialog
      target={session.target}
      preview={session.preview}
      error={session.error}
      review={session.review}
      isPending={isPending}
      onCancel={() => {
        if (isPending) return
        active.current = false
        setSession(null)
      }}
      onRetry={() => {
        void loadPreview(session, !session.reviewRequired)
      }}
      onConfirm={() => {
        if (!session.preview || isPending) return
        void commit(
          {
            target: session.target,
            preview: session.preview,
            onDeleted: session.onDeleted,
          },
          true
        ).catch(() => {})
      }}
    />
  ) : null

  return { requestDeletion, dialog, isPending }
}
