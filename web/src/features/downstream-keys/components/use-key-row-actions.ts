import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useCallback } from 'react'
import { useTranslation } from 'react-i18next'

import { useReauthReplay } from '@/components/common/reauth-provider'
import { api } from '@/lib/api'
import { toast } from '@/lib/toast'
import { useUndoableDelete } from '@/lib/undoable-delete'

import {
  downstreamKeysQueryKeys,
  type DownstreamApiKeyItem,
  type DownstreamKeysResponse,
} from '../types'

export function useKeyRowActions() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const handleReauth = useReauthReplay()
  const toggleMutation = useMutation({
    mutationFn: async (input: {
      id: number
      enabled: boolean
      confirmToken?: string
    }) =>
      input.confirmToken
        ? api.updateDownstreamApiKey(
            input.id,
            { enabled: input.enabled },
            input.confirmToken
          )
        : api.updateDownstreamApiKey(input.id, { enabled: input.enabled }),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: downstreamKeysQueryKeys.all,
      })
      toast.success(t('settings.downstream.keys.toast.updated'))
    },
    onError: (error, input) => {
      // Enable/disable rides the same sensitive PUT; a reauth demand must not
      // be reported as an ordinary update failure.
      if (
        handleReauth(
          error,
          t('reauth.operations.downstreamKey'),
          (confirmToken) =>
            toggleMutation.mutateAsync({ ...input, confirmToken })
        )
      ) {
        return
      }
      toast.error(t('settings.downstream.keys.toast.updateFailed'))
    },
  })

  // Delete-with-undo tier: key revoke is leaf (no cascade) — no confirm dialog;
  // the row leaves immediately and a 6s undo toast gates the real DELETE.
  const undoableDelete = useUndoableDelete()
  const deleteKey = useCallback(
    (item: DownstreamApiKeyItem) =>
      undoableDelete<DownstreamKeysResponse, DownstreamApiKeyItem>({
        item,
        queryKey: downstreamKeysQueryKeys.list(),
        removeFromCache: (data, target) => ({
          ...data,
          items: data.items.filter((entry) => entry.id !== target.id),
        }),
        deleteFn: (target) => api.deleteDownstreamApiKey(target.id),
        title: t('settings.downstream.keys.toast.deleted'),
        undoLabel: t('common.undo'),
        errorTitle: t('settings.downstream.keys.toast.deleteFailed'),
      }),
    [undoableDelete, t]
  )
  return {
    toggleKeyMutate: toggleMutation.mutate,
    toggleKeyPending: toggleMutation.isPending,
    deleteKey,
  }
}
