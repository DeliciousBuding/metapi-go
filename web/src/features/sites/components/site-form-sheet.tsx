// metapi-go/features/sites — add/edit site form sheet (RHF + Zod + shadcn).
//
// One sheet serves both create and edit. The `editingSite` prop selects the
// mode; when null the sheet is in "add" mode and a successful submit
// triggers `onCreated(createdSite)` so the page can open the guided
// `SiteCreatedModal`. On edit, the form preserves fields the sheet does
// not expose (notably `customHeaders` when untouched) by passing the
// original values through to the payload — editing the name must not wipe
// the endpoint list. `apiEndpoints` IS exposed: the structured endpoint row
// editor (see `components/endpoints-editor.tsx`, free-form JSON textarea
// remains behind the "advanced" toggle), and the untouched-preserve path
// still applies — when the editor is not dirty the original endpoint objects
// are passed through unparsed. The primary site URL is analyzed live; a
// common API request suffix (`/v1`, `/v1/models`, …) is stripped on save
// with a notice (port of the TS original's sitePrimaryUrl guidance).

import { zodResolver } from '@hookform/resolvers/zod'
import { Search as SearchIcon } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { useDirtyDialogClose } from '@/components/form/dirty-dialog-close'
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
import { Notice } from '@/components/ui/notice'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
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
import { Switch } from '@/components/ui/switch'
import type { ConnectionTemplate } from '@/lib/platform-catalog'
import { toast } from '@/lib/toast'

import { useCreateSite, useDetectSite, useUpdateSite } from '../api'
import {
  parseEndpointsEditorText,
  serializeEndpointsForEditor,
  type ParsedEndpoint,
} from '../lib/endpoints'
import { analyzePrimarySiteUrl } from '../lib/site-primary-url'
import {
  SITE_FORM_DEFAULT_VALUES,
  siteFormSchema,
  type SiteFormValues,
} from '../lib/sites-schema'
import type { Site, SiteFormPayload, SiteProbeScope } from '../types'
import { CustomHeadersField } from './custom-headers-field'
import { EndpointsEditor } from './endpoints-editor'
import { SiteConnectionTemplates } from './site-connection-templates'
import { SitePlatformPicker } from './site-platform-picker'

type SiteFormSheetProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  editingSite: Site | null
  onCreated?: (site: Site) => void
}

function nullableBoolToSelectValue(value: boolean | null): string {
  if (value === null) return 'inherit'
  if (value) return 'enabled'
  return 'disabled'
}

function selectValueToNullableBool(
  value: string | null | undefined
): boolean | null {
  if (value === 'inherit' || value == null) return null
  if (value === 'enabled') return true
  return false
}

function matchesTemplate(
  template: ConnectionTemplate,
  url: string,
  platform: string
) {
  function savedURL(value: string) {
    const analysis = analyzePrimarySiteUrl(value)
    return analysis.action === 'auto_strip_known_api_suffix'
      ? analysis.persistedUrl
      : value.trim().replace(/\/+$/, '')
  }
  return (
    platform.trim() === template.platform &&
    savedURL(url) === savedURL(template.url)
  )
}

function siteToFormValues(site: Site): SiteFormValues {
  return {
    name: site.name ?? '',
    url: site.url ?? '',
    externalCheckinUrl: site.externalCheckinUrl ?? '',
    platform: site.platform ?? '',
    proxyUrl: site.proxyUrl ?? '',
    useSystemProxy: site.useSystemProxy ?? false,
    customHeaders: site.customHeaders ?? '',
    customHeadersOverrideRequestHeaders:
      site.customHeadersOverrideRequestHeaders ?? false,
    globalWeight: site.globalWeight ?? 1,
    maxConcurrency: site.maxConcurrency ?? 0,
    postRefreshProbeEnabled: site.postRefreshProbeEnabled ?? false,
    postRefreshProbeModel: site.postRefreshProbeModel ?? '',
    postRefreshProbeScope:
      (site.postRefreshProbeScope as SiteProbeScope | undefined) ?? 'single',
    postRefreshProbeLatencyThresholdMs:
      site.postRefreshProbeLatencyThresholdMs ?? 0,
    resinEnabled: site.resinEnabled ?? null,
    useUtls: site.useUtls ?? null,
    apiEndpointsText: serializeEndpointsForEditor(site.apiEndpoints),
  }
}

