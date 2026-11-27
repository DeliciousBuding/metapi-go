import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Cloud } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { useReauthReplay } from '@/components/common/reauth-provider'
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
import { api, type BackupWebdavResponse } from '@/lib/api'
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

const WEBDAV_FORM_ID = 'settings-content-import-export-webdav-form'
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

export function WebdavBackupSection({
  onExport,
  exportPending,
}: {
  onExport: () => void
  exportPending: boolean
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const handleReauth = useReauthReplay()
  const [confirmWebdavImportOpen, setConfirmWebdavImportOpen] = useState(false)
  const webdavQuery = useQuery<BackupWebdavResponse>({
    queryKey: webdavQueryKeys.all,
    queryFn: async () => api.getBackupWebdavConfig(),
    staleTime: 30 * 1000,
  })

  const config = webdavQuery.data?.config

  const serverValues = config
    ? {
        enabled: config.enabled,
        fileUrl: config.fileUrl,
        username: config.username,
        password: '',
        exportType: config.exportType,
        autoSyncEnabled: config.autoSyncEnabled,
        autoSyncSchedule: scheduleFromLegacy({ cron: config.autoSyncCron }),
      }
    : null
  const { form, baseline, syncFromServer } = useSettingsForm<WebdavFormValues>({
    schema: webdavSchema,
    defaultValues: DEFAULT_WEBDAV_VALUES,
    serverValues,
  })

  const saveWebdavMutation = useMutation({
    mutationFn: async (
      input: Partial<WebdavFormValues> & { confirmToken?: string }
    ) => {
      const { confirmToken, ...values } = input
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
      return confirmToken
        ? api.saveBackupWebdavConfig(payload, confirmToken)
        : api.saveBackupWebdavConfig(payload)
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: webdavQueryKeys.all })
      toast.success(t('settings.content.importExport.toast.webdavSaved'))
    },
    onError: (error, input) => {
      if (
        handleReauth(
          error,
          t('reauth.operations.backupWebdavSave'),
          (confirmToken) =>
            saveWebdavMutation.mutateAsync({ ...input, confirmToken })
        )
      ) {
        return
      }
      toast.error(t('settings.content.importExport.toast.webdavSaveFailed'))
    },
  })

  const importWebdavMutation = useMutation({
    mutationFn: async (input: { confirmToken?: string }) =>
      input.confirmToken
        ? api.importBackupFromWebdav(input.confirmToken)
        : api.importBackupFromWebdav(),
    onSuccess: () => {
      toast.success(t('settings.content.importExport.toast.webdavImported'))
      void queryClient.invalidateQueries()
    },
    onError: (error, input) => {
      if (
        handleReauth(
          error,
          t('reauth.operations.backupWebdavImport'),
          (confirmToken) =>
            importWebdavMutation.mutateAsync({ ...input, confirmToken })
        )
      ) {
        return
      }
      toast.error(t('settings.content.importExport.toast.webdavImportFailed'))
    },
  })

  function onWebdavSubmit(values: WebdavFormValues) {
    const changed = collectChangedFields(values, baseline)
    if (!hasChanges(changed)) {
      toast.info(t('settings.common.noChanges'))
      return
    }
    saveWebdavMutation.mutate(changed)
  }

  const isWebdavDirty = form.formState.isDirty
  return (
    <>
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
              description={t('settings.content.importExport.design.webdavHint')}
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
                  syncFromServer(serverValues ?? DEFAULT_WEBDAV_VALUES)
                }
                saveLabel={t('settings.content.importExport.saveWebdav')}
              />
              <div className='flex flex-wrap gap-2'>
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  disabled={exportPending}
                  onClick={onExport}
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
      <FormNavigationGuard enabled={isWebdavDirty} />
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
          importWebdavMutation.mutate({})
        }}
        onCancel={() => setConfirmWebdavImportOpen(false)}
      />
    </>
  )
}
