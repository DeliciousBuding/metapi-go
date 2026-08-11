// metapi-go/features/settings/sections/content/components — import/export
// section. Three export types, JSON paste import with preview, and a WebDAV
// auto-backup config form. Mirrors the legacy ImportExport page, trimmed to
// the actionable surfaces.

import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { api,type BackupWebdavResponse } from '@/lib/api'

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
  SettingsSectionCard,
  SettingsSectionSkeleton,
} from '../../../components/settings-section-card'

const WEBDAV_FORM_ID = 'settings-content-import-export-webdav-form'

const webdavSchema = z.object({
  enabled: z.boolean().optional(),
  fileUrl: z.string().optional(),
  username: z.string().optional(),
  password: z.string().optional(),
  clearPassword: z.boolean().optional(),
  exportType: z.enum(['all', 'accounts', 'preferences']).optional(),
  autoSyncEnabled: z.boolean().optional(),
  autoSyncCron: z.string().optional(),
})

type WebdavFormValues = z.infer<typeof webdavSchema>

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

  const webdavQuery = useQuery<BackupWebdavResponse>({
    queryKey: webdavQueryKeys.all,
    queryFn: async () => api.getBackupWebdavConfig(),
    staleTime: 30 * 1000,
  })

  const form = useForm<WebdavFormValues>({
    resolver: zodResolver(webdavSchema) as never,
    defaultValues: {
      enabled: false,
      fileUrl: '',
      username: '',
      password: '',
      clearPassword: false,
      exportType: 'all',
      autoSyncEnabled: false,
      autoSyncCron: '',
    },
  })

  useEffect(() => {
    const config = webdavQuery.data?.config
    if (!config) {
      return
    }
    form.reset(
      {
        enabled: config.enabled,
        fileUrl: config.fileUrl,
        username: config.username,
        password: '',
        clearPassword: false,
        exportType: config.exportType,
        autoSyncEnabled: config.autoSyncEnabled,
        autoSyncCron: config.autoSyncCron,
      },
      { keepDirtyValues: true },
    )
  }, [webdavQuery.data, form])

  const exportMutation = useMutation({
    mutationFn: async (type: 'all' | 'accounts' | 'preferences') => {
      const text = await (async () => {
        const response = await fetch(
          `/api/settings/backup/export?type=${encodeURIComponent(type)}`,
        )
        return response.text()
      })()
      return { type, text }
    },
    onSuccess: ({ type, text }) => {
      const today = new Date().toISOString().slice(0, 10)
      downloadTextFile(`metapi-${type}-${today}.json`, text)
      toast.success(t('settings.content.importExport.toast.exported'))
    },
    onError: () => toast.error(t('settings.content.importExport.toast.exportFailed')),
  })

  const importMutation = useMutation({
    mutationFn: async (raw: string) => {
      const data = JSON.parse(raw) as unknown
      await api.previewBackupImport(data)
      return api.importBackup(data)
    },
    onSuccess: () => {
      toast.success(t('settings.content.importExport.toast.imported'))
      setImportText('')
    },
    onError: () => toast.error(t('settings.content.importExport.toast.importFailed')),
  })

  const saveWebdavMutation = useMutation({
    mutationFn: async (values: WebdavFormValues) =>
      api.saveBackupWebdavConfig({
        enabled: Boolean(values.enabled),
        fileUrl: values.fileUrl ?? '',
        username: values.username ?? '',
        password: values.password || undefined,
        clearPassword: Boolean(values.clearPassword),
        exportType: values.exportType ?? 'all',
        autoSyncEnabled: Boolean(values.autoSyncEnabled),
        autoSyncCron: values.autoSyncCron ?? '',
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: webdavQueryKeys.all })
      toast.success(t('settings.content.importExport.toast.webdavSaved'))
    },
    onError: () => toast.error(t('settings.content.importExport.toast.webdavSaveFailed')),
  })

  const exportWebdavMutation = useMutation({
    mutationFn: async (type?: 'all' | 'accounts' | 'preferences') =>
      api.exportBackupToWebdav(type),
    onSuccess: () => toast.success(t('settings.content.importExport.toast.webdavExported')),
    onError: () => toast.error(t('settings.content.importExport.toast.webdavExportFailed')),
  })

  const importWebdavMutation = useMutation({
    mutationFn: async () => api.importBackupFromWebdav(),
    onSuccess: () => toast.success(t('settings.content.importExport.toast.webdavImported')),
    onError: () => toast.error(t('settings.content.importExport.toast.webdavImportFailed')),
  })

  function onWebdavSubmit(values: WebdavFormValues) {
    saveWebdavMutation.mutate(values)
  }

  if (webdavQuery.isLoading) {
    return <SettingsSectionSkeleton />
  }

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
              disabled={importMutation.isPending || !importText.trim()}
              onClick={() => importMutation.mutate(importText)}
            >
              {importMutation.isPending
                ? t('settings.common.saving')
                : t('settings.content.importExport.import')}
            </Button>
          </div>
        </div>

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
            <div className='grid grid-cols-2 gap-4'>
              <FormField
                control={form.control}
                name='username'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      {t('settings.content.importExport.fields.webdavUsername')}
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
                      {t('settings.content.importExport.fields.webdavPassword')}
                    </FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        value={field.value ?? ''}
                        type='password'
                        placeholder={t('settings.content.importExport.fields.webdavPasswordHint')}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('settings.content.importExport.fields.webdavPasswordDescription')}
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
                    {t('settings.content.importExport.fields.webdavExportType')}
                  </FormLabel>
                  <Select
                    value={field.value ?? 'all'}
                    onValueChange={field.onChange}
                  >
                    <FormControl>
                      <SelectTrigger>
                        <SelectValue />
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
                    {t('settings.content.importExport.fields.webdavAutoSyncEnabled')}
                  </FormLabel>
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='autoSyncCron'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {t('settings.content.importExport.fields.webdavAutoSyncCron')}
                  </FormLabel>
                  <FormControl>
                    <Input {...field} value={field.value ?? ''} placeholder='0 */6 * * *' />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <div className='flex flex-wrap gap-2'>
              <Button
                type='submit'
                form={WEBDAV_FORM_ID}
                size='sm'
                disabled={saveWebdavMutation.isPending}
              >
                {saveWebdavMutation.isPending
                  ? t('settings.common.saving')
                  : t('settings.content.importExport.saveWebdav')}
              </Button>
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
              <p className='text-xs text-muted-foreground'>
                {t('settings.content.importExport.lastSync', {
                  at: webdavQuery.data.state.lastSyncAt,
                })}
              </p>
            ) : null}
            {webdavQuery.data?.state?.lastError ? (
              <p className='text-xs text-destructive'>
                {t('settings.content.importExport.lastError', {
                  error: webdavQuery.data.state.lastError,
                })}
              </p>
            ) : null}
          </form>
        </Form>
      </div>
    </SettingsSectionCard>
  )
}
