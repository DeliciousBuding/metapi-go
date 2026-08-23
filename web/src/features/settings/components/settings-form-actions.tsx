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
    // Full-width row with two fixed slots (status left, actions right) so the
    // "saved / unsaved" text swap never shifts the Reset/Save buttons
    // (plan §3.4: 状态文案固定占位防跳动).
    <div className='flex min-w-full items-center justify-between gap-2'>
      <span className='text-muted-foreground text-xs whitespace-nowrap'>
        {isDirty ? t('settings.common.unsaved') : t('settings.common.saved')}
      </span>
      <div className='flex shrink-0 items-center gap-2'>
        <Button
          type='button'
          variant='outline'
          size='sm'
          disabled={!isDirty || isPending}
          onClick={onReset}
        >
          {t('settings.common.reset')}
        </Button>
        <Button
          type='submit'
          form={formId}
          size='sm'
          disabled={!isDirty || isPending}
        >
          {isPending ? pendingLabel : label}
        </Button>
      </div>
    </div>
  )
}
