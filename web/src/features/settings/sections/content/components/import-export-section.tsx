// metapi-go/features/settings/sections/content/components — import/export
// section. Three export types (now checked for non-OK responses), JSON paste
// import with a preview + confirmation step, and a WebDAV auto-backup config
// form using the shared semantic schedule editor.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  ArrowRight,
  Download,
  Upload,
  Cloud,
  Database,
  Users,
  SlidersHorizontal,
} from 'lucide-react'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { ReauthDialog } from '@/components/common/reauth-dialog'
import { SectionCard } from '@/components/common/section-card'
import { SectionError } from '@/components/common/section-error'
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
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import {
  api,
  type BackupWebdavExportType,
  type BackupWebdavResponse,
} from '@/lib/api'
import {
  detectExternalBackupSourceText,
  type ExternalBackupSource,
} from '@/lib/helpers/external-backup-source'
import { isReauthRequired } from '@/lib/http-client'
import { toast } from '@/lib/toast'

import { FormNavigationGuard } from '../../../components/form-navigation-guard'
import { ScheduleEditor } from '../../../components/schedule-editor'
import { SettingsFormActions } from '../../../components/settings-form-actions'
import { SettingsSubsection } from '../../../components/settings-subsection'
import { useSettingsForm } from '../../../hooks/use-settings-form'
import {
  collectChangedFields,
  hasChanges,
} from '../../../lib/collect-changed-fields'
import { scheduleFromLegacy, scheduleToCron } from '../../../lib/schedule'
import {
  BackupImportPreviewPanel,
  type BackupImportPreview,
  type BackupImportTablePlan,
  type AxonHubV14Preview,
  type OctopusV5Preview,
} from './backup-import-preview-panel'
import { BackupImportSource } from './backup-import-source'

const WEBDAV_FORM_ID = 'settings-content-import-export-webdav-form'
const BACKUP_IMPORT_MAX_BYTES = 20 * 1024 * 1024

type ImportPreviewSnapshot = {
  raw: string
  originKey?: string
  source?: ExternalBackupSource
  plan: BackupImportPreview
}

function isExternalPreview(
  plan: BackupImportTablePlan | OctopusV5Preview | AxonHubV14Preview
): plan is OctopusV5Preview | AxonHubV14Preview {
  const candidate = plan as OctopusV5Preview
  return (
    typeof candidate.source === 'string' &&
    typeof candidate.originKey === 'string' &&
    typeof candidate.sections === 'object' &&
    candidate.sections !== null &&
    !Array.isArray(candidate.sections) &&
    (candidate.blocking === undefined || Array.isArray(candidate.blocking))
  )
}

const webdavSchema = z.object({
  enabled: z.boolean(),
  fileUrl: z.string().optional(),
  username: z.string().optional(),
  password: z.string().optional(),
  exportType: z.enum(['all', 'accounts', 'preferences']),
  autoSyncEnabled: z.boolean(),
  autoSyncSchedule: z.discriminatedUnion('kind', [
    z.object({
      version: z.literal(1),
      kind: z.literal('daily'),
      time: z.string(),
    }),
    z.object({
      version: z.literal(1),
      kind: z.literal('interval'),
      everyHours: z.number().int().min(1).max(24),
    }),
    z.object({
      version: z.literal(1),
      kind: z.literal('window'),
      windowStart: z.string(),
      windowEnd: z.string(),
    }),
    z.object({
      version: z.literal(1),
      kind: z.literal('custom'),
      cron: z.string(),
    }),
  ]),
})

type WebdavFormValues = z.infer<typeof webdavSchema>

const DEFAULT_WEBDAV_VALUES: WebdavFormValues = {
  enabled: false,
  fileUrl: '',
  username: '',
  password: '',
  exportType: 'all',
  autoSyncEnabled: false,
  autoSyncSchedule: { version: 1, kind: 'interval', everyHours: 6 },
}

const webdavQueryKeys = {
  all: ['backup-webdav'] as const,
}

