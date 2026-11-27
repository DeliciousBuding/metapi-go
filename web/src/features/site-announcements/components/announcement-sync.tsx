import { useMutation, useQueryClient } from '@tanstack/react-query'
import { RefreshCw } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { api } from '@/lib/api'
import { toast } from '@/lib/toast'

import { useSiteAnnouncementSyncTask } from '../api'
import {
  siteAnnouncementsKeys,
  TERMINAL_SYNC_TASK_STATUSES,
  type SiteAnnouncementSyncTask,
} from '../types'

/** i18n key for the inline sync progress chip fallback label. */
function syncStatusKey(syncTask: SiteAnnouncementSyncTask | undefined): string {
  if (syncTask && syncTask.status === 'running') {
    return 'siteAnnouncements.sync.running'
  }
  return 'siteAnnouncements.sync.pending'
}

export function useAnnouncementSync(siteId: number | null) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [syncTaskId, setSyncTaskId] = useState<string | null>(null)
  const syncTaskQuery = useSiteAnnouncementSyncTask(syncTaskId)
  const syncTask = syncTaskQuery.data
  // Active from the moment the task id is known until the polled task reaches
  // a terminal status — covers the window between the POST response and the
  // first poll result where `syncTask` is still undefined.
  const syncIsActive =
    syncTaskId !== null &&
    !syncTaskQuery.isError &&
    (syncTask === undefined ||
      !TERMINAL_SYNC_TASK_STATUSES.has(syncTask.status))

  const handledTerminalRef = useRef<string | null>(null)
  useEffect(() => {
    if (!syncTask || !TERMINAL_SYNC_TASK_STATUSES.has(syncTask.status)) return
    const terminalKey = `${syncTask.id}:${syncTask.status}`
    if (handledTerminalRef.current === terminalKey) return
    handledTerminalRef.current = terminalKey
    void queryClient.invalidateQueries({ queryKey: siteAnnouncementsKeys.all })
    if (syncTask.status === 'succeeded') {
      const result = syncTask.result
      if (result && result.failed > 0) {
        toast.warning(
          t('siteAnnouncements.sync.successWithFailures', {
            inserted: result.inserted,
            updated: result.updated,
            failed: result.failed,
          })
        )
      } else {
        toast.success(
          t('siteAnnouncements.sync.success', {
            inserted: result?.inserted ?? 0,
            updated: result?.updated ?? 0,
          })
        )
      }
    } else {
      toast.error(syncTask.error || t('siteAnnouncements.sync.failed'))
    }
  }, [syncTask, queryClient, t])

  const syncMutation = useMutation({
    mutationFn: async () =>
      api.syncSiteAnnouncements(siteId !== null ? { siteId } : undefined),
    onSuccess: (response) => {
      handledTerminalRef.current = null
      setSyncTaskId(response.taskId)
    },
    onError: () => toast.error(t('siteAnnouncements.sync.startFailed')),
  })
  return {
    start: syncMutation.mutate,
    busy: syncMutation.isPending || syncIsActive,
    status: syncIsActive
      ? syncTask?.message || t(syncStatusKey(syncTask))
      : null,
  }
}

export function SyncButton({
  sync,
  className,
}: {
  sync: ReturnType<typeof useAnnouncementSync>
  className?: string
}) {
  const { t } = useTranslation()
  return (
    <Button
      type='button'
      className={className}
      variant='outline'
      size='sm'
      disabled={sync.busy}
      onClick={() => sync.start()}
    >
      {sync.busy ? <Spinner /> : <RefreshCw className='size-3.5' />}
      {t('siteAnnouncements.actions.sync')}
    </Button>
  )
}
