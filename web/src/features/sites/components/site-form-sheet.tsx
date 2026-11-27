import { zodResolver } from '@hookform/resolvers/zod'
import { Search as SearchIcon } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { useReauthReplay } from '@/components/common/reauth-provider'
import { useDirtyDialogClose } from '@/components/form/dirty-dialog-close'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Notice } from '@/components/ui/notice'
import { Separator } from '@/components/ui/separator'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Spinner } from '@/components/ui/spinner'
import type { ConnectionTemplate } from '@/lib/platform-catalog'
import { toast } from '@/lib/toast'

import { useCreateSite, useDetectSite, useUpdateSite } from '../api'
import { analyzePrimarySiteUrl } from '../lib/site-primary-url'
import {
  SITE_FORM_DEFAULT_VALUES,
  siteFormSchema,
  type SiteFormValues,
} from '../lib/sites-schema'
import type { Site } from '../types'
import { EndpointsEditor } from './endpoints-editor'
import { SiteConnectionTemplates } from './site-connection-templates'
import { SiteFormAdvancedFields } from './site-form-advanced-fields'
import {
  matchesTemplate,
  siteToFormValues,
  buildPayload,
} from './site-form-values'
import { SitePlatformPicker } from './site-platform-picker'

type SiteFormSheetProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  editingSite: Site | null
  onCreated?: (site: Site) => void
}

