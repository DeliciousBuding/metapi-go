// metapi-go/features/settings/sections/system-info/components — database
// section. Runtime DB dialect (sqlite|postgres) + connection string + ssl
// toggle + test-connection / migrate / save-as-runtime buttons. The migrate
// action is present in api.ts but the Go runtime disables it in-app; we
// still expose it so the user sees the toast from the server rather than a
// silent no-op.

import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { api } from '@/lib/api'

import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
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
import {
  SettingsSectionCard,
  SettingsSectionSkeleton,
} from '../../../components/settings-section-card'

const SAVE_FORM_ID = 'settings-system-info-database-form'

const databaseSchema = z.object({
  dialect: z.enum(['sqlite', 'postgres']),
  connectionString: z.string().min(1, 'settings.systemInfo.database.schema.connectionRequired'),
  ssl: z.boolean().optional(),
  overwrite: z.boolean().optional(),
})

type DatabaseFormValues = z.infer<typeof databaseSchema>

type RuntimeDatabaseConfig = {
  active?: { dialect: string; connection: string; ssl?: boolean }
  saved?: {
    dialect: 'sqlite' | 'postgres'
    connectionString: string
    ssl?: boolean
  }
  restartRequired?: boolean
}

const runtimeDatabaseQueryKeys = {
  all: ['runtime-database-config'] as const,
}

