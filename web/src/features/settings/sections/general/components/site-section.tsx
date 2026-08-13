// metapi-go/features/settings/sections/general/components — site & branding
// section. Six branding keys (systemName / logo / footer / about /
// homePageContent / serverAddress) read from and written back through
// GET/PUT /api/settings/runtime via the shared settings form.

import { useTranslation } from 'react-i18next'
import { z } from 'zod'

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
import { Textarea } from '@/components/ui/textarea'
import { toast } from '@/lib/toast'

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
  type RuntimeSettings,
} from '../../../lib/runtime-settings'

const FORM_ID = 'settings-general-site-form'

const siteSchema = z.object({
  systemName: z.string().optional(),
  logo: z.string().optional(),
  footer: z.string().optional(),
  about: z.string().optional(),
  homePageContent: z.string().optional(),
  serverAddress: z.string().optional(),
})

type SiteFormValues = z.infer<typeof siteSchema>

const DEFAULT_VALUES: SiteFormValues = {
  systemName: '',
  logo: '',
  footer: '',
  about: '',
  homePageContent: '',
  serverAddress: '',
}

function deriveServerValues(
  data: RuntimeSettings | undefined
): SiteFormValues | null {
  if (!data) {
    return null
  }
  return {
    systemName: asString(data.systemName),
    logo: asString(data.logo),
    footer: asString(data.footer),
    about: asString(data.about),
    homePageContent: asString(data.homePageContent),
    serverAddress: asString(data.serverAddress),
  }
}

export function SiteSection() {
  const { t } = useTranslation()
  const { data, isLoading, isError, refetch } = useRuntimeSettings()
  const updateMutation = useUpdateRuntimeSettings()

  const serverValues = deriveServerValues(data)
  const { form, baseline, syncFromServer } = useSettingsForm<SiteFormValues>({
    schema: siteSchema,
    defaultValues: DEFAULT_VALUES,
    serverValues,
  })

  function onSubmit(values: SiteFormValues) {
    const changed = collectChangedFields(
      values as unknown as Record<string, unknown>,
      baseline as unknown as Record<string, unknown> | null
    ) as Partial<SiteFormValues>
    if (!hasChanges(changed)) {
      toast.info(t('settings.common.noChanges'))
      return
    }
    updateMutation.mutate(changed, {
      onSuccess: () => toast.success(t('settings.general.site.toast.saved')),
      onError: () => toast.error(t('settings.general.site.toast.saveFailed')),
    })
  }

  if (isLoading) {
    return (
      <SettingsSectionCard
        title={t('settings.general.site.title')}
        description={t('settings.general.site.description')}
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
        title={t('settings.general.site.title')}
        onRetry={() => void refetch()}
      />
    )
  }

  const isDirty = form.formState.isDirty

  return (
    <SettingsSectionCard
      title={t('settings.general.site.title')}
      description={t('settings.general.site.description')}
    >
      <Form {...form}>
        <form
          id={FORM_ID}
          onSubmit={form.handleSubmit(onSubmit)}
          className='space-y-4'
        >
          <FormField
            control={form.control}
            name='systemName'
            render={({ field }) => (
              <FormItem>
                <FormLabel>
                  {t('settings.general.site.fields.systemName')}
                </FormLabel>
                <FormControl>
                  <Input
                    {...field}
                    value={field.value ?? ''}
                    placeholder={t(
                      'settings.general.site.fields.systemNamePlaceholder'
                    )}
                  />
                </FormControl>
                <FormDescription>
                  {t('settings.general.site.fields.systemNameHint')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='logo'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('settings.general.site.fields.logo')}</FormLabel>
                <FormControl>
                  <Input
                    {...field}
                    value={field.value ?? ''}
                    placeholder='https://example.com/logo.png'
                  />
                </FormControl>
                <FormDescription>
                  {t('settings.general.site.fields.logoHint')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='footer'
            render={({ field }) => (
              <FormItem>
                <FormLabel>
                  {t('settings.general.site.fields.footer')}
                </FormLabel>
                <FormControl>
                  <Input
                    {...field}
                    value={field.value ?? ''}
                    placeholder={t(
                      'settings.general.site.fields.footerPlaceholder'
                    )}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='about'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('settings.general.site.fields.about')}</FormLabel>
                <FormControl>
                  <Textarea {...field} value={field.value ?? ''} rows={4} />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='homePageContent'
            render={({ field }) => (
              <FormItem>
                <FormLabel>
                  {t('settings.general.site.fields.homePageContent')}
                </FormLabel>
                <FormControl>
                  <Textarea
                    {...field}
                    value={field.value ?? ''}
                    rows={6}
                    placeholder={t(
                      'settings.general.site.fields.homePageContentPlaceholder'
                    )}
                  />
                </FormControl>
                <FormDescription>
                  {t('settings.general.site.fields.homePageContentHint')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='serverAddress'
            render={({ field }) => (
              <FormItem>
                <FormLabel>
                  {t('settings.general.site.fields.serverAddress')}
                </FormLabel>
                <FormControl>
                  <Input
                    {...field}
                    value={field.value ?? ''}
                    placeholder='https://api.example.com'
                  />
                </FormControl>
                <FormDescription>
                  {t('settings.general.site.fields.serverAddressHint')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          <SettingsFormActions
            formId={FORM_ID}
            isDirty={isDirty}
            isPending={updateMutation.isPending}
            onReset={() =>
              syncFromServer(deriveServerValues(data) ?? DEFAULT_VALUES)
            }
          />
        </form>
      </Form>
      <FormNavigationGuard enabled={isDirty} />
    </SettingsSectionCard>
  )
}