export function SiteFormSheet({
  open,
  onOpenChange,
  editingSite,
  onCreated,
}: SiteFormSheetProps) {
  const { t } = useTranslation()
  const isEditing = editingSite !== null

  const form = useForm<SiteFormValues>({
    resolver: zodResolver(siteFormSchema),
    defaultValues: SITE_FORM_DEFAULT_VALUES,
  })

  const { handleOpenChange, guard } = useDirtyDialogClose({
    enabled: form.formState.isDirty,
    onDiscard: () => form.reset(),
    onOpenChange,
  })

  const createSite = useCreateSite()
  const updateSite = useUpdateSite()
  const handleReauth = useReauthReplay()
  const detectSite = useDetectSite()
  const detectSiteAsync = detectSite.mutateAsync
  const [platformMode, setPlatformMode] = useState<'select' | 'custom'>(
    'select'
  )
  const [selectedTemplate, setSelectedTemplate] =
    useState<ConnectionTemplate | null>(null)
  const [templateFeedback, setTemplateFeedback] = useState('')
  const templateValues = useRef<
    Partial<Pick<SiteFormValues, 'name' | 'url' | 'platform'>>
  >({})

  useEffect(() => {
    if (!open) return
    if (editingSite) {
      form.reset(siteToFormValues(editingSite))
    } else {
      form.reset(SITE_FORM_DEFAULT_VALUES)
    }
    setPlatformMode('select')
    setSelectedTemplate(null)
    setTemplateFeedback('')
    templateValues.current = {}
  }, [open, editingSite, form])

  const watchedUrl = form.watch('url')
  const watchedPlatform = form.watch('platform')
  const isSubmitting = createSite.isPending || updateSite.isPending
  // Live primary-site URL classification for the normalization alerts.
  const urlAnalysis = analyzePrimarySiteUrl(watchedUrl)

  useEffect(() => {
    if (
      selectedTemplate?.group === 'api' &&
      !matchesTemplate(selectedTemplate, watchedUrl, watchedPlatform)
    ) {
      setSelectedTemplate(null)
      setTemplateFeedback(t('sites.templates.customized'))
    }
  }, [selectedTemplate, watchedUrl, watchedPlatform, t])

  // Auto-recognize platform when a URL is pasted and no platform has been
  // chosen yet. Unknown sites resolve to an empty result and stay manually
  // specifiable; a user-entered platform always wins over auto-detection.
  useEffect(() => {
    const url = watchedUrl.trim()
    if (!url || isEditing || watchedPlatform.trim() !== '') return

    const timer = window.setTimeout(() => {
      void (async () => {
        try {
          const detected = await detectSiteAsync(url)
          if (detected.platform && !form.getValues('platform').trim()) {
            form.setValue('platform', detected.platform, { shouldDirty: true })
          }
        } catch {
          // Unknown site: leave the platform empty for manual entry.
        }
      })()
    }, 600)

    return () => window.clearTimeout(timer)
  }, [watchedUrl, watchedPlatform, isEditing, detectSiteAsync, form])

  function handlePreset(preset: ConnectionTemplate) {
    const platform = form.getValues('platform').trim()
    if (
      platform &&
      platform !== preset.platform &&
      platform !== templateValues.current.platform
    ) {
      setTemplateFeedback(t('sites.templates.platformConflict'))
      return false
    }
    const nextValues: typeof templateValues.current = {}
    const preserved: string[] = []
    for (const [key, value] of [
      ['name', preset.name],
      ['url', preset.url],
      ['platform', preset.platform],
    ] as const) {
      const current = form.getValues(key)
      if (!current.trim() || current === templateValues.current[key]) {
        form.setValue(key, value, { shouldDirty: true })
        nextValues[key] = value
      } else {
        preserved.push(t(`sites.form.${key}`))
      }
    }
    templateValues.current = nextValues
    setSelectedTemplate(preset)
    setTemplateFeedback(
      preserved.length
        ? t('sites.templates.preserved', { fields: preserved.join(', ') })
        : ''
    )
    return true
  }

  async function handleDetect() {
    const url = watchedUrl.trim()
    if (!url) {
      // Surface the missing URL next to the field instead of a detached
      // toast — the operator needs to know WHICH input to fix.
      form.setError('url', { message: t('sites.form.detectRequiresUrl') })
      return
    }
    try {
      const detected = await detectSite.mutateAsync(url)
      if (detected.platform) {
        form.setValue('platform', detected.platform, { shouldDirty: true })
      }
      if (detected.externalCheckinUrl) {
        form.setValue('externalCheckinUrl', detected.externalCheckinUrl, {
          shouldDirty: true,
        })
      }
      toast.success(t('sites.form.detectSucceeded'))
    } catch {
      // http-client toasted
    }
  }

  async function onSubmit(values: SiteFormValues) {
    const endpointsTouched = form.getFieldState('apiEndpointsText').isDirty
    // Primary URL normalization guidance: a known API request suffix is
    // stripped to the site root on save (mirrors the TS original). Other
    // extra paths are preserved verbatim — the live alerts told the operator.
    const analysis = analyzePrimarySiteUrl(values.url)
    const persistedUrl =
      analysis.action === 'auto_strip_known_api_suffix' && analysis.persistedUrl
        ? analysis.persistedUrl
        : values.url
    const urlWasNormalized = persistedUrl !== values.url.trim()
    const payload = buildPayload(
      values,
      editingSite,
      endpointsTouched,
      persistedUrl
    )
    if (
      !isEditing &&
      selectedTemplate?.group === 'api' &&
      matchesTemplate(selectedTemplate, persistedUrl, values.platform)
    ) {
      payload.initializationPresetId = selectedTemplate.id
    }
    const finishSave = (created?: Site) => {
      toast.success(
        t(
          isEditing
            ? 'sites.form.updateSucceeded'
            : 'sites.form.createSucceeded',
          { name: values.name }
        )
      )
      if (urlWasNormalized) {
        toast.info(t('sites.form.urlNormalizedToast', { url: persistedUrl }))
      }
      form.reset()
      onOpenChange(false)
      if (!isEditing && created) {
        onCreated?.(created)
      }
    }
    const save = async (confirmToken?: string) => {
      if (isEditing && editingSite) {
        await updateSite.mutateAsync({
          id: editingSite.id,
          payload,
          confirmToken,
        })
        finishSave()
        return
      }
      const created = await createSite.mutateAsync({ payload, confirmToken })
      finishSave(created)
    }
    try {
      await save()
    } catch (error) {
      // Setting a per-site proxy redirects stored upstream credentials; the
      // backend rejects it with reauthRequired until the master token is
      // re-entered. Replay the same form values once through the dialog.
      handleReauth(error, t('reauth.operations.siteProxy'), (confirmToken) =>
        save(confirmToken)
      )
    }
  }

  return (
    <Sheet open={open} onOpenChange={handleOpenChange}>
      <SheetContent
        className='gap-0 overflow-hidden sm:max-w-2xl'
        showMobileCloseBar={false}
      >
        <SheetHeader className='shrink-0 border-b px-4 py-5 pr-12 sm:px-6'>
          <SheetTitle>
            {isEditing ? t('sites.form.editTitle') : t('sites.form.addTitle')}
          </SheetTitle>
          <SheetDescription className='sr-only'>
            {isEditing
              ? t('sites.form.editDescription')
              : t('sites.form.addDescription')}
          </SheetDescription>
        </SheetHeader>

        <Form {...form}>
          <form
            onSubmit={form.handleSubmit(onSubmit, () =>
              toast.error(t('sites.form.invalid'))
            )}
            className='flex min-h-0 flex-1 flex-col'
          >
            <div
              data-slot='site-form-body'
              className='min-h-0 flex-1 overflow-y-auto overscroll-contain px-4 py-6 sm:px-6'
            >
              {!isEditing && (
                <>
                  <SiteConnectionTemplates
                    selected={selectedTemplate}
                    feedback={templateFeedback}
                    url={watchedUrl}
                    platform={watchedPlatform}
                    onSelect={handlePreset}
                  />
                  <Separator className='my-6' />
                </>
              )}
              <fieldset className='min-w-0 space-y-4'>
                <legend className='mb-4 text-sm font-semibold'>
                  {t('sites.form.sections.connection')}
                </legend>
                <div className='grid items-start gap-x-5 gap-y-4 sm:grid-cols-2'>
                  <FormField
                    control={form.control}
                    name='name'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('sites.form.name')}</FormLabel>
                        <FormControl>
                          <Input
                            placeholder={t('sites.form.namePlaceholder')}
                            autoFocus={isEditing}
                            {...field}
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                  <FormField
                    control={form.control}
                    name='platform'
                    render={({ field }) => {
                      return (
                        <FormItem>
                          <FormLabel>{t('sites.form.platform')}</FormLabel>
                          {platformMode === 'select' ? (
                            <>
                              <FormControl>
                                <SitePlatformPicker
                                  value={field.value}
                                  onValueChange={field.onChange}
                                />
                              </FormControl>
                              <div className='flex items-center justify-between gap-2'>
                                <Button
                                  type='button'
                                  variant='link'
                                  size='xs'
                                  onClick={() => setPlatformMode('custom')}
                                >
                                  {t('sites.form.platformCustomToggle')}
                                </Button>
                              </div>
                            </>
                          ) : (
                            <>
                              <FormControl>
                                <Input
                                  placeholder={t(
                                    'sites.form.platformPlaceholder'
                                  )}
                                  {...field}
                                />
                              </FormControl>
                              <div className='flex items-center justify-between gap-2'>
                                <Button
                                  type='button'
                                  variant='link'
                                  size='xs'
                                  onClick={() => setPlatformMode('select')}
                                >
                                  {t('sites.form.platformSelectToggle')}
                                </Button>
                              </div>
                            </>
                          )}
                          <FormMessage />
                        </FormItem>
                      )
                    }}
                  />
                </div>

                <FormField
                  control={form.control}
                  name='url'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('sites.form.url')}</FormLabel>
                      <div className='flex gap-2'>
                        <FormControl>
                          <Input
                            placeholder='https://example.com'
                            className='flex-1'
                            {...field}
                          />
                        </FormControl>
                        <Button
                          type='button'
                          variant='outline'
                          onClick={handleDetect}
                          disabled={detectSite.isPending}
                        >
                          {detectSite.isPending ? (
                            <Spinner />
                          ) : (
                            <SearchIcon className='size-3.5' />
                          )}
                          {t('sites.form.detect')}
                        </Button>
                      </div>
                      {selectedTemplate?.group !== 'api' &&
                        urlAnalysis.action === 'auto_strip_known_api_suffix' &&
                        urlAnalysis.persistedUrl && (
                          <Notice tone='info' size='compact'>
                            {t('sites.form.urlAutoStripInfo', {
                              url: urlAnalysis.persistedUrl,
                            })}
                          </Notice>
                        )}
                      {selectedTemplate?.group !== 'api' &&
                        urlAnalysis.action === 'preserve_api_path' &&
                        urlAnalysis.persistedUrl && (
                          <Notice tone='warning' size='compact'>
                            {t('sites.form.urlPreserveApiPath')}
                          </Notice>
                        )}
                      {selectedTemplate?.group !== 'api' &&
                        urlAnalysis.action === 'preserve_unknown_path' &&
                        urlAnalysis.persistedUrl && (
                          <Notice tone='warning' size='compact'>
                            {t('sites.form.urlPreserveUnknownPath')}
                          </Notice>
                        )}
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name='apiEndpointsText'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('sites.form.apiEndpoints')}</FormLabel>
                      <FormControl>
                        <EndpointsEditor
                          value={field.value}
                          onChange={field.onChange}
                          liveEndpoints={editingSite?.apiEndpoints}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name='externalCheckinUrl'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        {t('sites.form.externalCheckinUrl')}
                      </FormLabel>
                      <FormControl>
                        <Input
                          placeholder={t('sites.form.optionalUrlPlaceholder')}
                          {...field}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </fieldset>
              <SiteFormAdvancedFields form={form} />
            </div>
            <SheetFooter className='shrink-0 flex-row justify-end gap-2 border-t px-4 py-4 sm:px-6'>
              <Button
                type='button'
                variant='outline'
                onClick={() => handleOpenChange(false)}
                disabled={isSubmitting}
              >
                {t('sites.form.cancel')}
              </Button>
              <Button type='submit' disabled={isSubmitting}>
                {isSubmitting && <Spinner />}
                {isEditing ? t('sites.form.save') : t('sites.form.create')}
              </Button>
            </SheetFooter>
          </form>
        </Form>
      </SheetContent>
      {guard}
    </Sheet>
  )
}
