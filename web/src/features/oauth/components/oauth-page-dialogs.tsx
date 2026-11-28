import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Spinner } from '@/components/ui/spinner'
import { toast } from '@/lib/toast'

import { useDeleteOAuthConnection } from '../api'
import type { OAuthClient } from '../types'
import { OAuthDetailSheet } from './oauth-detail-sheet'
import { OAuthStartDialog } from './oauth-start-dialog'

export function useOAuthDialogs(connections: OAuthClient[]) {
  const [startOpen, setStartOpen] = useState(false)
  const [deletingClient, setDeletingClient] = useState<OAuthClient | null>(null)
  // The detail sheet tracks the account id rather than a row snapshot: the
  // sheet's own refresh-quota action invalidates the connections query, and a
  // frozen snapshot would keep showing the pre-refresh quota. Re-deriving
  // from the current page makes the refreshed numbers appear in place.
  const [detailAccountId, setDetailAccountId] = useState<number | null>(null)
  const [detailOpen, setDetailOpen] = useState(false)
  const detailConnection =
    connections.find(
      (connection) => connection.accountId === detailAccountId
    ) ?? null

  return {
    startOpen,
    setStartOpen,
    openStart: () => setStartOpen(true),
    detailConnection,
    detailOpen,
    openDetails: (client: OAuthClient) => {
      setDetailAccountId(client.accountId)
      setDetailOpen(true)
    },
    onDetailOpenChange: (open: boolean) => {
      setDetailOpen(open)
      if (!open) setDetailAccountId(null)
    },
    deletingClient,
    setDeletingClient,
  }
}

export function OAuthPageDialogs({
  dialogs,
  handleRefreshQuota,
  handleRebind,
  isRefreshingQuota,
  isRebinding,
}: {
  dialogs: ReturnType<typeof useOAuthDialogs>
  handleRefreshQuota: (client: OAuthClient) => void
  handleRebind: (client: OAuthClient) => void
  isRefreshingQuota: boolean
  isRebinding: boolean
}) {
  const {
    startOpen,
    setStartOpen,
    detailConnection,
    detailOpen,
    onDetailOpenChange,
    deletingClient,
    setDeletingClient,
  } = dialogs
  const { t } = useTranslation()
  const deleteConnection = useDeleteOAuthConnection()
  async function confirmDelete() {
    if (!deletingClient) return
    const client = deletingClient
    try {
      await deleteConnection.mutateAsync(client.accountId)
      toast.success(t('oauth.toast.deleted', { id: client.accountId }))
    } catch {
      // http-client toasted
    } finally {
      setDeletingClient(null)
    }
  }
  return (
    <>
      <OAuthStartDialog open={startOpen} onOpenChange={setStartOpen} />

      <OAuthDetailSheet
        connection={detailConnection}
        // A row that leaves the current page (deleted, or the operator paged
        // away) closes its own sheet instead of leaving an empty panel open.
        open={detailOpen && detailConnection !== null}
        onOpenChange={onDetailOpenChange}
        onRefreshQuota={handleRefreshQuota}
        onRebind={handleRebind}
        isRefreshingQuota={isRefreshingQuota}
        isRebinding={isRebinding}
      />

      <Dialog
        open={deletingClient !== null}
        onOpenChange={(open) => {
          if (!open) setDeletingClient(null)
        }}
      >
        <DialogContent className='sm:max-w-md'>
          <DialogHeader>
            <DialogTitle>{t('oauth.delete.title')}</DialogTitle>
            <DialogDescription>
              {t('oauth.delete.description', {
                provider: deletingClient?.provider ?? '',
                id: deletingClient?.accountId ?? 0,
              })}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant='outline'
              onClick={() => setDeletingClient(null)}
              disabled={deleteConnection.isPending}
            >
              {t('oauth.delete.cancel')}
            </Button>
            <Button
              variant='destructive'
              onClick={confirmDelete}
              disabled={deleteConnection.isPending}
            >
              {deleteConnection.isPending && <Spinner />}
              {t('oauth.delete.confirm')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