function buildPayload(
  values: SiteFormValues,
  editingSite: Site | null,
  apiEndpointsTouched: boolean,
  primaryUrl: string
): SiteFormPayload {
  const preservedEndpoints = (editingSite?.apiEndpoints ?? []).map(
    (endpoint) => ({
      url: endpoint.url,
      enabled: endpoint.enabled ?? true,
      sortOrder: endpoint.sortOrder ?? 0,
    })
  )
  // Untouched preserve: as long as the operator did not edit the endpoints
  // editor, the original endpoint objects pass through unparsed (the
  // row-free behaviour for name-only edits). When touched, the schema
  // has already validated the text, so the parse cannot fail.
  const parsedEndpoints = parseEndpointsEditorText(values.apiEndpointsText)
  let apiEndpoints: ParsedEndpoint[] = preservedEndpoints
  if (apiEndpointsTouched && 'endpoints' in parsedEndpoints) {
    apiEndpoints = parsedEndpoints.endpoints
  }
  return {
    name: values.name,
    url: primaryUrl,
    externalCheckinUrl: values.externalCheckinUrl,
    platform: values.platform,
    proxyUrl: values.proxyUrl,
    useSystemProxy: values.useSystemProxy,
    apiEndpoints,
    customHeaders: values.customHeaders,
    customHeadersOverrideRequestHeaders:
      values.customHeadersOverrideRequestHeaders,
    globalWeight: values.globalWeight,
    maxConcurrency: values.maxConcurrency,
    postRefreshProbeEnabled: values.postRefreshProbeEnabled,
    postRefreshProbeModel: values.postRefreshProbeModel,
    postRefreshProbeScope: values.postRefreshProbeScope,
    postRefreshProbeLatencyThresholdMs:
      values.postRefreshProbeLatencyThresholdMs,
    resinEnabled: values.resinEnabled,
    useUtls: values.useUtls,
  }
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
  const probeEnabled = form.watch('postRefreshProbeEnabled')
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
        : t('sites.templates.applied')
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
    try {
      if (isEditing && editingSite) {
        await updateSite.mutateAsync({ id: editingSite.id, payload })
        toast.success(t('sites.form.updateSucceeded', { name: values.name }))
        if (urlWasNormalized) {
          toast.info(t('sites.form.urlNormalizedToast', { url: persistedUrl }))
        }
        form.reset()
        onOpenChange(false)
      } else {
        const created = await createSite.mutateAsync(payload)
        toast.success(t('sites.form.createSucceeded', { name: values.name }))
        if (urlWasNormalized) {
          toast.info(t('sites.form.urlNormalizedToast', { url: persistedUrl }))
        }
        form.reset()
        onOpenChange(false)
        onCreated?.(created)
      }
    } catch {
      // http-client toasted
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
          <SheetDescription>
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
                                <FormDescription>
                                  {t('sites.form.platformSelectHint')}
                                </FormDescription>
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
                                <FormDescription>
                                  {t('sites.form.platformCustomHint')}
                                </FormDescription>
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
                      {urlAnalysis.action === 'auto_strip_known_api_suffix' &&
                        urlAnalysis.persistedUrl && (
                          <Notice tone='info' size='compact'>
                            {t('sites.form.urlAutoStripInfo', {
                              url: urlAnalysis.persistedUrl,
                            })}
                          </Notice>
                        )}
                      {urlAnalysis.action === 'preserve_api_path' &&
                        urlAnalysis.persistedUrl && (
                          <Notice tone='warning' size='compact'>
                            {t('sites.form.urlPreserveApiPath')}
                          </Notice>
                        )}
                      {urlAnalysis.action === 'preserve_unknown_path' &&
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
              <Separator className='my-6' />
              <fieldset className='min-w-0 space-y-4'>
                <legend className='mb-4 text-sm font-semibold'>
                  {t('sites.form.sections.routing')}
                </legend>
                <div className='grid items-start gap-x-5 gap-y-4 sm:grid-cols-2'>
                  <FormField
                    control={form.control}
                    name='globalWeight'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('sites.form.globalWeight')}</FormLabel>
                        <FormControl>
                          <Input
                            type='number'
                            min={0}
                            step={1}
                            value={field.value}
                            onChange={(event) =>
                              field.onChange(
                                Number.isNaN(event.target.valueAsNumber)
                                  ? 0
                                  : event.target.valueAsNumber
                              )
                            }
                            onBlur={field.onBlur}
                            name={field.name}
                            ref={field.ref}
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                  <FormField
                    control={form.control}
                    name='maxConcurrency'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('sites.form.maxConcurrency')}</FormLabel>
                        <FormControl>
                          <Input
                            type='number'
                            min={0}
                            step={1}
                            value={field.value}
                            onChange={(event) =>
                              field.onChange(
                                Number.isNaN(event.target.valueAsNumber)
                                  ? 0
                                  : event.target.valueAsNumber
                              )
                            }
                            onBlur={field.onBlur}
                            name={field.name}
                            ref={field.ref}
                          />
                        </FormControl>
                        <FormDescription>
                          {t('sites.form.maxConcurrencyHint')}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </div>

                <div className='space-y-4'>
                  <FormField
                    control={form.control}
                    name='postRefreshProbeEnabled'
                    render={({ field }) => (
                      <FormItem className='flex items-center justify-between gap-4'>
                        <div className='space-y-0.5'>
                          <FormLabel>
                            {t('sites.form.postRefreshProbeEnabled')}
                          </FormLabel>
                          <FormDescription>
                            {t('sites.form.postRefreshProbeEnabledHint')}
                          </FormDescription>
                        </div>
                        <FormControl>
                          <Switch
                            checked={field.value}
                            onCheckedChange={(checked) =>
                              field.onChange(checked)
                            }
                          />
                        </FormControl>
                      </FormItem>
                    )}
                  />
                  {probeEnabled && (
                    <div className='mt-3 grid items-start gap-x-5 gap-y-4 sm:grid-cols-2'>
                      <FormField
                        control={form.control}
                        name='postRefreshProbeModel'
                        render={({ field }) => (
                          <FormItem className='sm:col-span-2'>
                            <FormLabel>
                              {t('sites.form.postRefreshProbeModel')}
                            </FormLabel>
                            <FormControl>
                              <Input
                                placeholder={t(
                                  'sites.form.postRefreshProbeModelPlaceholder'
                                )}
                                {...field}
                              />
                            </FormControl>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                      <FormField
                        control={form.control}
                        name='postRefreshProbeScope'
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>
                              {t('sites.form.postRefreshProbeScope')}
                            </FormLabel>
                            <Select
                              value={field.value}
                              onValueChange={(value) =>
                                field.onChange(value as SiteProbeScope)
                              }
                            >
                              <FormControl>
                                <SelectTrigger className='w-full'>
                                  <SelectValue
                                    placeholder={t(
                                      'sites.form.scopePlaceholder'
                                    )}
                                  >
                                    {(value: unknown) =>
                                      value === 'all'
                                        ? t('sites.form.scopeAll')
                                        : t('sites.form.scopeSingle')
                                    }
                                  </SelectValue>
                                </SelectTrigger>
                              </FormControl>
                              <SelectContent>
                                <SelectItem value='single'>
                                  {t('sites.form.scopeSingle')}
                                </SelectItem>
                                <SelectItem value='all'>
                                  {t('sites.form.scopeAll')}
                                </SelectItem>
                              </SelectContent>
                            </Select>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                      <FormField
                        control={form.control}
                        name='postRefreshProbeLatencyThresholdMs'
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>
                              {t(
                                'sites.form.postRefreshProbeLatencyThresholdMsLabel'
                              )}
                            </FormLabel>
                            <FormControl>
                              <Input
                                type='number'
                                min={0}
                                step={100}
                                value={field.value}
                                onChange={(event) =>
                                  field.onChange(
                                    Number.isNaN(event.target.valueAsNumber)
                                      ? 0
                                      : event.target.valueAsNumber
                                  )
                                }
                                onBlur={field.onBlur}
                                name={field.name}
                                ref={field.ref}
                              />
                            </FormControl>
                            <FormDescription>
                              {t(
                                'sites.form.postRefreshProbeLatencyThresholdMsHint'
                              )}
                            </FormDescription>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                    </div>
                  )}
                </div>
              </fieldset>
              <Separator className='my-6' />
              <fieldset className='min-w-0 space-y-4'>
                <legend className='mb-4 text-sm font-semibold'>
                  {t('sites.form.sections.request')}
                </legend>
                <FormField
                  control={form.control}
                  name='proxyUrl'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('sites.form.proxyUrl')}</FormLabel>
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
                <FormField
                  control={form.control}
                  name='useSystemProxy'
                  render={({ field }) => (
                    <FormItem className='flex items-center justify-between gap-4'>
                      <div className='space-y-0.5'>
                        <FormLabel>{t('sites.form.useSystemProxy')}</FormLabel>
                        <FormDescription>
                          {t('sites.form.useSystemProxyHint')}
                        </FormDescription>
                      </div>
                      <FormControl>
                        <Switch
                          checked={field.value}
                          onCheckedChange={(checked) => field.onChange(checked)}
                        />
                      </FormControl>
                    </FormItem>
                  )}
                />

                <div className='grid items-start gap-x-5 gap-y-4 sm:grid-cols-2'>
                  <FormField
                    control={form.control}
                    name='resinEnabled'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('sites.form.resinEnabled')}</FormLabel>
                        <Select
                          value={nullableBoolToSelectValue(field.value)}
                          onValueChange={(value) =>
                            field.onChange(selectValueToNullableBool(value))
                          }
                        >
                          <FormControl>
                            <SelectTrigger className='w-full'>
                              <SelectValue>
                                {(selected) => {
                                  const resinLabels: Record<string, string> = {
                                    enabled: t('sites.form.resinForceOn'),
                                    disabled: t('sites.form.resinForceOff'),
                                    inherit: t('sites.form.resinInherit'),
                                  }
                                  return (
                                    resinLabels[String(selected)] ??
                                    t('sites.form.resinInherit')
                                  )
                                }}
                              </SelectValue>
                            </SelectTrigger>
                          </FormControl>
                          <SelectContent>
                            <SelectItem value='inherit'>
                              {t('sites.form.resinInherit')}
                            </SelectItem>
                            <SelectItem value='enabled'>
                              {t('sites.form.resinForceOn')}
                            </SelectItem>
                            <SelectItem value='disabled'>
                              {t('sites.form.resinForceOff')}
                            </SelectItem>
                          </SelectContent>
                        </Select>
                        <FormDescription>
                          {t('sites.form.resinEnabledHint')}
                        </FormDescription>
                      </FormItem>
                    )}
                  />
                  <FormField
                    control={form.control}
                    name='useUtls'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('sites.form.useUtls')}</FormLabel>
                        <Select
                          value={nullableBoolToSelectValue(field.value)}
                          onValueChange={(value) =>
                            field.onChange(selectValueToNullableBool(value))
                          }
                        >
                          <FormControl>
                            <SelectTrigger className='w-full'>
                              <SelectValue>
                                {(selected) => {
                                  const utlsLabels: Record<string, string> = {
                                    enabled: t('sites.form.utlsForceOn'),
                                    disabled: t('sites.form.utlsForceOff'),
                                    inherit: t('sites.form.utlsInherit'),
                                  }
                                  return (
                                    utlsLabels[String(selected)] ??
                                    t('sites.form.utlsInherit')
                                  )
                                }}
                              </SelectValue>
                            </SelectTrigger>
                          </FormControl>
                          <SelectContent>
                            <SelectItem value='inherit'>
                              {t('sites.form.utlsInherit')}
                            </SelectItem>
                            <SelectItem value='enabled'>
                              {t('sites.form.utlsForceOn')}
                            </SelectItem>
                            <SelectItem value='disabled'>
                              {t('sites.form.utlsForceOff')}
                            </SelectItem>
                          </SelectContent>
                        </Select>
                        <FormDescription>
                          {t('sites.form.useUtlsHint')}
                        </FormDescription>
                      </FormItem>
                    )}
                  />
                </div>

                <FormField
                  control={form.control}
                  name='customHeaders'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('sites.form.customHeaders')}</FormLabel>
                      <FormControl>
                        <CustomHeadersField
                          value={field.value}
                          onChange={field.onChange}
                        />
                      </FormControl>
                      <FormDescription>
                        {t('sites.form.customHeadersHint')}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name='customHeadersOverrideRequestHeaders'
                  render={({ field }) => (
                    <FormItem className='flex items-center justify-between gap-4'>
                      <div className='space-y-0.5'>
                        <FormLabel>
                          {t('sites.form.customHeadersOverrideRequestHeaders')}
                        </FormLabel>
                        <FormDescription>
                          {t(
                            'sites.form.customHeadersOverrideRequestHeadersHint'
                          )}
                        </FormDescription>
                      </div>
                      <FormControl>
                        <Switch
                          checked={field.value}
                          onCheckedChange={(checked) => field.onChange(checked)}
                        />
                      </FormControl>
                    </FormItem>
                  )}
                />
              </fieldset>
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
