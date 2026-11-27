import type { Table } from '@tanstack/react-table'
import { Trash2 as Trash2Icon } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { DataTableBulkActions } from '@/components/data-table'
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

import { useBatchUpdateSites } from '../api'
import type { Site, SiteBatchAction } from '../types'

type BulkConfirmation = { action: 'delete' | 'disable'; ids: number[] }

export function useSitesBulkActions(table: Table<Site>) {
  const { t } = useTranslation()
  const batchUpdateSites = useBatchUpdateSites()
  const [confirmation, setConfirmation] = useState<BulkConfirmation | null>(
    null
  )

  async function runAction(ids: number[], action: SiteBatchAction) {
    try {
      const result = await batchUpdateSites.mutateAsync({ ids, action })
      const successCount = result.successIds?.length ?? 0
      const failedCount = ids.length - successCount
      if (failedCount <= 0) {
        toast.success(t('sites.toast.bulkSucceeded', { count: successCount }))
      } else {
        toast.warning(
          t('sites.toast.bulkPartial', {
            success: successCount,
            failed: failedCount,
          })
        )
      }
      table.resetRowSelection()
    } catch {
      // http-client toasted
    }
  }

  function handleBulkAction(action: SiteBatchAction) {
    const ids = table
      .getFilteredSelectedRowModel()
      .rows.map((row) => row.original.id)
    if (!ids.length) return
    if (action === 'delete' || action === 'disable') {
      setConfirmation({ action, ids })
      return
    }
    void runAction(ids, action)
  }

  async function confirmAction() {
    if (!confirmation) return
    await runAction(confirmation.ids, confirmation.action)
    setConfirmation(null)
  }

  return {
    toolbar: (
      <DataTableBulkActions table={table} entityName={t('sites.entityName')}>
        <Button
          variant='outline'
          size='sm'
          onClick={() => handleBulkAction('enable')}
          disabled={batchUpdateSites.isPending}
        >
          {t('sites.bulk.enable')}
        </Button>
        <Button
          variant='outline'
          size='sm'
          onClick={() => handleBulkAction('disable')}
          disabled={batchUpdateSites.isPending}
        >
          {t('sites.bulk.disable')}
        </Button>
        <Button
          variant='outline'
          size='sm'
          onClick={() => handleBulkAction('enableSystemProxy')}
          disabled={batchUpdateSites.isPending}
        >
          {t('sites.bulk.enableSystemProxy')}
        </Button>
        <Button
          variant='outline'
          size='sm'
          onClick={() => handleBulkAction('disableSystemProxy')}
          disabled={batchUpdateSites.isPending}
        >
          {t('sites.bulk.disableSystemProxy')}
        </Button>
        <Button
          variant='destructive'
          size='sm'
          onClick={() => handleBulkAction('delete')}
          disabled={batchUpdateSites.isPending}
        >
          <Trash2Icon className='size-3.5' />
          {t('sites.bulk.delete')}
        </Button>
      </DataTableBulkActions>
    ),
    dialog: (['delete', 'disable'] as const).map((action) => (
      <Dialog
        key={action}
        open={confirmation?.action === action}
        onOpenChange={(open) => {
          if (!open && confirmation?.action === action) setConfirmation(null)
        }}
      >
        <DialogContent className='sm:max-w-sm'>
          <DialogHeader>
            <DialogTitle>{t(`sites.bulk.${action}ConfirmTitle`)}</DialogTitle>
            <DialogDescription>
              {t(`sites.bulk.${action}ConfirmDescription`, {
                count:
                  confirmation?.action === action ? confirmation.ids.length : 0,
              })}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant='outline'
              onClick={() => setConfirmation(null)}
              disabled={batchUpdateSites.isPending}
            >
              {t(`sites.bulk.${action}ConfirmCancel`)}
            </Button>
            <Button
              variant='destructive'
              onClick={confirmAction}
              disabled={batchUpdateSites.isPending}
            >
              {batchUpdateSites.isPending && <Spinner className='mr-2' />}
              {t(`sites.bulk.${action}ConfirmConfirm`)}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    )),
  }
}
