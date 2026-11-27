import type { Table } from '@tanstack/react-table'
import { Power, RefreshCw, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/common/confirm-dialog'
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

import { type BatchAccountAction, useBatchUpdateAccounts } from '../api'
import type { Account } from '../types'

export function AccountsBulkActions({ table }: { table: Table<Account> }) {
  const { t } = useTranslation()
  const batchMutation = useBatchUpdateAccounts()
  const [confirmDeleteOpen, setConfirmDeleteOpen] = useState(false)
  const [confirmDisableOpen, setConfirmDisableOpen] = useState(false)

  // Derived per render — `table` identity is stable across selection changes
  // (TanStack `useReactTable` memoizes the instance), so a `useMemo([table])`
  // here would freeze the ids at their mount-time (empty) value and every
  // batch action would silently no-op.
  const selectedIds = table
    .getFilteredSelectedRowModel()
    .rows.map((row) => row.original.id)

  const runBatch = async (action: BatchAccountAction) => {
    if (selectedIds.length === 0) return
    try {
      const result = await batchMutation.mutateAsync({
        ids: selectedIds,
        action,
      })
      // Partial failures are toasted by the mutation hook (failedItems);
      // only report success here to avoid double toasts.
      if ((result?.failedItems ?? []).length === 0) {
        toast.success(
          t('accounts.toast.bulkSucceeded', {
            count: result?.successIds?.length ?? selectedIds.length,
          })
        )
      }
      table.resetRowSelection()
    } catch {
      // http-client toasted
    }
  }

  const confirmBulkDelete = async () => {
    if (selectedIds.length === 0) return
    try {
      const result = await batchMutation.mutateAsync({
        ids: selectedIds,
        action: 'delete',
      })
      if ((result?.failedItems ?? []).length === 0) {
        toast.success(
          t('accounts.toast.bulkSucceeded', {
            count: result?.successIds?.length ?? selectedIds.length,
          })
        )
      }
      table.resetRowSelection()
      setConfirmDeleteOpen(false)
    } catch {
      // http-client toasted
      setConfirmDeleteOpen(false)
    }
  }

  return (
    <>
      <DataTableBulkActions
        table={table}
        entityName={t('accounts.bulk.entityName')}
      >
        <Button
          size='sm'
          variant='outline'
          onClick={() => runBatch('refreshBalance')}
          disabled={batchMutation.isPending}
        >
          <RefreshCw />
          {t('accounts.bulk.refreshBalance')}
        </Button>
        <Button
          size='sm'
          variant='outline'
          onClick={() => runBatch('enable')}
          disabled={batchMutation.isPending}
        >
          <Power />
          {t('accounts.bulk.enable')}
        </Button>
        <Button
          size='sm'
          variant='outline'
          onClick={() => setConfirmDisableOpen(true)}
          disabled={batchMutation.isPending}
        >
          {t('accounts.bulk.disable')}
        </Button>
        <Button
          size='sm'
          variant='destructive'
          onClick={() => setConfirmDeleteOpen(true)}
          disabled={batchMutation.isPending}
        >
          <Trash2 />
          {t('accounts.bulk.delete')}
        </Button>
      </DataTableBulkActions>

      <Dialog open={confirmDeleteOpen} onOpenChange={setConfirmDeleteOpen}>
        <DialogContent className='sm:max-w-sm'>
          <DialogHeader>
            <DialogTitle>{t('accounts.bulk.deleteConfirmTitle')}</DialogTitle>
            <DialogDescription>
              {t('accounts.bulk.deleteConfirmDescription', {
                count: selectedIds.length,
              })}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant='outline'
              onClick={() => setConfirmDeleteOpen(false)}
              disabled={batchMutation.isPending}
            >
              {t('accounts.bulk.deleteConfirmCancel')}
            </Button>
            <Button
              variant='destructive'
              onClick={confirmBulkDelete}
              disabled={batchMutation.isPending}
            >
              {batchMutation.isPending && <Spinner />}
              {t('accounts.bulk.deleteConfirmConfirm')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={confirmDisableOpen}
        title={t('accounts.bulk.disableConfirmTitle')}
        description={t('accounts.bulk.disableConfirmDescription', {
          count: selectedIds.length,
        })}
        confirmLabel={t('accounts.bulk.disableConfirmConfirm')}
        cancelLabel={t('accounts.bulk.deleteConfirmCancel')}
        destructive
        onConfirm={() => {
          setConfirmDisableOpen(false)
          void runBatch('disable')
        }}
        onCancel={() => setConfirmDisableOpen(false)}
      />
    </>
  )
}
