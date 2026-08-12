// metapi-go/features/settings/sections/downstream/components — proxy-token
// section. The global downstream PROXY_TOKEN (sk- prefix fixed, random
// suffix editable). Mirrors the legacy card 8: shows the masked current
// value, a "random generate" button (does NOT auto-save), and a save button
// that writes the full token via PUT /api/settings/runtime. The input is
// never prefilled from the server; a blank (unchanged) field is not sent.

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

import { FormNavigationGuard } from '../../../components/form-navigation-guard'
import { SettingsFormActions } from '../../../components/settings-form-actions'
import { SettingsSectionCard } from '../../../components/settings-section-card'
import { SettingsSectionError } from '../../../components/settings-section-error'
import { useSettingsForm } from '../../../hooks/use-settings-form'
import {
  collectChangedFields,
  hasChanges,
} from '../../../lib/collect-changed-fields'
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

const DEFAULT_VALUES: ProxyTokenFormValues = { proxyTokenSuffix: '' }

function generateHighEntropySuffix(): string {
  const bytes = new Uint8Array(48)
  crypto.getRandomValues(bytes)
  return Array.from(bytes, (byte) => byte.toString(16).padStart(2, '0')).join(
    ''
  )
}

export function ProxyTokenSection() {
  const { t } = useTranslation()
  const { data, isLoading, isError, refetch } = useRuntimeSettings()
  const updateMutation = useUpdateRuntimeSettings()

  // The runtime response masks the suffix; we never prefill the input. The
  // server snapshot is always the empty baseline so a typed value is always
  // "dirty" and a blank value is never submitted.
  const { form, baseline, syncFromServer } =
    useSettingsForm<ProxyTokenFormValues>({
      schema: proxyTokenSchema,
      defaultValues: DEFAULT_VALUES,
      serverValues: data ? { proxyTokenSuffix: '' } : null,
    })

  function generateNewSuffix() {
    form.setValue('proxyTokenSuffix', generateHighEntropySuffix(), {
      shouldDirty: true,
    })
  }

  function onSubmit(values: ProxyTokenFormValues) {
    const changed = collectChangedFields(
      values as unknown as Record<string, unknown>,
      baseline as unknown as Record<string, unknown> | null
    ) as Partial<ProxyTokenFormValues>
    if (!hasChanges(changed) || !changed.proxyTokenSuffix) {
      toast.info(t('settings.common.noChanges'))
      return
    }
    const trimmed = changed.proxyTokenSuffix.trim().replace(/^sk-/, '')
    updateMutation.mutate(
      { proxyToken: `sk-${trimmed}` },
      {
        onSuccess: () => {
          syncFromServer({ proxyTokenSuffix: '' })
          toast.success(t('settings.downstream.proxyToken.toast.saved'))
        },
        onError: () =>
          toast.error(t('settings.downstream.proxyToken.toast.saveFailed')),
      }
    )
  }

  if (isLoading) {
    return (
      <SettingsSectionCard
        title={t('settings.downstream.proxyToken.title')}
        description={t('settings.downstream.proxyToken.description')}
      >
        <p className='text-muted-foreground text-sm'>
          {t('settings.common.loading')}
        </p>
      </SettingsSectionCard>
    )
  }

  if (isError || !data) {
    return (
      <SettingsSectionError
        title={t('settings.downstream.proxyToken.title')}
        onRetry={() => void refetch()}
      />
    )
  }

  const isDirty = form.formState.isDirty
  const masked = asString(data.proxyTokenMasked)

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
          <SettingsFormActions
            formId={FORM_ID}
            isDirty={isDirty}
            isPending={updateMutation.isPending}
            onReset={() => syncFromServer(DEFAULT_VALUES)}
            saveLabel={t('settings.downstream.proxyToken.save')}
          />
        </form>
      </Form>
      <FormNavigationGuard enabled={isDirty} />
    </SettingsSectionCard>
  )
}
