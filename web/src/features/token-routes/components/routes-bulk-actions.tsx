import type { Table } from '@tanstack/react-table'
import { Power } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { DataTableBulkActions } from '@/components/data-table'
import { Button } from '@/components/ui/button'

import { type BatchRouteAction, useBatchUpdateRoutes } from '../api'
import type { RouteSummaryRow } from '../types'

export function RoutesBulkActions({
  table,
}: {
  table: Table<RouteSummaryRow>
}) {
  const { t } = useTranslation()
  const batchMutation = useBatchUpdateRoutes()
  const [confirmDisableOpen, setConfirmDisableOpen] = useState(false)

  // Derived per render — `table` identity is stable across selection changes
  // then a `useMemo([table])` would freeze the ids at their mount-time value
  // and every batch action would silently no-op.
  const selectedIds = table
    .getFilteredSelectedRowModel()
    .rows.map((row) => (row.original as RouteSummaryRow).id)
    .filter((id) => id > 0)

  const runBatch = async (action: BatchRouteAction) => {
    if (selectedIds.length === 0) return
    try {
      await batchMutation.mutateAsync({ ids: selectedIds, action })
      table.resetRowSelection()
    } catch {
      // http-client toasted
    }
  }

  return (
    <>
      <DataTableBulkActions
        table={table}
        entityName={t('tokenRoutes.page.bulkEntityName')}
      >
        <Button
          size='sm'
          variant='outline'
          onClick={() => runBatch('enable')}
          disabled={batchMutation.isPending}
        >
          <Power />
          {t('tokenRoutes.page.bulkEnable')}
        </Button>
        <Button
          size='sm'
          variant='outline'
          onClick={() => setConfirmDisableOpen(true)}
          disabled={batchMutation.isPending}
        >
          {t('tokenRoutes.page.bulkDisable')}
        </Button>
      </DataTableBulkActions>

      <ConfirmDialog
        open={confirmDisableOpen}
        title={t('tokenRoutes.page.bulkDisableConfirmTitle')}
        description={t('tokenRoutes.page.bulkDisableConfirmDescription', {
          count: selectedIds.length,
        })}
        confirmLabel={t('tokenRoutes.page.bulkDisable')}
        cancelLabel={t('common.cancel')}
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
