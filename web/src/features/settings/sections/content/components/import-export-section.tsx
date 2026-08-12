// metapi-go/features/settings/sections/content/components — import/export
// section. Three export types (now checked for non-OK responses), JSON paste
// import with a preview + confirmation step, and a WebDAV auto-backup config
// form using the shared semantic schedule editor.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { ConfirmDialog } from '@/components/common/confirm-dialog'
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
import { Textarea } from '@/components/ui/textarea'
import {
  api,
  type BackupWebdavExportType,
  type BackupWebdavResponse,
} from '@/lib/api'

import { FormNavigationGuard } from '../../../components/form-navigation-guard'
import { ScheduleEditor } from '../../../components/schedule-editor'
import { SettingsFormActions } from '../../../components/settings-form-actions'
import { SettingsSectionCard } from '../../../components/settings-section-card'
import { SettingsSectionError } from '../../../components/settings-section-error'
import { useSettingsForm } from '../../../hooks/use-settings-form'
import {
  collectChangedFields,
  hasChanges,
} from '../../../lib/collect-changed-fields'
import { scheduleFromLegacy, scheduleToCron } from '../../../lib/schedule'

const WEBDAV_FORM_ID = 'settings-content-import-export-webdav-form'

type BackupImportPlan = Record<
  string,
  { rows: number; toInsert: number; duplicates: number; skippedRows: number }
