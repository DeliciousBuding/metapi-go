import { useQueryClient } from '@tanstack/react-query'
import { useCallback, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { channelsKeys } from '@/features/channels'
import { tokenRoutesApi } from '@/lib/api/token-routes'
import { toast } from '@/lib/toast'

import type { BatchProbeResult } from '../types'

export function useDisableFailedChannels(
  comparison: BatchProbeResult[] | null
) {
  const { t } = useTranslation()
  const [disableFailedDialogOpen, setDisableFailedDialogOpen] = useState(false)
  const [isDisablingFailed, setIsDisablingFailed] = useState(false)
  const queryClient = useQueryClient()

  // Channels whose comparison probe failed — the bulk disable action
  // targets exactly these rows.
  const failedChannelIds = useMemo(
    () =>
      (comparison ?? [])
        .filter((result) => result.status === 'failure')
        .map((result) => result.channelId),
    [comparison]
  )

  // One-click closure for failed comparison rows: after the operator
  // confirms, PUT /api/channels/batch disables exactly the failed channels
  // (marked manual_override server-side so rebuilds keep it). The envelope
  // reports per-item truth; a partial failure surfaces the precise
  // breakdown instead of a generic toast. AuditMiddleware records the call.
  const handleDisableFailedConfirmed = useCallback(async () => {
    setDisableFailedDialogOpen(false)
    if (failedChannelIds.length === 0 || isDisablingFailed) return
    setIsDisablingFailed(true)
    try {
      const result = await tokenRoutesApi.batchUpdateChannels(
        failedChannelIds.map((id) => ({ id, enabled: false }))
      )
      const succeededCount = result.successIds?.length ?? 0
      const failedItems = result.failedItems ?? []
      if (failedItems.length === 0) {
        toast.success(
          t('modelTester.disableFailed.success', { count: succeededCount })
        )
      } else {
        const detail = failedItems
          .map((item) => `#${item.id}: ${item.message}`)
          .join('; ')
        toast.warning(
          t('modelTester.disableFailed.partial', {
            succeeded: succeededCount,
            failed: failedItems.length,
            detail,
          })
        )
      }
      if (succeededCount > 0) {
        queryClient.invalidateQueries({ queryKey: channelsKeys.list() })
      }
    } catch {
      toast.error(t('modelTester.disableFailed.error'))
    } finally {
      setIsDisablingFailed(false)
    }
  }, [failedChannelIds, isDisablingFailed, queryClient, t])

  return {
    open: disableFailedDialogOpen,
    setOpen: setDisableFailedDialogOpen,
    isPending: isDisablingFailed,
    count: failedChannelIds.length,
    confirm: handleDisableFailedConfirmed,
  }
}
