// metapi-go/features/settings/sections/system-info/components — maintenance
// section. Cache rebuild, usage-log purge, and the irreversible factory-reset
// danger zone. The factory reset uses a 3-second countdown confirm dialog
// matching the legacy UX.

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { api } from '@/lib/api'

import { SettingsSectionCard } from '../../../components/settings-section-card'

const FACTORY_RESET_COUNTDOWN_SECONDS = 3

export function MaintenanceSection() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [factoryResetOpen, setFactoryResetOpen] = useState(false)
  const [countdown, setCountdown] = useState(FACTORY_RESET_COUNTDOWN_SECONDS)

  useEffect(() => {
    if (!factoryResetOpen) {
      setCountdown(FACTORY_RESET_COUNTDOWN_SECONDS)
      return
    }
    if (countdown <= 0) {
      return
    }
    const timer = window.setTimeout(() => {
      setCountdown((remaining) => remaining - 1)
    }, 1000)
    return () => window.clearTimeout(timer)
  }, [factoryResetOpen, countdown])

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

  const factoryResetMutation = useMutation({
    mutationFn: async () => api.factoryReset(),
    onSuccess: () => {
      toast.success(t('settings.systemInfo.maintenance.toast.factoryResetDone'))
      setFactoryResetOpen(false)
      // The backend wipes session + DB; reload to drop the in-memory auth state.
      window.location.reload()
    },
    onError: () => {
      toast.error(t('settings.systemInfo.maintenance.toast.factoryResetFailed'))
      setFactoryResetOpen(false)
    },
  })

  return (
    <SettingsSectionCard
      title={t('settings.systemInfo.maintenance.title')}
      description={t('settings.systemInfo.maintenance.description')}
    >
      <div className='space-y-6'>
        <div className='space-y-3 rounded-lg border p-4'>
          <h4 className='text-sm font-medium'>
            {t('settings.systemInfo.maintenance.toolsGroup')}
          </h4>
          <div className='flex flex-wrap gap-2'>
            <Button
              type='button'
              variant='outline'
              size='sm'
              disabled={clearCacheMutation.isPending}
              onClick={() => clearCacheMutation.mutate()}
            >
              {clearCacheMutation.isPending
                ? t('settings.common.saving')
                : t('settings.systemInfo.maintenance.clearCache')}
            </Button>
            <Button
              type='button'
              variant='outline'
              size='sm'
              disabled={clearUsageMutation.isPending}
              onClick={() => clearUsageMutation.mutate()}
            >
              {clearUsageMutation.isPending
                ? t('settings.common.saving')
                : t('settings.systemInfo.maintenance.clearUsage')}
            </Button>
          </div>
        </div>

        <div className='border-destructive/40 bg-destructive/5 space-y-3 rounded-lg border p-4'>
          <h4 className='text-destructive text-sm font-medium'>
            {t('settings.systemInfo.maintenance.dangerZone')}
          </h4>
          <p className='text-muted-foreground text-xs'>
            {t('settings.systemInfo.maintenance.factoryResetHint')}
          </p>
          <Button
            type='button'
            variant='destructive'
            size='sm'
            onClick={() => setFactoryResetOpen(true)}
          >
            {t('settings.systemInfo.maintenance.factoryReset')}
          </Button>
        </div>
      </div>

      <Dialog
        open={factoryResetOpen}
        onOpenChange={(open) => {
          if (!open) {
            setFactoryResetOpen(false)
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle className='text-destructive'>
              {t('settings.systemInfo.maintenance.factoryResetTitle')}
            </DialogTitle>
            <DialogDescription>
              {t('settings.systemInfo.maintenance.factoryResetDescription')}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant='outline'
              onClick={() => setFactoryResetOpen(false)}
            >
              {t('settings.common.cancel')}
            </Button>
            <Button
              variant='destructive'
              disabled={countdown > 0 || factoryResetMutation.isPending}
              onClick={() => factoryResetMutation.mutate()}
            >
              {countdown > 0
                ? t('settings.systemInfo.maintenance.factoryResetCountdown', {
                    seconds: countdown,
                  })
                : t('settings.systemInfo.maintenance.factoryResetConfirm')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </SettingsSectionCard>
  )
}
