// metapi-go/features/sites — add/edit site form dialog (RHF + Zod + shadcn).
//
// One dialog serves both create and edit. The `editingSite` prop selects the
// mode; when null the dialog is in "add" mode and a successful submit
// triggers `onCreated(createdSite)` so the page can open the guided
// `SiteCreatedModal`. On edit, the form preserves fields the dialog does
// not expose (notably `apiEndpoints` and `customHeaders` when untouched) by
// passing the original values through to the payload — editing the name
// must not wipe the endpoint list.

import { zodResolver } from '@hookform/resolvers/zod'
import { Search as SearchIcon } from 'lucide-react'
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { useDirtyDialogClose } from '@/components/form/dirty-dialog-close'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
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
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { toast } from '@/lib/toast'

import { useCreateSite, useDetectSite, useUpdateSite } from '../api'
import {
  SITE_FORM_DEFAULT_VALUES,
  siteFormSchema,
  type SiteFormValues,
} from '../lib/sites-schema'
import type { Site, SiteFormPayload, SiteProbeScope } from '../types'

type SiteFormDialogProps = {
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
  }
}

function buildPayload(
  values: SiteFormValues,
  editingSite: Site | null
): SiteFormPayload {
  const preservedEndpoints = (editingSite?.apiEndpoints ?? []).map(
    (endpoint) => ({
      url: endpoint.url,
      enabled: endpoint.enabled ?? true,
      sortOrder: endpoint.sortOrder ?? 0,
    })
  )
  return {
    name: values.name,
    url: values.url,
    externalCheckinUrl: values.externalCheckinUrl,
    platform: values.platform,
    proxyUrl: values.proxyUrl,
    useSystemProxy: values.useSystemProxy,
    apiEndpoints: preservedEndpoints,
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

export function SiteFormDialog({
  open,
  onOpenChange,
  editingSite,
  onCreated,
}: SiteFormDialogProps) {
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

  useEffect(() => {
    if (!open) return
    if (editingSite) {
      form.reset(siteToFormValues(editingSite))
    } else {
      form.reset(SITE_FORM_DEFAULT_VALUES)
    }
  }, [open, editingSite, form])

  const watchedUrl = form.watch('url')
  const watchedPlatform = form.watch('platform')
  const probeEnabled = form.watch('postRefreshProbeEnabled')
  const isSubmitting = createSite.isPending || updateSite.isPending

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

  async function handleDetect() {
    const url = watchedUrl.trim()
    if (!url) {
      toast.error(t('sites.form.detectRequiresUrl'))
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
    const payload = buildPayload(values, editingSite)
    try {
      if (isEditing && editingSite) {
        await updateSite.mutateAsync({ id: editingSite.id, payload })
        toast.success(t('sites.form.updateSucceeded', { name: values.name }))
        form.reset()
        onOpenChange(false)
      } else {
        const created = await createSite.mutateAsync(payload)
        toast.success(t('sites.form.createSucceeded', { name: values.name }))
        form.reset()
        onOpenChange(false)
        onCreated?.(created)
      }
    } catch {
      // http-client toasted
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className='sm:max-w-2xl'>
        <DialogHeader>
          <DialogTitle>
            {isEditing ? t('sites.form.editTitle') : t('sites.form.addTitle')}
          </DialogTitle>
          <DialogDescription>
            {isEditing
              ? t('sites.form.editDescription')
              : t('sites.form.addDescription')}
          </DialogDescription>
        </DialogHeader>

        <Form {...form}>
          <form
            onSubmit={form.handleSubmit(onSubmit, () =>
              toast.error(t('sites.form.invalid'))
            )}
            className='grid gap-4'
          >
            <div className='grid gap-4 sm:grid-cols-2'>
              <FormField
                control={form.control}
                name='name'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('sites.form.name')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t('sites.form.namePlaceholder')}
                        autoFocus
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
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('sites.form.platform')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t('sites.form.platformPlaceholder')}
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
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
                      size='sm'
                      onClick={handleDetect}
                      disabled={detectSite.isPending}
                    >
                      {detectSite.isPending ? (
                        <Spinner className='mr-1' />
                      ) : (
                        <SearchIcon className='mr-1 size-3.5' />
                      )}
                      {t('sites.form.detect')}
                    </Button>
                  </div>
                  <FormMessage />
                </FormItem>
              )}
            />

            <div className='grid gap-4 sm:grid-cols-2'>
              <FormField
                control={form.control}
                name='externalCheckinUrl'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('sites.form.externalCheckinUrl')}</FormLabel>
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
            </div>

            <div className='grid gap-4 sm:grid-cols-2'>
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

            <FormField
              control={form.control}
              name='useSystemProxy'
              render={({ field }) => (
                <FormItem className='flex items-center justify-between rounded-lg border p-3'>
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

            <div className='grid gap-4 sm:grid-cols-2'>
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
                          <SelectValue />
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
                          <SelectValue />
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
                    <Textarea
                      rows={3}
                      placeholder='{"X-Custom-Header":"value"}'
                      className='font-mono text-xs'
                      {...field}
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
                <FormItem className='flex items-center justify-between rounded-lg border p-3'>
                  <div className='space-y-0.5'>
                    <FormLabel>
                      {t('sites.form.customHeadersOverrideRequestHeaders')}
                    </FormLabel>
                    <FormDescription>
                      {t('sites.form.customHeadersOverrideRequestHeadersHint')}
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

            <div className='rounded-lg border p-3'>
              <FormField
                control={form.control}
                name='postRefreshProbeEnabled'
                render={({ field }) => (
                  <FormItem className='flex items-center justify-between'>
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
                        onCheckedChange={(checked) => field.onChange(checked)}
                      />
                    </FormControl>
                  </FormItem>
                )}
              />
              {probeEnabled && (
                <div className='mt-3 grid gap-4 sm:grid-cols-2'>
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
                                placeholder={t('sites.form.scopePlaceholder')}
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

            <DialogFooter showCloseButton={false}>
              <Button
                type='button'
                variant='outline'
                onClick={() => onOpenChange(false)}
                disabled={isSubmitting}
              >
                {t('sites.form.cancel')}
              </Button>
              <Button type='submit' disabled={isSubmitting}>
                {isSubmitting && <Spinner className='mr-2' />}
                {isEditing ? t('sites.form.save') : t('sites.form.create')}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
      {guard}
    </Dialog>
  )
}
