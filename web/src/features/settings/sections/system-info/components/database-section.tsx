// metapi-go/features/settings/sections/system-info/components — database
// section. Runtime DB selection is restart-pending configuration; the data
// migration action lives in DatabaseMigrationSection, rendered as its own
// card below this one.

import { useMutation, useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

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
import { api } from '@/lib/api'
import { toast } from '@/lib/toast'

import { FormNavigationGuard } from '../../../components/form-navigation-guard'
import { SettingsFormActions } from '../../../components/settings-form-actions'
import {
  SettingsSectionCard,
  SettingsSectionSkeleton,
} from '../../../components/settings-section-card'
import { SettingsSectionError } from '../../../components/settings-section-error'
import { useSettingsForm } from '../../../hooks/use-settings-form'
import {
  collectChangedFields,
  hasChanges,
} from '../../../lib/collect-changed-fields'
import { DatabaseMigrationSection } from './database-migration-section'
import {
  runtimeDatabaseQueryKeys,
  type RuntimeDatabaseConfig,
} from './database-shared'

const SAVE_FORM_ID = 'settings-system-info-database-form'

const databaseSchema = z.object({
  dialect: z.enum(['sqlite', 'postgres']),
  connectionString: z.string(),
  ssl: z.boolean(),
})

type DatabaseFormValues = z.infer<typeof databaseSchema>

const DEFAULT_VALUES: DatabaseFormValues = {
  dialect: 'sqlite',
  connectionString: '',
  ssl: false,
}

function deriveServerValues(
  data: RuntimeDatabaseConfig | undefined
): DatabaseFormValues | null {
  if (!data) return null
  if (!data.saved) return DEFAULT_VALUES
  return {
    dialect: data.saved.dialect,
    connectionString: '',
    ssl: Boolean(data.saved.ssl),
  }
}

export function DatabaseSection() {
  const { t } = useTranslation()

  const configQuery = useQuery<RuntimeDatabaseConfig>({
    queryKey: runtimeDatabaseQueryKeys.all,
    queryFn: async () =>
      (await api.getRuntimeDatabaseConfig()) as RuntimeDatabaseConfig,
    staleTime: 30 * 1000,
  })

  const serverValues = deriveServerValues(configQuery.data)
  const { form, baseline, syncFromServer } =
    useSettingsForm<DatabaseFormValues>({
      schema: databaseSchema,
      defaultValues: DEFAULT_VALUES,
      serverValues,
    })

  const dialect = form.watch('dialect')
  const savedConfig = configQuery.data?.saved
  const isDirty = form.formState.isDirty
  let connectionPlaceholder = './data/target.db'
  if (savedConfig?.hasConnectionString) {
    connectionPlaceholder =
      savedConfig.connectionStringMasked ||
      t('settings.systemInfo.database.fields.connectionSaved')
  } else if (dialect === 'postgres') {
    connectionPlaceholder = 'postgres://user:pass@host:5432/db'
  }

  const testMutation = useMutation({
    mutationFn: async (values: DatabaseFormValues) =>
      api.testExternalDatabaseConnection({
        dialect: values.dialect,
        connectionString: values.connectionString,
        ssl: values.ssl,
      }),
    onSuccess: () =>
      toast.success(t('settings.systemInfo.database.toast.testOk')),
    onError: () =>
      toast.error(t('settings.systemInfo.database.toast.testFailed')),
  })

  const saveMutation = useMutation({
    mutationFn: async (values: Partial<DatabaseFormValues>) =>
      api.updateRuntimeDatabaseConfig(values),
    onSuccess: (response) => {
      const saved = (response as RuntimeDatabaseConfig).saved
      if (saved) {
        syncFromServer({
          dialect: saved.dialect,
          connectionString: '',
          ssl: Boolean(saved.ssl),
        })
      }
      void configQuery.refetch()
      toast.success(t('settings.systemInfo.database.toast.saved'))
    },
    onError: () =>
      toast.error(t('settings.systemInfo.database.toast.saveFailed')),
  })

  function requireConnection(messageKey: string): string | null {
    const connection = form.getValues('connectionString').trim()
    if (connection) return connection
    form.setError('connectionString', { message: messageKey })
    return null
  }

  function onSave(values: DatabaseFormValues) {
    const changed = collectChangedFields(
      values as unknown as Record<string, unknown>,
      baseline as unknown as Record<string, unknown> | null
    ) as Partial<DatabaseFormValues>
    const connection = values.connectionString.trim()
    const requiresConnection =
      !savedConfig?.hasConnectionString || changed.dialect !== undefined
    if (requiresConnection && !connection) {
      requireConnection(
        'settings.systemInfo.database.schema.connectionRequired'
      )
      return
    }
    if (connection) {
      changed.connectionString = connection
    } else {
      delete changed.connectionString
    }
    if (!hasChanges(changed)) {
      toast.info(t('settings.common.noChanges'))
      return
    }
    saveMutation.mutate(changed)
  }

  const testConnection = form.handleSubmit((values) => {
    const connection = requireConnection(
      'settings.systemInfo.database.schema.testConnectionRequired'
    )
    if (!connection) return
    testMutation.mutate({ ...values, connectionString: connection })
  })

  if (configQuery.isLoading) {
    return (
      <>
        <SettingsSectionSkeleton />
        <DatabaseMigrationSection />
      </>
    )
  }
  if (configQuery.isError || !configQuery.data) {
    return (
      <>
        <SettingsSectionError
          title={t('settings.systemInfo.database.title')}
          onRetry={() => void configQuery.refetch()}
        />
        <DatabaseMigrationSection />
      </>
    )
  }

  const active = configQuery.data.active

  return (
    <>
      <SettingsSectionCard
        title={t('settings.systemInfo.database.title')}
        description={t('settings.systemInfo.database.description')}
      >
        <div className='space-y-4'>
          {active ? (
            <div className='bg-muted/25 rounded-lg border p-3'>
              <p className='text-muted-foreground text-xs font-medium'>
                {t('settings.systemInfo.database.currentRuntime')}
              </p>
              <code className='text-foreground mt-1 block text-xs break-all'>
                {active.dialect} · {active.connection}
                {active.ssl ? ' · SSL' : ''}
              </code>
            </div>
          ) : null}
          {configQuery.data.restartRequired ? (
            <div className='border-warning/35 bg-warning/10 text-warning-soft-fg rounded-lg border px-3 py-2 text-sm'>
              {t('settings.systemInfo.database.restartRequired')}
            </div>
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
                    <FormLabel>
                      {t('settings.systemInfo.database.fields.dialect')}
                    </FormLabel>
                    <Select
                      value={field.value}
                      onValueChange={(value) => {
                        field.onChange(value)
                        if (value === 'sqlite') {
                          form.setValue('ssl', false, { shouldDirty: true })
                        }
                      }}
                    >
                      <FormControl>
                        <SelectTrigger>
                          <SelectValue>
                            {(selected) => {
                              if (selected === 'sqlite') return 'SQLite'
                              if (selected === 'postgres') return 'PostgreSQL'
                              return ''
                            }}
                          </SelectValue>
                        </SelectTrigger>
                      </FormControl>
                      <SelectContent>
                        <SelectItem value='sqlite'>SQLite</SelectItem>
                        <SelectItem value='postgres'>PostgreSQL</SelectItem>
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
                      {t(
                        'settings.systemInfo.database.fields.connectionString'
                      )}
                    </FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        value={field.value ?? ''}
                        className='font-mono'
                        autoComplete='off'
                        spellCheck={false}
                        placeholder={connectionPlaceholder}
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        savedConfig?.hasConnectionString
                          ? 'settings.systemInfo.database.fields.connectionSavedHint'
                          : 'settings.systemInfo.database.fields.connectionStringHint'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
              {dialect === 'postgres' ? (
                <FormField
                  control={form.control}
                  name='ssl'
                  render={({ field }) => (
                    <FormItem className='flex flex-row items-start gap-3 rounded-lg border p-3'>
                      <FormControl>
                        <Checkbox
                          checked={field.value}
                          onCheckedChange={field.onChange}
                        />
                      </FormControl>
                      <div className='space-y-1'>
                        <FormLabel className='cursor-pointer'>
                          {t('settings.systemInfo.database.fields.ssl')}
                        </FormLabel>
                        <FormDescription>
                          {t('settings.systemInfo.database.fields.sslHint')}
                        </FormDescription>
                      </div>
                    </FormItem>
                  )}
                />
              ) : null}

              <div className='flex flex-col gap-3 border-t pt-4 sm:flex-row sm:items-center sm:justify-between'>
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  disabled={testMutation.isPending}
                  onClick={() => void testConnection()}
                >
                  {testMutation.isPending
                    ? t('settings.common.testing')
                    : t('settings.systemInfo.database.testConnection')}
                </Button>
                <SettingsFormActions
                  formId={SAVE_FORM_ID}
                  isDirty={isDirty}
                  isPending={saveMutation.isPending}
                  onReset={() => syncFromServer(serverValues ?? DEFAULT_VALUES)}
                  saveLabel={t('settings.systemInfo.database.saveAsRuntime')}
                />
              </div>
            </form>
          </Form>
        </div>
        <FormNavigationGuard enabled={isDirty} />
      </SettingsSectionCard>
      <DatabaseMigrationSection />
    </>
  )
}
