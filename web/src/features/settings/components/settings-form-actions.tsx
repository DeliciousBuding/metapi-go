// metapi-go/features/settings/components — shared Save / Reset action row.
// Sections with a unified form render this instead of a bare submit button:
// Save is disabled when nothing changed, Reset restores the server baseline,
// and a "no unsaved changes" hint removes the always-on Save affordance.

import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

type SettingsFormActionsProps = {
  /** form id the Save button submits (`form={...}`). */
  formId: string
  isDirty: boolean
  isPending?: boolean
  onReset: () => void
  saveLabel?: string
  savingLabel?: string
}

export function SettingsFormActions({
  formId,
  isDirty,
  isPending,
  onReset,
  saveLabel,
  savingLabel,
}: SettingsFormActionsProps) {
  const { t } = useTranslation()
  const label = saveLabel ?? t('settings.common.save')
  const pendingLabel = savingLabel ?? t('settings.common.saving')
  return (
    <div className='flex items-center gap-2'>
      {!isDirty ? (
        <span className='text-xs text-muted-foreground'>
          {t('settings.common.saved')}
        </span>
      ) : null}
      <Button
        type='button'
        variant='outline'
        size='sm'
        disabled={!isDirty || isPending}
        onClick={onReset}
      >
        {t('settings.common.reset')}
      </Button>
      <Button type='submit' form={formId} size='sm' disabled={!isDirty || isPending}>
        {isPending ? pendingLabel : label}
      </Button>
    </div>
  )
}