>

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
  const [importPlan, setImportPlan] = useState<BackupImportPlan | null>(null)
  const [confirmImportOpen, setConfirmImportOpen] = useState(false)

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
    mutationFn: async (type: BackupWebdavExportType) => {
      const text = await api.exportBackupRaw(type)
      return { type, text }
    },
    onSuccess: ({ type, text }) => {
      const today = new Date().toISOString().slice(0, 10)
      downloadTextFile(`metapi-${type}-${today}.json`, text)
      toast.success(t('settings.content.importExport.toast.exported'))
    },
    onError: () =>
      toast.error(t('settings.content.importExport.toast.exportFailed')),
  })

  const previewMutation = useMutation({
    mutationFn: async (raw: string) => {
      const data = JSON.parse(raw) as unknown
      const result = (await api.previewBackupImport(data)) as {
        success?: boolean
        plan?: BackupImportPlan
      }
      return result.plan ?? {}
    },
  })

  const importMutation = useMutation({
    mutationFn: async (raw: string) => {
      const data = JSON.parse(raw) as unknown
      return api.importBackup(data)
    },
    onSuccess: () => {
      toast.success(t('settings.content.importExport.toast.imported'))
      setImportText('')
      setImportPlan(null)
    },
    onError: () =>
      toast.error(t('settings.content.importExport.toast.importFailed')),
  })

  async function handlePreviewImport() {
    try {
      const plan = await previewMutation.mutateAsync(importText)
      setImportPlan(plan)
      setConfirmImportOpen(true)
    } catch {
      toast.error(t('settings.content.importExport.toast.importFailed'))
    }
  }

  const saveWebdavMutation = useMutation({
    mutationFn: async (values: Partial<WebdavFormValues>) => {
      const payload: Parameters<typeof api.saveBackupWebdavConfig>[0] = {}
      if (values.enabled !== undefined) payload.enabled = values.enabled
      if (values.fileUrl !== undefined) payload.fileUrl = values.fileUrl ?? ''
      if (values.username !== undefined)
        payload.username = values.username ?? ''
      if (values.password) payload.password = values.password
      if (values.exportType !== undefined)
        payload.exportType = values.exportType
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
    mutationFn: async (type?: BackupWebdavExportType) =>
      api.exportBackupToWebdav(type),
    onSuccess: () =>
      toast.success(t('settings.content.importExport.toast.webdavExported')),
    onError: () =>
      toast.error(t('settings.content.importExport.toast.webdavExportFailed')),
  })

  const importWebdavMutation = useMutation({
    mutationFn: async () => api.importBackupFromWebdav(),
    onSuccess: () =>
      toast.success(t('settings.content.importExport.toast.webdavImported')),
    onError: () =>
      toast.error(t('settings.content.importExport.toast.webdavImportFailed')),
  })

  function onWebdavSubmit(values: WebdavFormValues) {
    const changed = collectChangedFields(
      values as unknown as Record<string, unknown>,
      baseline as unknown as Record<string, unknown> | null
    ) as Partial<WebdavFormValues>
    if (!hasChanges(changed)) {
      toast.info(t('settings.common.noChanges'))
      return
    }
    saveWebdavMutation.mutate(changed)
  }

  const planEntries = importPlan ? Object.entries(importPlan) : []
  const isWebdavDirty = form.formState.isDirty

  return (
    <SettingsSectionCard
      title={t('settings.content.importExport.title')}
      description={t('settings.content.importExport.description')}
    >
      <div className='space-y-6'>
        <div className='space-y-3'>
          <h4 className='text-sm font-medium'>
            {t('settings.content.importExport.exportGroup')}
          </h4>
          <div className='flex flex-wrap gap-2'>
            <Button
              type='button'
              variant='outline'
              size='sm'
              disabled={exportMutation.isPending}
              onClick={() => exportMutation.mutate('all')}
            >
              {t('settings.content.importExport.exportAll')}
            </Button>
            <Button
              type='button'
              variant='outline'
              size='sm'
              disabled={exportMutation.isPending}
              onClick={() => exportMutation.mutate('accounts')}
            >
              {t('settings.content.importExport.exportAccounts')}
            </Button>
            <Button
              type='button'
              variant='outline'
              size='sm'
              disabled={exportMutation.isPending}
              onClick={() => exportMutation.mutate('preferences')}
            >
              {t('settings.content.importExport.exportPreferences')}
            </Button>
          </div>
        </div>

        <div className='space-y-3'>
          <h4 className='text-sm font-medium'>
            {t('settings.content.importExport.importGroup')}
          </h4>
          <Textarea
            value={importText}
            onChange={(event) => setImportText(event.target.value)}
            rows={8}
            placeholder='{ "version": "..." }'
            className='font-mono text-xs'
          />
          <div className='flex gap-2'>
            <Button
              type='button'
              variant='outline'
              size='sm'
              disabled={previewMutation.isPending || !importText.trim()}
              onClick={() => void handlePreviewImport()}
            >
              {previewMutation.isPending
                ? t('settings.common.saving')
                : t('settings.content.importExport.importPreview')}
            </Button>
          </div>
        </div>

        {webdavQuery.isLoading ? (
          <p className='text-muted-foreground text-sm'>
            {t('settings.common.loading')}
          </p>
        ) : null}
        {!webdavQuery.isLoading && (webdavQuery.isError || !config) ? (
          <SettingsSectionError
            title={t('settings.content.importExport.webdavGroup')}
            onRetry={() => void webdavQuery.refetch()}
          />
        ) : null}
        {!webdavQuery.isLoading && !webdavQuery.isError && config ? (
          <Form {...form}>
            <form
              id={WEBDAV_FORM_ID}
              onSubmit={form.handleSubmit(onWebdavSubmit)}
              className='space-y-4 rounded-lg border p-4'
            >
              <h4 className='text-sm font-medium'>
                {t('settings.content.importExport.webdavGroup')}
              </h4>
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
                      {t('settings.content.importExport.fields.webdavEnabled')}
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
                      {t('settings.content.importExport.fields.webdavFileUrl')}
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
                                ? (labels[String(selected)] ?? String(selected))
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
                          {t('settings.content.importExport.exportPreferences')}
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
                  onClick={() => exportWebdavMutation.mutate(undefined)}
                >
                  {t('settings.content.importExport.exportToWebdav')}
                </Button>
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  disabled={importWebdavMutation.isPending}
                  onClick={() => importWebdavMutation.mutate()}
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
            </form>
          </Form>
        ) : null}
      </div>
      <FormNavigationGuard enabled={isWebdavDirty} />
      <ConfirmDialog
        open={confirmImportOpen}
        title={t('settings.content.importExport.importConfirmTitle')}
        description={t(
          'settings.content.importExport.importConfirmDescription'
        )}
        confirmLabel={t('settings.content.importExport.import')}
        cancelLabel={t('settings.common.cancel')}
        destructive
        onConfirm={() => {
          setConfirmImportOpen(false)
          importMutation.mutate(importText)
        }}
        onCancel={() => setConfirmImportOpen(false)}
      />
      {importPlan && planEntries.length > 0 ? (
        <div className='mt-4 space-y-2 rounded-lg border p-4'>
          <h4 className='text-sm font-medium'>
            {t('settings.content.importExport.importPreviewTitle')}
          </h4>
          <ul className='text-muted-foreground list-inside list-disc space-y-1 text-xs'>
            {planEntries.map(([table, plan]) => (
              <li key={table}>
                {t('settings.content.importExport.importPreviewRow', {
                  table,
                  toInsert: plan.toInsert,
                  duplicates: plan.duplicates,
                  skipped: plan.skippedRows,
                })}
              </li>
            ))}
          </ul>
        </div>
      ) : null}
    </SettingsSectionCard>
  )
}
