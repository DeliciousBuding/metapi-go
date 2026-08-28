// metapi-go/features/settings/sections/operations/components — danger zone
// section. Irreversible factory reset behind a 3-second countdown confirm
// dialog matching the legacy UX. Kept as its own section so the destructive
// action is never nested inside routine-tools pages.

import { useMutation } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
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
import { Input } from '@/components/ui/input'
import { api } from '@/lib/api'
import { toast } from '@/lib/toast'

import { SettingsSectionCard } from '../../../components/settings-section-card'

const FACTORY_RESET_COUNTDOWN_SECONDS = 3
// Type-to-confirm word: the countdown stops misclicks but not "read it yet
// didn't absorb it"; requiring the operator to type the word proves attention
// (W19-T3 N2 / T1 §0.2, GitHub-style hard gate).
const FACTORY_RESET_CONFIRM_WORD = 'RESET'

export function DangerZoneSection() {
  const { t } = useTranslation()
  const [factoryResetOpen, setFactoryResetOpen] = useState(false)
  const [countdown, setCountdown] = useState(FACTORY_RESET_COUNTDOWN_SECONDS)
  const [confirmText, setConfirmText] = useState('')

  useEffect(() => {
    if (!factoryResetOpen) {
      setCountdown(FACTORY_RESET_COUNTDOWN_SECONDS)
      setConfirmText('')
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
      toast.success(t('settings.operations.dangerZone.toast.factoryResetDone'))
      setFactoryResetOpen(false)
      // The backend wipes session + DB; reload to drop the in-memory auth state.
      window.location.reload()
    },
    onError: () => {
      toast.error(t('settings.operations.dangerZone.toast.factoryResetFailed'))
      setFactoryResetOpen(false)
    },
  })

  return (
    <SettingsSectionCard
      title={t('settings.operations.dangerZone.title')}
      description={t('settings.operations.dangerZone.description')}
    >
      <div className='border-destructive/40 bg-destructive/5 space-y-3 rounded-lg border p-4'>
        <Button
          type='button'
          variant='destructive'
          size='sm'
          onClick={() => setFactoryResetOpen(true)}
        >
          {t('settings.operations.dangerZone.factoryReset')}
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
              {t('settings.operations.dangerZone.factoryResetTitle')}
            </DialogTitle>
            <DialogDescription>
              {t('settings.operations.dangerZone.factoryResetDescription')}
            </DialogDescription>
          </DialogHeader>

          <div className='grid gap-2'>
            <label
              htmlFor='factory-reset-confirm-word'
              className='text-muted-foreground text-sm'
            >
              {t('settings.operations.dangerZone.factoryResetTypeHint', {
                word: FACTORY_RESET_CONFIRM_WORD,
              })}
            </label>
            <Input
              id='factory-reset-confirm-word'
              value={confirmText}
              onChange={(event) => setConfirmText(event.target.value)}
              placeholder={FACTORY_RESET_CONFIRM_WORD}
              autoComplete='off'
              spellCheck={false}
            />
          </div>

          <DialogFooter>
            <Button
              variant='outline'
              onClick={() => setFactoryResetOpen(false)}
            >
              {t('settings.common.cancel')}
            </Button>
            <Button
              variant='destructive'
              disabled={
                countdown > 0 ||
                factoryResetMutation.isPending ||
                confirmText.trim() !== FACTORY_RESET_CONFIRM_WORD
              }
              onClick={() => factoryResetMutation.mutate()}
            >
              {countdown > 0
                ? t('settings.operations.dangerZone.factoryResetCountdown', {
                    seconds: countdown,
                  })
                : t('settings.operations.dangerZone.factoryResetConfirm')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </SettingsSectionCard>
  )
}