export function DatabaseSection() {
  const { t } = useTranslation()
  const [confirmMigrateOpen, setConfirmMigrateOpen] = useState(false)

  const configQuery = useQuery<RuntimeDatabaseConfig>({
    queryKey: runtimeDatabaseQueryKeys.all,
    queryFn: async () => (await api.getRuntimeDatabaseConfig()) as RuntimeDatabaseConfig,
    staleTime: 30 * 1000,
  })

  const form = useForm<DatabaseFormValues>({
    resolver: zodResolver(databaseSchema) as never,
    defaultValues: {
      dialect: 'sqlite',
      connectionString: '',
      ssl: false,
      overwrite: false,
    },
  })

  useEffect(() => {
    const saved = configQuery.data?.saved
    if (!saved) {
      return
    }
    form.reset(
      {
        dialect: saved.dialect,
        connectionString: saved.connectionString,
        ssl: Boolean(saved.ssl),
        overwrite: false,
      },
      { keepDirtyValues: true },
    )
  }, [configQuery.data, form])

  const dialect = form.watch('dialect')

  const testMutation = useMutation({
    mutationFn: async (values: DatabaseFormValues) =>
      api.testExternalDatabaseConnection({
        dialect: values.dialect,
        connectionString: values.connectionString,
        ssl: Boolean(values.ssl),
      }),
    onSuccess: () => toast.success(t('settings.systemInfo.database.toast.testOk')),
    onError: () => toast.error(t('settings.systemInfo.database.toast.testFailed')),
  })

  const migrateMutation = useMutation({
    mutationFn: async (values: DatabaseFormValues) =>
      api.migrateExternalDatabase({
        dialect: values.dialect,
        connectionString: values.connectionString,
        overwrite: Boolean(values.overwrite),
        ssl: Boolean(values.ssl),
      }),
    onSuccess: () => {
      toast.success(t('settings.systemInfo.database.toast.migrated'))
      setConfirmMigrateOpen(false)
    },
    onError: () => {
      toast.error(t('settings.systemInfo.database.toast.migrateFailed'))
      setConfirmMigrateOpen(false)
    },
  })

  const saveMutation = useMutation({
    mutationFn: async (values: DatabaseFormValues) =>
      api.updateRuntimeDatabaseConfig({
        dialect: values.dialect,
        connectionString: values.connectionString,
        ssl: Boolean(values.ssl),
      }),
    onSuccess: () => toast.success(t('settings.systemInfo.database.toast.saved')),
    onError: () => toast.error(t('settings.systemInfo.database.toast.saveFailed')),
  })

  function onSave(values: DatabaseFormValues) {
    saveMutation.mutate(values)
  }

  if (configQuery.isLoading) {
    return <SettingsSectionSkeleton />
  }

  const active = configQuery.data?.active

  return (
    <SettingsSectionCard
      title={t('settings.systemInfo.database.title')}
      description={t('settings.systemInfo.database.description')}
    >
      <div className='space-y-4'>
        {active ? (
          <div className='text-xs text-muted-foreground'>
            <span>{t('settings.systemInfo.database.currentRuntime')}</span>
            <code className='ml-2 rounded bg-muted px-2 py-0.5'>
              {active.dialect} · {active.connection}
              {active.ssl ? ' · SSL' : ''}
            </code>
          </div>
        ) : null}
        {configQuery.data?.restartRequired ? (
          <p className='text-xs text-warning'>
            {t('settings.systemInfo.database.restartRequired')}
          </p>
        ) : null}

        <Form {...form}>
          <form
            id={SAVE_FORM_ID}
            onSubmit={form.handleSubmit(onSave)}
            className='space-y-4'
          >
            <FormField
              control={form.control}
              name='dialect'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('settings.systemInfo.database.fields.dialect')}</FormLabel>
                  <Select value={field.value} onValueChange={field.onChange}>
                    <FormControl>
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent>
                      <SelectItem value='sqlite'>sqlite</SelectItem>
                      <SelectItem value='postgres'>postgres</SelectItem>
                    </SelectContent>
                  </Select>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='connectionString'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {t('settings.systemInfo.database.fields.connectionString')}
                  </FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      value={field.value ?? ''}
                      className='font-mono'
                      placeholder={
                        dialect === 'postgres'
                          ? 'postgres://user:pass@host:5432/db'
                          : './data/target.db'
                      }
                    />
                  </FormControl>
                  <FormDescription>
                    {t('settings.systemInfo.database.fields.connectionStringHint')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='ssl'
              render={({ field }) => (
                <FormItem className='flex flex-row items-center gap-3'>
                  <FormControl>
                    <Checkbox
                      checked={Boolean(field.value)}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                  <FormLabel className='cursor-pointer'>
                    {t('settings.systemInfo.database.fields.ssl')}
                  </FormLabel>
                  <FormDescription>
                    {t('settings.systemInfo.database.fields.sslHint')}
                  </FormDescription>
                </FormItem>
              )}
            />

            <div className='flex flex-wrap gap-2'>
              <Button
                type='button'
                variant='outline'
                size='sm'
                disabled={testMutation.isPending}
                onClick={() => testMutation.mutate(form.getValues())}
              >
                {testMutation.isPending
                  ? t('settings.common.testing')
                  : t('settings.systemInfo.database.testConnection')}
              </Button>
              <Button
                type='button'
                variant='outline'
                size='sm'
                disabled={migrateMutation.isPending}
                onClick={() => setConfirmMigrateOpen(true)}
              >
                {t('settings.systemInfo.database.migrate')}
              </Button>
              <Button
                type='submit'
                form={SAVE_FORM_ID}
                size='sm'
                disabled={saveMutation.isPending}
              >
                {saveMutation.isPending
                  ? t('settings.common.saving')
                  : t('settings.systemInfo.database.saveAsRuntime')}
              </Button>
            </div>
          </form>
        </Form>

        {confirmMigrateOpen ? (
          <div className='space-y-3 rounded-lg border border-destructive/40 bg-destructive/5 p-4'>
            <h4 className='text-sm font-medium'>
              {t('settings.systemInfo.database.migrateConfirmTitle')}
            </h4>
            <FormField
              control={form.control}
              name='overwrite'
              render={({ field }) => (
                <FormItem className='flex flex-row items-center gap-3'>
                  <FormControl>
                    <Checkbox
                      checked={Boolean(field.value)}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                  <FormLabel className='cursor-pointer'>
                    {t('settings.systemInfo.database.fields.overwrite')}
                  </FormLabel>
                </FormItem>
              )}
            />
            <div className='flex gap-2'>
              <Button
                type='button'
                variant='destructive'
                size='sm'
                disabled={migrateMutation.isPending}
                onClick={() => migrateMutation.mutate(form.getValues())}
              >
                {migrateMutation.isPending
                  ? t('settings.common.saving')
                  : t('settings.systemInfo.database.migrateConfirm')}
              </Button>
              <Button
                type='button'
                variant='outline'
                size='sm'
                onClick={() => setConfirmMigrateOpen(false)}
              >
                {t('settings.common.cancel')}
              </Button>
            </div>
          </div>
        ) : null}
      </div>
    </SettingsSectionCard>
  )
}
