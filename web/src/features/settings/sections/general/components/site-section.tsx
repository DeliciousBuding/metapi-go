// metapi-go/features/settings/sections/general/components — site & branding
// section. Reads branding keys from the runtime-settings document and writes
// them back via PUT /api/settings/runtime. The branding keys
// (systemName / logo / footer / about / homePageContent / serverAddress) are
// not declared on RuntimeSettingsPayload (the legacy TS backend stored them
// there anyway); the loose RuntimeSettings bag lets the form read them.

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
import { Textarea } from '@/components/ui/textarea'

import {
  SettingsSectionCard,
  SettingsSectionSkeleton,
} from '../../../components/settings-section-card'
import {
  asString,
  useRuntimeSettings,
  useUpdateRuntimeSettings,
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

export function SiteSection() {
  const { t } = useTranslation()
  const { data, isLoading } = useRuntimeSettings()
  const updateMutation = useUpdateRuntimeSettings()

  const form = useForm<SiteFormValues>({
    resolver: zodResolver(siteSchema) as never,
    defaultValues: {
      systemName: '',
      logo: '',
      footer: '',
      about: '',
      homePageContent: '',
      serverAddress: '',
    },
  })

  useEffect(() => {
    if (!data) {
      return
    }
    form.reset(
      {
        systemName: asString(data.systemName),
        logo: asString(data.logo),
        footer: asString(data.footer),
        about: asString(data.about),
        homePageContent: asString(data.homePageContent),
        serverAddress: asString(data.serverAddress),
      },
      { keepDirtyValues: true },
    )
  }, [data, form])

  function onSubmit(values: SiteFormValues) {
    updateMutation.mutate(values as never, {
      onSuccess: () => toast.success(t('settings.general.site.toast.saved')),
      onError: () => toast.error(t('settings.general.site.toast.saveFailed')),
    })
  }

  if (isLoading) {
    return <SettingsSectionSkeleton />
  }

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
                <FormLabel>{t('settings.general.site.fields.systemName')}</FormLabel>
                <FormControl>
                  <Input
                    {...field}
                    value={field.value ?? ''}
                    placeholder='TokenDance Gateway'
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
                <FormLabel>{t('settings.general.site.fields.footer')}</FormLabel>
                <FormControl>
                  <Input
                    {...field}
                    value={field.value ?? ''}
                    placeholder='© 2026 TokenDance'
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
                  <Textarea
                    {...field}
                    value={field.value ?? ''}
                    rows={4}
                  />
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
                    placeholder='Markdown supported…'
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
          <Button
            type='submit'
            form={FORM_ID}
            disabled={updateMutation.isPending}
          >
            {updateMutation.isPending
              ? t('settings.common.saving')
              : t('settings.common.save')}
          </Button>
        </form>
      </Form>
    </SettingsSectionCard>
  )
}
