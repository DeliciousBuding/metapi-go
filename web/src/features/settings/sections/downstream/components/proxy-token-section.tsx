// metapi-go/features/settings/sections/downstream/components — proxy-token
// section. The global downstream PROXY_TOKEN (sk- prefix fixed, random
// suffix editable). Mirrors the legacy card 8: shows the masked current
// value, a "random generate" button (does NOT auto-save), and a save button
// that writes the full token via PUT /api/settings/runtime.

import { zodResolver } from '@hookform/resolvers/zod'
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'

import {
  SettingsSectionCard,
  SettingsSectionSkeleton,
} from '../../../components/settings-section-card'
import {
  asString,
  useRuntimeSettings,
  useUpdateRuntimeSettings,
} from '../../../lib/runtime-settings'

const FORM_ID = 'settings-downstream-proxy-token-form'

const proxyTokenSchema = z.object({
  proxyTokenSuffix: z
    .string()
    .min(8, 'settings.downstream.proxyToken.schema.suffixMinLength'),
})

type ProxyTokenFormValues = z.infer<typeof proxyTokenSchema>

function generateHighEntropySuffix(): string {
  const bytes = new Uint8Array(48)
  crypto.getRandomValues(bytes)
  return Array.from(bytes, (byte) => byte.toString(16).padStart(2, '0')).join(
    ''
  )
}

export function ProxyTokenSection() {
  const { t } = useTranslation()
  const { data, isLoading } = useRuntimeSettings()
  const updateMutation = useUpdateRuntimeSettings()

  const form = useForm<ProxyTokenFormValues>({
    resolver: zodResolver(proxyTokenSchema) as never,
    defaultValues: { proxyTokenSuffix: '' },
  })

  useEffect(() => {
    if (!data) {
      return
    }
    // The runtime response masks the suffix (proxyTokenMasked). We do not
    // prefill the input — the user either generates a new token or types one.
    form.reset({ proxyTokenSuffix: '' }, { keepDirtyValues: true })
  }, [data, form])

  function generateNewSuffix() {
    form.setValue('proxyTokenSuffix', generateHighEntropySuffix(), {
      shouldDirty: true,
    })
  }

  function onSubmit(values: ProxyTokenFormValues) {
    // The backend stores the full token; we prepend the fixed sk- prefix
    // and strip any user-typed sk- prefix to keep the wire format stable.
    const trimmed = values.proxyTokenSuffix.trim().replace(/^sk-/, '')
    updateMutation.mutate(
      { proxyToken: `sk-${trimmed}` },
      {
        onSuccess: () =>
          toast.success(t('settings.downstream.proxyToken.toast.saved')),
        onError: () =>
          toast.error(t('settings.downstream.proxyToken.toast.saveFailed')),
      }
    )
  }

  if (isLoading) {
    return <SettingsSectionSkeleton />
  }

  const masked = asString(data?.proxyTokenMasked)

  return (
    <SettingsSectionCard
      title={t('settings.downstream.proxyToken.title')}
      description={t('settings.downstream.proxyToken.description')}
    >
      <div className='text-muted-foreground mb-4 text-sm'>
        <span>{t('settings.downstream.proxyToken.current')}</span>
        <code className='bg-muted ml-2 rounded px-2 py-0.5 text-xs'>
          {masked || t('settings.downstream.proxyToken.notSet')}
        </code>
      </div>
      <Form {...form}>
        <form
          id={FORM_ID}
          onSubmit={form.handleSubmit(onSubmit)}
          className='space-y-4'
        >
          <FormField
            control={form.control}
            name='proxyTokenSuffix'
            render={({ field }) => (
              <FormItem>
                <FormLabel>
                  {t('settings.downstream.proxyToken.fields.suffix')}
                </FormLabel>
                <div className='flex gap-2'>
                  <FormControl>
                    <Input
                      {...field}
                      value={field.value ?? ''}
                      className='font-mono'
                      placeholder='…'
                    />
                  </FormControl>
                  <Button
                    type='button'
                    variant='outline'
                    size='sm'
                    onClick={generateNewSuffix}
                  >
                    {t('settings.downstream.proxyToken.generate')}
                  </Button>
                </div>
                <FormDescription>
                  {t('settings.downstream.proxyToken.fields.suffixHint')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          <Button
            type='submit'
            form={FORM_ID}
            disabled={updateMutation.isPending}
          >
            {updateMutation.isPending
              ? t('settings.common.saving')
              : t('settings.downstream.proxyToken.save')}
          </Button>
        </form>
      </Form>
    </SettingsSectionCard>
  )
}
