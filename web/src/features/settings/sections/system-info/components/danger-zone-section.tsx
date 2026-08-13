// metapi-go/features/settings/sections/system-info/components — danger zone
// section. Irreversible factory reset behind a 3-second countdown confirm
// dialog matching the legacy UX. Kept as its own section so the destructive
// action is never nested inside routine-tools pages.

import { useMutation } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from '@/lib/toast'

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

export function DangerZoneSection() {
  const { t } = useTranslation()
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

  const factoryResetMutation = useMutation({
    mutationFn: async () => api.factoryReset(),
    onSuccess: () => {
      toast.success(t('settings.systemInfo.dangerZone.toast.factoryResetDone'))
      setFactoryResetOpen(false)
      // The backend wipes session + DB; reload to drop the in-memory auth state.
      window.location.reload()
    },
    onError: () => {
      toast.error(t('settings.systemInfo.dangerZone.toast.factoryResetFailed'))
      setFactoryResetOpen(false)
    },
  })

  return (
    <SettingsSectionCard
      title={t('settings.systemInfo.dangerZone.title')}
      description={t('settings.systemInfo.dangerZone.description')}
    >
      <div className='border-destructive/40 bg-destructive/5 space-y-3 rounded-lg border p-4'>
        <Button
          type='button'
          variant='destructive'
          size='sm'
          onClick={() => setFactoryResetOpen(true)}
        >
          {t('settings.systemInfo.dangerZone.factoryReset')}
        </Button>
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
              {t('settings.systemInfo.dangerZone.factoryResetTitle')}
            </DialogTitle>
            <DialogDescription>
              {t('settings.systemInfo.dangerZone.factoryResetDescription')}
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
                ? t('settings.systemInfo.dangerZone.factoryResetCountdown', {
                    seconds: countdown,
                  })
                : t('settings.systemInfo.dangerZone.factoryResetConfirm')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </SettingsSectionCard>
  )
}