function downloadTextFile(filename: string, text: string) {
  const blob = new Blob([text], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = filename
  anchor.click()
  URL.revokeObjectURL(url)
}

export function ImportExportSection() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [importText, setImportText] = useState('')
  const [importFileName, setImportFileName] = useState('')
  const [externalOriginKey, setExternalOriginKey] = useState('')
  const [allowChannelsOnlyImport, setAllowChannelsOnlyImport] = useState(false)
  const [importPreview, setImportPreview] =
    useState<BackupImportPreview | null>(null)
  const [previewSnapshot, setPreviewSnapshot] =
    useState<ImportPreviewSnapshot | null>(null)
  const importRevision = useRef(0)
  const [confirmImportOpen, setConfirmImportOpen] = useState(false)
  const [confirmWebdavImportOpen, setConfirmWebdavImportOpen] = useState(false)
  // #1034: backup export is a sensitive op — the operator must re-present
  // the master token before the export request is sent.
  const [reauthTarget, setReauthTarget] = useState<
    | { kind: 'download'; type: 'all' | 'accounts' | 'preferences' }
    | { kind: 'webdav' }
    | null
  >(null)
  const [reauthError, setReauthError] = useState<string | null>(null)

  const webdavQuery = useQuery<BackupWebdavResponse>({
    queryKey: webdavQueryKeys.all,
    queryFn: async () => api.getBackupWebdavConfig(),
    staleTime: 30 * 1000,
  })

  const config = webdavQuery.data?.config

  const { form, baseline, syncFromServer } = useSettingsForm<WebdavFormValues>({
    schema: webdavSchema,
    defaultValues: DEFAULT_WEBDAV_VALUES,
    serverValues: config
      ? {
          enabled: config.enabled,
          fileUrl: config.fileUrl,
          username: config.username,
          password: '',
          exportType: config.exportType,
          autoSyncEnabled: config.autoSyncEnabled,
          autoSyncSchedule: scheduleFromLegacy({ cron: config.autoSyncCron }),
        }
      : null,
  })

  const exportMutation = useMutation({
    mutationFn: async ({
      type,
      confirmToken,
    }: {
      type: BackupWebdavExportType
      confirmToken: string
    }) => {
      const text = await api.exportBackupRaw(type, confirmToken)
      return { type, text }
    },
    onSuccess: ({ type, text }) => {
      setReauthTarget(null)
      setReauthError(null)
      const today = new Date().toISOString().slice(0, 10)
      downloadTextFile(`metapi-${type}-${today}.json`, text)
      toast.success(t('settings.content.importExport.toast.exported'))
    },
    onError: (error: unknown) => {
      if (isReauthRequired(error)) {
        setReauthError(t('reauth.errorInvalid'))
        return
      }
      setReauthTarget(null)
      setReauthError(null)
      toast.error(t('settings.content.importExport.toast.exportFailed'))
    },
  })

  /**
   * A backup import merges rows across the whole schema — the backend's
   * `AllTables` (service/backup/export.go) spans ~30 tables (sites, accounts,
   * account tokens, routes, channels, check-in logs, model availability,
   * OAuth route units, settings, …), far beyond any hand-maintained key list.
   * Invalidate the entire query cache so every list page, dashboard widget and
   * settings section refetches the restored data instead of serving stale rows
   * (the previous targeted list missed routes/channels/check-in/oauth and the
   * shared runtime-settings key).
   */
  function invalidateAfterImport() {
    void queryClient.invalidateQueries()
  }

  const previewMutation = useMutation({
    mutationFn: async (input: { raw: string; originKey?: string }) => {
      const { raw, originKey } = input
      const data = JSON.parse(raw) as unknown
      const external = detectExternalBackupSourceText(raw)
      const result = (await api.previewBackupImport(
        data,
        external ? originKey : undefined
      )) as {
        success?: boolean
        plan?: BackupImportTablePlan | OctopusV5Preview | AxonHubV14Preview
      }
      if (external) {
        const plan = result.plan
        if (!plan || !isExternalPreview(plan)) {
          throw new Error('Invalid external import preview')
        }
        if (plan.originKey !== originKey) {
          throw new Error('Preview origin key does not match the request')
        }
        if (external === 'axonhub-v1.4') {
          const axonhub = plan as AxonHubV14Preview
          if (
            typeof axonhub.routable !== 'object' ||
            axonhub.routable === null ||
            Array.isArray(axonhub.routable) ||
            (axonhub.residuals !== undefined &&
              !Array.isArray(axonhub.residuals)) ||
            (axonhub.skippedChannels !== undefined &&
              !Array.isArray(axonhub.skippedChannels))
          ) {
            throw new Error('Invalid AxonHub v1.4 import preview')
          }
          return {
            kind: 'axonhub' as const,
            data: axonhub,
          }
        }
        const octopus = plan as OctopusV5Preview
        if (
          octopus.adaptations !== undefined &&
          !Array.isArray(octopus.adaptations)
        ) {
          throw new Error('Invalid Octopus v5 import preview')
        }
        return {
          kind: 'octopus' as const,
          data: octopus,
        }
      }
      if (
        !result.plan ||
        typeof result.plan !== 'object' ||
        Array.isArray(result.plan)
      ) {
        throw new Error('Invalid backup import preview')
      }
      return {
        kind: 'tables' as const,
        tables: result.plan as BackupImportTablePlan,
      }
    },
  })

  const importMutation = useMutation({
    mutationFn: async (input: {
      raw: string
      originKey?: string
      source?: ExternalBackupSource
      channelsOnly?: boolean
      replaceOrigin?: boolean
    }) => {
      const { raw, originKey, source, channelsOnly, replaceOrigin } = input
      const data = JSON.parse(raw) as unknown
      const octopus = source === 'octopus-v5'
      return api.importBackup(
        data,
        source ? originKey : undefined,
        octopus && channelsOnly ? 'channels-only' : undefined,
        octopus && replaceOrigin,
        source === 'axonhub-v1.4' && replaceOrigin
      )
    },
    onSuccess: () => {
      toast.success(t('settings.content.importExport.toast.imported'))
      setImportText('')
      setImportFileName('')
      setExternalOriginKey('')
      setAllowChannelsOnlyImport(false)
      setImportPreview(null)
      setPreviewSnapshot(null)
      invalidateAfterImport()
    },
    onError: () =>
      toast.error(t('settings.content.importExport.toast.importFailed')),
  })

  function resetImportReview() {
    importRevision.current++
    setImportPreview(null)
    setPreviewSnapshot(null)
    setConfirmImportOpen(false)
    setAllowChannelsOnlyImport(false)
  }

  async function handleImportFile(file: File) {
    resetImportReview()
    const revision = importRevision.current
    setImportText('')
    setImportFileName('')
    if (file.size > BACKUP_IMPORT_MAX_BYTES) {
      toast.error(t('settings.content.importExport.importFileTooLarge'))
      return
    }
    try {
      const raw = await file.text()
      if (revision !== importRevision.current) return
      setImportText(raw)
      setImportFileName(file.name)
    } catch {
      if (revision !== importRevision.current) return
      toast.error(t('settings.content.importExport.toast.importFailed'))
    }
  }

  async function handlePreviewImport() {
    const raw = importText
    const originKey = externalOriginKey.trim()
    const source = detectExternalBackupSourceText(raw)
    const revision = importRevision.current
    try {
      if (source && !originKey) {
        toast.error(t('settings.content.importExport.originKeyRequired'))
        return
      }
      const plan = await previewMutation.mutateAsync({ raw, originKey })
      if (revision !== importRevision.current) return
      setImportPreview(plan)
      setPreviewSnapshot({
        raw,
        originKey: plan.kind === 'tables' ? undefined : originKey,
        source: plan.kind === 'tables' ? undefined : (source ?? undefined),
        plan,
      })
    } catch {
      if (revision !== importRevision.current) return
      toast.error(t('settings.content.importExport.toast.importFailed'))
    }
  }

  const saveWebdavMutation = useMutation({
    mutationFn: async (values: Partial<WebdavFormValues>) => {
      const payload: Parameters<typeof api.saveBackupWebdavConfig>[0] = {}
      if (values.enabled !== undefined) payload.enabled = values.enabled
      if (values.fileUrl !== undefined) payload.fileUrl = values.fileUrl ?? ''
      if (values.username !== undefined) {
        payload.username = values.username ?? ''
      }
      if (values.password) payload.password = values.password
      if (values.exportType !== undefined) {
        payload.exportType = values.exportType
      }
      if (values.autoSyncEnabled !== undefined) {
        payload.autoSyncEnabled = values.autoSyncEnabled
      }
      if (values.autoSyncSchedule !== undefined) {
        payload.autoSyncCron =
          scheduleToCron(values.autoSyncSchedule, config?.autoSyncCron) ??
          config?.autoSyncCron ??
          '0 */6 * * *'
      }
      return api.saveBackupWebdavConfig(payload)
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: webdavQueryKeys.all })
      toast.success(t('settings.content.importExport.toast.webdavSaved'))
    },
    onError: () =>
      toast.error(t('settings.content.importExport.toast.webdavSaveFailed')),
  })

  const exportWebdavMutation = useMutation({
    mutationFn: async ({
      type,
      confirmToken,
    }: {
      type?: BackupWebdavExportType
      confirmToken: string
    }) => api.exportBackupToWebdav(type, confirmToken),
    onSuccess: () => {
      setReauthTarget(null)
      setReauthError(null)
      toast.success(t('settings.content.importExport.toast.webdavExported'))
    },
    onError: (error: unknown) => {
      if (isReauthRequired(error)) {
        setReauthError(t('reauth.errorInvalid'))
        return
      }
      setReauthTarget(null)
      setReauthError(null)
      toast.error(t('settings.content.importExport.toast.webdavExportFailed'))
    },
  })

  const importWebdavMutation = useMutation({
    mutationFn: async () => api.importBackupFromWebdav(),
    onSuccess: () => {
      toast.success(t('settings.content.importExport.toast.webdavImported'))
      invalidateAfterImport()
    },
    onError: () =>
      toast.error(t('settings.content.importExport.toast.webdavImportFailed')),
  })

  function onWebdavSubmit(values: WebdavFormValues) {
    const changed = collectChangedFields(values, baseline)
    if (!hasChanges(changed)) {
      toast.info(t('settings.common.noChanges'))
      return
    }
    saveWebdavMutation.mutate(changed)
  }

  const externalImportSource = detectExternalBackupSourceText(importText)
  const hasNotImportedOctopusSections =
    importPreview?.kind === 'octopus' &&
    Object.values(importPreview.data.notImported ?? {}).some(
      (count) => count > 0
    )
  const hasOctopusPolicyAdaptations =
    importPreview?.kind === 'octopus' &&
    (importPreview.data.adaptations?.length ?? 0) > 0
  const canImport =
    previewSnapshot !== null &&
    !(importPreview && importPreview.kind !== 'tables'
      ? (importPreview.data.blocking?.length ?? 0) > 0
      : false) &&
    (!(hasNotImportedOctopusSections || hasOctopusPolicyAdaptations) ||
      allowChannelsOnlyImport)
  let confirmImportDescriptionKey =
    'settings.content.importExport.importConfirmDescription'
  if (importPreview?.kind === 'octopus') {
    confirmImportDescriptionKey =
      hasNotImportedOctopusSections || hasOctopusPolicyAdaptations
        ? 'settings.content.importExport.octopusPartialImportConfirmDescription'
        : 'settings.content.importExport.octopusImportConfirmDescription'
  } else if (importPreview?.kind === 'axonhub') {
    confirmImportDescriptionKey =
      'settings.content.importExport.axonhubImportConfirmDescription'
  }
  const isWebdavDirty = form.formState.isDirty
  const hasExternalRemovals =
    importPreview !== null &&
    importPreview.kind !== 'tables' &&
    Object.values(importPreview.data.removals ?? {}).some((count) => count > 0)

  return (
    <SectionCard
      title={t('settings.content.importExport.title')}
      description={t('settings.content.importExport.description')}
    >
      <div className='space-y-6'>
        <SettingsSubsection
          title={t('settings.content.importExport.importGroup')}
          description={t('settings.content.importExport.design.importHint')}
          icon={<Upload className='size-5' />}
        >
          <BackupImportSource
            raw={importText}
            fileName={importFileName}
            originKey={externalOriginKey}
            external={externalImportSource !== null}
            disabled={importMutation.isPending}
            onFile={(file) => void handleImportFile(file)}
            onText={(raw) => {
              resetImportReview()
              setImportText(raw)
              setImportFileName('')
            }}
            onOrigin={(origin) => {
              resetImportReview()
              setExternalOriginKey(origin)
            }}
          />
          <div className='flex items-center justify-end gap-3'>
            <Button
              type='button'
              disabled={
                previewMutation.isPending ||
                !importText.trim() ||
                (externalImportSource !== null && !externalOriginKey.trim())
              }
              onClick={() => void handlePreviewImport()}
            >
              <ArrowRight className='size-4' aria-hidden='true' />
              {previewMutation.isPending
                ? t('settings.common.saving')
                : t('settings.content.importExport.importPreview')}
            </Button>
          </div>
          <BackupImportPreviewPanel
            preview={importPreview}
            acknowledged={allowChannelsOnlyImport}
            onAcknowledge={(checked) => {
              setAllowChannelsOnlyImport(checked)
            }}
          />
          {importPreview ? (
            <div className='flex justify-end'>
              <Button
                type='button'
                disabled={!canImport || importMutation.isPending}
                onClick={() => setConfirmImportOpen(true)}
              >
                <Upload className='size-4' aria-hidden='true' />
                {t('settings.content.importExport.import')}
              </Button>
            </div>
          ) : null}
        </SettingsSubsection>
        <SettingsSubsection
          title={t('settings.content.importExport.exportGroup')}
          description={t('settings.content.importExport.design.exportHint')}
          icon={<Download className='size-5' />}
        >
          <div className='grid gap-3 sm:grid-cols-3'>
            {(
              [
                {
                  type: 'all',
                  label: 'exportAll',
                  detail: 'exportAllHint',
                  icon: Database,
                },
                {
                  type: 'accounts',
                  label: 'exportAccounts',
                  detail: 'exportAccountsHint',
                  icon: Users,
                },
                {
                  type: 'preferences',
                  label: 'exportPreferences',
                  detail: 'exportPreferencesHint',
                  icon: SlidersHorizontal,
                },
              ] as const
            ).map((item) => (
              <Button
                key={item.type}
                variant='outline'
                className='h-auto min-w-0 items-start justify-start gap-3 p-4 text-left whitespace-normal'
                disabled={exportMutation.isPending}
                onClick={() => {
                  setReauthError(null)
                  setReauthTarget({ kind: 'download', type: item.type })
                }}
              >
                <item.icon
                  className='text-primary mt-0.5 size-4 shrink-0'
                  aria-hidden='true'
                />
                <span className='min-w-0 space-y-1'>
                  <span className='block text-sm font-medium'>
                    {t(`settings.content.importExport.${item.label}`)}
                  </span>
                  <span className='text-muted-foreground block text-xs leading-relaxed font-normal'>
                    {t(`settings.content.importExport.design.${item.detail}`)}
                  </span>
                </span>
              </Button>
            ))}
          </div>
        </SettingsSubsection>

        {webdavQuery.isLoading ? (
          <p className='text-muted-foreground text-sm'>
            {t('settings.common.loading')}
          </p>
        ) : null}
        {!webdavQuery.isLoading && (webdavQuery.isError || !config) ? (
          <SectionError
            title={t('settings.content.importExport.webdavGroup')}
            messageKey='settings.common.loadFailed'
            onRetry={() => void webdavQuery.refetch()}
          />
        ) : null}
        {!webdavQuery.isLoading && !webdavQuery.isError && config ? (
          <Form {...form}>
            <form
              id={WEBDAV_FORM_ID}
              onSubmit={form.handleSubmit(onWebdavSubmit)}
              className='bg-muted/20 space-y-4 rounded-xl border p-5'
            >
              <SettingsSubsection
                title={t('settings.content.importExport.webdavGroup')}
                description={t(
                  'settings.content.importExport.design.webdavHint'
                )}
                icon={<Cloud className='size-5' />}
              >
                <FormField
                  control={form.control}
                  name='enabled'
                  render={({ field }) => (
                    <FormItem className='flex flex-row items-center gap-3'>
                      <FormControl>
                        <Switch
                          checked={Boolean(field.value)}
                          onCheckedChange={field.onChange}
                        />
                      </FormControl>
                      <FormLabel className='cursor-pointer'>
                        {t(
                          'settings.content.importExport.fields.webdavEnabled'
                        )}
                      </FormLabel>
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name='fileUrl'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        {t(
                          'settings.content.importExport.fields.webdavFileUrl'
                        )}
                      </FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          value={field.value ?? ''}
                          placeholder='https://dav.example.com/backups/metapi.json'
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <div className='grid grid-cols-1 gap-4 sm:grid-cols-2'>
                  <FormField
                    control={form.control}
                    name='username'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>
                          {t(
                            'settings.content.importExport.fields.webdavUsername'
                          )}
                        </FormLabel>
                        <FormControl>
                          <Input {...field} value={field.value ?? ''} />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                  <FormField
                    control={form.control}
                    name='password'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>
                          {t(
                            'settings.content.importExport.fields.webdavPassword'
                          )}
                        </FormLabel>
                        <FormControl>
                          <Input
                            {...field}
                            value={field.value ?? ''}
                            type='password'
                            placeholder={t(
                              'settings.content.importExport.fields.webdavPasswordHint'
                            )}
                          />
                        </FormControl>
                        <FormDescription>
                          {t(
                            'settings.content.importExport.fields.webdavPasswordDescription'
                          )}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </div>
                <FormField
                  control={form.control}
                  name='exportType'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        {t(
                          'settings.content.importExport.fields.webdavExportType'
                        )}
                      </FormLabel>
                      <Select
                        value={field.value ?? 'all'}
                        onValueChange={field.onChange}
                      >
                        <FormControl>
                          <SelectTrigger>
                            <SelectValue>
                              {(selected) => {
                                const labels: Record<string, string> = {
                                  all: t(
                                    'settings.content.importExport.exportAll'
                                  ),
                                  accounts: t(
                                    'settings.content.importExport.exportAccounts'
                                  ),
                                  preferences: t(
                                    'settings.content.importExport.exportPreferences'
                                  ),
                                }
                                return selected
                                  ? (labels[String(selected)] ??
                                      String(selected))
                                  : ''
                              }}
                            </SelectValue>
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent>
                          <SelectItem value='all'>
                            {t('settings.content.importExport.exportAll')}
                          </SelectItem>
                          <SelectItem value='accounts'>
                            {t('settings.content.importExport.exportAccounts')}
                          </SelectItem>
                          <SelectItem value='preferences'>
                            {t(
                              'settings.content.importExport.exportPreferences'
                            )}
                          </SelectItem>
                        </SelectContent>
                      </Select>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name='autoSyncEnabled'
                  render={({ field }) => (
                    <FormItem className='flex flex-row items-center gap-3'>
                      <FormControl>
                        <Switch
                          checked={Boolean(field.value)}
                          onCheckedChange={field.onChange}
                        />
                      </FormControl>
                      <FormLabel className='cursor-pointer'>
                        {t(
                          'settings.content.importExport.fields.webdavAutoSyncEnabled'
                        )}
                      </FormLabel>
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name='autoSyncSchedule'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        {t(
                          'settings.content.importExport.fields.webdavAutoSyncCron'
                        )}
                      </FormLabel>
                      <FormControl>
                        <ScheduleEditor
                          value={field.value}
                          onChange={field.onChange}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <SettingsFormActions
                  formId={WEBDAV_FORM_ID}
                  isDirty={isWebdavDirty}
                  isPending={saveWebdavMutation.isPending}
                  onReset={() =>
                    syncFromServer(
                      config
                        ? {
                            enabled: config.enabled,
                            fileUrl: config.fileUrl,
                            username: config.username,
                            password: '',
                            exportType: config.exportType,
                            autoSyncEnabled: config.autoSyncEnabled,
                            autoSyncSchedule: scheduleFromLegacy({
                              cron: config.autoSyncCron,
                            }),
                          }
                        : DEFAULT_WEBDAV_VALUES
                    )
                  }
                  saveLabel={t('settings.content.importExport.saveWebdav')}
                />
                <div className='flex flex-wrap gap-2'>
                  <Button
                    type='button'
                    variant='outline'
                    size='sm'
                    disabled={exportWebdavMutation.isPending}
                    onClick={() => {
                      setReauthError(null)
                      setReauthTarget({ kind: 'webdav' })
                    }}
                  >
                    {t('settings.content.importExport.exportToWebdav')}
                  </Button>
                  <Button
                    type='button'
                    variant='outline'
                    size='sm'
                    disabled={importWebdavMutation.isPending}
                    onClick={() => setConfirmWebdavImportOpen(true)}
                  >
                    {t('settings.content.importExport.importFromWebdav')}
                  </Button>
                </div>
                {webdavQuery.data?.state?.lastSyncAt ? (
                  <p className='text-muted-foreground text-xs'>
                    {t('settings.content.importExport.lastSync', {
                      at: webdavQuery.data.state.lastSyncAt,
                    })}
                  </p>
                ) : null}
                {webdavQuery.data?.state?.lastError ? (
                  <p className='text-destructive text-xs'>
                    {t('settings.content.importExport.lastError', {
                      error: webdavQuery.data.state.lastError,
                    })}
                  </p>
                ) : null}
              </SettingsSubsection>
            </form>
          </Form>
        ) : null}
      </div>
      <ReauthDialog
        open={reauthTarget !== null}
        onOpenChange={(open) => {
          if (!open) {
            setReauthTarget(null)
            setReauthError(null)
          }
        }}
        description={t('settings.content.importExport.reauthDescription')}
        errorMessage={reauthError}
        isPending={exportMutation.isPending || exportWebdavMutation.isPending}
        onSubmit={(confirmToken) => {
          if (reauthTarget?.kind === 'download') {
            exportMutation.mutate({
              type: reauthTarget.type,
              confirmToken,
            })
          } else if (reauthTarget?.kind === 'webdav') {
            exportWebdavMutation.mutate({ type: undefined, confirmToken })
          }
        }}
      />
      <FormNavigationGuard enabled={isWebdavDirty} />
      <ConfirmDialog
        key={
          importPreview?.kind === 'octopus' &&
          Object.keys(importPreview.data.notImported ?? {}).length > 0
            ? 'blocked'
            : 'ready'
        }
        open={confirmImportOpen}
        title={t('settings.content.importExport.importConfirmTitle')}
        description={
          t(confirmImportDescriptionKey) +
          (hasExternalRemovals
            ? ` ${t('settings.content.importExport.octopusReplacementDescription')}`
            : '')
        }
        confirmLabel={t('settings.content.importExport.import')}
        cancelLabel={t('settings.common.cancel')}
        destructive
        onConfirm={() => {
          setConfirmImportOpen(false)
          if (!previewSnapshot) return
          importMutation.mutate({
            raw: previewSnapshot.raw,
            originKey: previewSnapshot.originKey,
            source: previewSnapshot.source,
            channelsOnly: allowChannelsOnlyImport,
            replaceOrigin: hasExternalRemovals,
          })
        }}
        onCancel={() => setConfirmImportOpen(false)}
      />
      <ConfirmDialog
        open={confirmWebdavImportOpen}
        title={t('settings.content.importExport.webdavImportConfirmTitle')}
        description={t(
          'settings.content.importExport.webdavImportConfirmDescription'
        )}
        confirmLabel={t('settings.content.importExport.importFromWebdav')}
        cancelLabel={t('settings.common.cancel')}
        destructive
        onConfirm={() => {
          setConfirmWebdavImportOpen(false)
          importWebdavMutation.mutate()
        }}
        onCancel={() => setConfirmWebdavImportOpen(false)}
      />
    </SectionCard>
  )
}
