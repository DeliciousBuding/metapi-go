// metapi-go/features/settings/sections/system-info/components — maintenance
// section. Cache rebuild + usage-log purge. The irreversible factory reset
// lives in the separate danger-zone section so destructive actions are never
// nested inside routine-tools pages.

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Link, type LinkProps } from '@tanstack/react-router'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { Button } from '@/components/ui/button'
import { api } from '@/lib/api'
import { toast } from '@/lib/toast'

import { SettingsSectionCard } from '../../../components/settings-section-card'

export function MaintenanceSection() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [confirmClearUsageOpen, setConfirmClearUsageOpen] = useState(false)

  const clearCacheMutation = useMutation({
    mutationFn: async () => api.clearRuntimeCache(),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['runtime-settings'] })
      toast.success(t('settings.systemInfo.maintenance.toast.cacheCleared'))
    },
    onError: () =>
      toast.error(t('settings.systemInfo.maintenance.toast.cacheClearFailed')),
  })

  const clearUsageMutation = useMutation({
    mutationFn: async () => api.clearUsageData(),
    onSuccess: () =>
      toast.success(t('settings.systemInfo.maintenance.toast.usageCleared')),
    onError: () =>
      toast.error(t('settings.systemInfo.maintenance.toast.usageClearFailed')),
  })

  return (
    <SettingsSectionCard
      title={t('settings.systemInfo.maintenance.title')}
      description={t('settings.systemInfo.maintenance.description')}
    >
      <div className='space-y-3 rounded-lg border p-4'>
        <h3 className='text-sm font-medium'>
          {t('settings.systemInfo.maintenance.toolsGroup')}
        </h3>
        <div className='flex flex-wrap gap-2'>
          <Button
            type='button'
            variant='outline'
            size='sm'
            disabled={clearCacheMutation.isPending}
            onClick={() => clearCacheMutation.mutate()}
          >
            {clearCacheMutation.isPending
              ? t('settings.systemInfo.maintenance.clearing')
              : t('settings.systemInfo.maintenance.clearCache')}
          </Button>
          <Button
            type='button'
            variant='outline'
            size='sm'
            disabled={clearUsageMutation.isPending}
            onClick={() => setConfirmClearUsageOpen(true)}
          >
            {clearUsageMutation.isPending
              ? t('settings.systemInfo.maintenance.clearing')
              : t('settings.systemInfo.maintenance.clearUsage')}
          </Button>
        </div>
        <p className='text-muted-foreground text-xs'>
          {t('settings.systemInfo.maintenance.eventsHint')}{' '}
          <Link
            to={
              '/settings/system-info/program-logs' as
                | LinkProps['to']
                | (string & {})
            }
            // WCAG 2.5.8 best-effort (F-line residual E): vertical click
            // padding on an inline element widens the hit area to ~24px
            // without growing the line box, so the hint layout is untouched.
            className='text-primary py-1 underline-offset-4 hover:underline'
          >
            {t('settings.systemInfo.maintenance.viewEvents')}
          </Link>
        </p>
      </div>

      <ConfirmDialog
        open={confirmClearUsageOpen}
        title={t('settings.systemInfo.maintenance.clearUsageConfirmTitle')}
        description={t(
          'settings.systemInfo.maintenance.clearUsageConfirmDescription'
        )}
        confirmLabel={t('settings.systemInfo.maintenance.clearUsage')}
        cancelLabel={t('settings.common.cancel')}
        destructive
        onConfirm={() => {
          setConfirmClearUsageOpen(false)
          clearUsageMutation.mutate()
        }}
        onCancel={() => setConfirmClearUsageOpen(false)}
      />
    </SettingsSectionCard>
  )
}
