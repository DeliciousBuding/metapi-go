import { zodResolver } from '@hookform/resolvers/zod'
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { Button } from '@/components/ui/button'
import { Form } from '@/components/ui/form'
import { gatewayUsersApi, type GroupPricing } from '@/lib/api/gateway-users'

import { useAdminWrite } from './api'
import { amountInput, TextField, type EditorState } from './editor-fields'
import { WriteError } from './feedback'
import { multiplierPreview } from './quota-preview'

export function PricingEditor({
  groupId,
  pricing,
  onStateChange,
}: {
  groupId: number
  pricing: GroupPricing
  onStateChange?: (state: EditorState) => void
}) {
  const { t } = useTranslation()
  const schema = z.object({
    multiplier: amountInput(t('userAdmin.multiplierHint'), true),
  })
  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: { multiplier: String(pricing.multiplier) },
    values: { multiplier: String(pricing.multiplier) },
    resetOptions: { keepDirtyValues: true },
  })
  const [saved, setSaved] = useState(false)
  const mutation = useAdminWrite(
    (value: z.infer<typeof schema>, token) =>
      gatewayUsersApi.setPricing(
        groupId,
        { multiplier: Number(value.multiplier) },
        token
      ),
    () => {
      setSaved(true)
      form.reset(form.getValues())
    }
  )
  const dirty = form.formState.isDirty
  const previewMultiplier = form.watch('multiplier')
  const example = multiplierPreview(previewMultiplier)
  useEffect(() => {
    onStateChange?.({ dirty, pending: mutation.isPending })
  }, [dirty, mutation.isPending, onStateChange])
  return (
    <Form {...form}>
      <form
        onSubmit={form.handleSubmit((value) => {
          setSaved(false)
          mutation.submit(value)
        })}
        className='space-y-3'
      >
        <TextField
          control={form.control}
          name='multiplier'
          label={t('userAdmin.multiplier')}
          type='number'
        />
        <p className='text-muted-foreground text-xs'>
          {t('userAdmin.pricingHint')}
        </p>
        <section
          className='bg-muted/50 space-y-2 rounded-lg border p-3'
          aria-live='polite'
        >
          <h3 className='text-sm font-medium'>
            {t('userAdmin.budgetPreview.multiplierTitle')}
          </h3>
          <p className='text-sm wrap-anywhere tabular-nums'>
            {example
              ? t('userAdmin.budgetPreview.multiplierExample', {
                  multiplier: previewMultiplier,
                  ...example,
                })
              : t('userAdmin.budgetPreview.invalidMultiplier')}
          </p>
          <p className='text-muted-foreground text-xs'>
            {t('userAdmin.budgetPreview.rounding')}
          </p>
        </section>
        <WriteError error={mutation.error} />
        <div className='flex items-center gap-3'>
          <Button type='submit' size='sm' disabled={mutation.isPending}>
            {t(mutation.isPending ? 'userAdmin.saving' : 'userAdmin.save')}
          </Button>
          {saved && <span role='status'>{t('userAdmin.saved')}</span>}
        </div>
      </form>
    </Form>
  )
}
