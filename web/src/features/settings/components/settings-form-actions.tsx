// metapi-go/features/settings/components — shared Save / Reset action row.
// Sections with a unified form render this instead of a bare submit button:
// Save is disabled when nothing changed, Reset restores the server baseline,
// and a "no unsaved changes" hint removes the always-on Save affordance.

import { CircleCheck, CircleDot, LoaderCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

type SettingsFormActionsProps = {
  /** form id the Save button submits (`form={...}`). */
  formId: string
  isDirty: boolean
  isPending?: boolean
  onReset: () => void
  saveLabel?: string
}

export function SettingsFormActions({
  formId,
  isDirty,
  isPending,
  onReset,
  saveLabel,
}: SettingsFormActionsProps) {
  const { t } = useTranslation()
  const label = saveLabel ?? t('settings.common.save')
  return (
    // Full-width row with two fixed slots (status left, actions right) so the
    // "saved / unsaved" text swap never shifts the Reset/Save buttons
    // (fixed placeholder copy, so the status text cannot shift the layout).
    // flex-1 + min-w-0, NOT min-w-full: inside a row that already holds a
    // sibling (e.g. the database section's "test connection" button),
    // min-w-full forced the row ~90px past the card edge and clipped Save.
    <div className='flex min-w-0 flex-1 flex-wrap items-center justify-between gap-3 border-t pt-4'>
      <span
        role='status'
        aria-live='polite'
        className={cn(
          'inline-flex items-center gap-2 text-xs leading-relaxed',
          isDirty ? 'text-warning-soft-fg' : 'text-muted-foreground'
        )}
      >
        {isPending ? (
          <LoaderCircle
            className='size-3.5 animate-spin motion-reduce:animate-none'
            aria-hidden='true'
          />
        ) : null}
        {!isPending && isDirty ? (
          <CircleDot className='size-3.5' aria-hidden='true' />
        ) : null}
        {!isPending && !isDirty ? (
          <CircleCheck className='size-3.5' aria-hidden='true' />
        ) : null}
        {isPending ? t('settings.common.saving') : null}
        {!isPending &&
          (isDirty ? t('settings.common.unsaved') : t('settings.common.saved'))}
      </span>
      <div className='ms-auto flex shrink-0 items-center gap-2'>
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
          {isPending ? t('settings.common.saving') : label}
        </Button>
      </div>
    </div>
  )
}
