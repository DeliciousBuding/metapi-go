import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { Button } from '@/components/ui/button'
import { toast } from '@/lib/toast'

import {
  useApplySettingsMigration,
  useSettingsMigrationPreview,
} from '../../../lib/runtime-settings'

export function SchedulingMigrationCard() {
  const { t } = useTranslation()
  const previewQuery = useSettingsMigrationPreview()
  const applyMutation = useApplySettingsMigration()
  const [confirmOpen, setConfirmOpen] = useState(false)

  const preview = previewQuery.data
  if (previewQuery.isError) {
    // Old backend without the migration endpoint — degrade silently.
    return null
  }
  if (!preview || preview.pending === 0) {
    return null
  }

  return (
    <div className='border-primary/30 bg-primary/5 mb-6 space-y-3 rounded-lg border p-4'>
      <div>
        <h3 className='text-sm font-medium'>
          {t('settings.operations.scheduling.migration.title')}
        </h3>
        <p className='text-muted-foreground mt-1 text-xs'>
          {t('settings.operations.scheduling.migration.summary', {
            current: preview.currentVersion,
            target: preview.targetVersion,
            pending: preview.pending,
          })}
        </p>
        <ul className='text-muted-foreground mt-2 list-inside list-disc space-y-1 text-xs'>
          <li>
            {t('settings.operations.scheduling.migration.customCount', {
              count: preview.customCount,
            })}
          </li>
          <li>
            {t('settings.operations.scheduling.migration.legacyPreserved')}
          </li>
        </ul>
      </div>
      <Button
        type='button'
        size='sm'
        disabled={applyMutation.isPending}
        onClick={() => setConfirmOpen(true)}
      >
        {applyMutation.isPending
          ? t('settings.common.saving')
          : t('settings.operations.scheduling.migration.apply')}
      </Button>
      <ConfirmDialog
        open={confirmOpen}
        title={t('settings.operations.scheduling.migration.title')}
        description={t('settings.operations.scheduling.migration.confirm')}
        confirmLabel={t('settings.operations.scheduling.migration.apply')}
        cancelLabel={t('settings.common.cancel')}
        onConfirm={() => {
          setConfirmOpen(false)
          applyMutation.mutate(undefined, {
            onSuccess: () =>
              toast.success(
                t('settings.operations.scheduling.migration.applied')
              ),
            onError: () =>
              toast.error(
                t('settings.operations.scheduling.migration.applyFailed')
              ),
          })
        }}
        onCancel={() => setConfirmOpen(false)}
      />
    </div>
  )
}
