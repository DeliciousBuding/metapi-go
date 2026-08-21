// metapi-go/features/settings/sections/system-info/components — database
// migration section. Queues a copy of the live runtime database onto an
// external sqlite/postgres target as an admin background task and polls
// /api/tasks/{id} for progress, replacing the retired CLI-only migration note.

import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useEffect, useRef, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { Badge } from '@/components/ui/badge'
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
import { toBcp47 } from '@/i18n/languages'
import { api } from '@/lib/api'
import { formatDateTime } from '@/lib/format'
import { toast } from '@/lib/toast'

import { FormNavigationGuard } from '../../../components/form-navigation-guard'
import {
  SettingsSectionCard,
  SettingsSectionSkeleton,
} from '../../../components/settings-section-card'
import { SettingsSectionError } from '../../../components/settings-section-error'
import {
  runtimeDatabaseQueryKeys,
  type RuntimeDatabaseConfig,
} from './database-shared'

const MIGRATION_POLL_INTERVAL_MS = 2000
const LOG_TAIL_LINES = 8

type MigrationTaskStatus = 'pending' | 'running' | 'succeeded' | 'failed'

const TERMINAL_TASK_STATUSES: ReadonlySet<MigrationTaskStatus> = new Set([
  'succeeded',
  'failed',
])

type BackgroundTaskLogEntry = {
  seq: number
  message: string
  createdAt: string
}

type MigrationTaskResult = {
  dialect: string
  connection: string
  overwrite: boolean
  version: string
  timestamp: number
  rows: Record<string, number>
}

// Mirrors handler/admin.BackgroundTask (camelCase JSON) as observed through
// GET /api/tasks/{id} → { success, task }.
type DatabaseMigrationTask = {
  id: string
  type: string
  title: string
  status: MigrationTaskStatus
  message: string
  error?: string | null
  result?: MigrationTaskResult | null
  dedupeKey?: string | null
  createdAt: string
  updatedAt: string
  startedAt?: string | null
  finishedAt?: string | null
  logs?: BackgroundTaskLogEntry[]
}

type GetTaskResponse = {
  success: boolean
  task: DatabaseMigrationTask
}

const migrationSchema = z.object({
  dialect: z.enum(['sqlite', 'postgres']),
  connectionString: z.string(),
  overwrite: z.boolean(),
  ssl: z.boolean(),
})

type MigrationFormValues = z.infer<typeof migrationSchema>

const DEFAULT_VALUES: MigrationFormValues = {
  dialect: 'postgres',
  connectionString: '',
  overwrite: true,
  ssl: false,
}

const taskStatusBadgeVariant: Record<
  MigrationTaskStatus,
  'secondary' | 'info' | 'success' | 'destructive'
> = {
  pending: 'secondary',
  running: 'info',
  succeeded: 'success',
  failed: 'destructive',
}

/**
 * The migrate endpoint surfaces rejection reasons (e.g. a target that
 * resolves to the live runtime database) as a plain 400 `{error}` message;
 * there is no machine-readable detail classifier to match on.
 */
function extractServerErrorMessage(error: unknown): string | null {
  const responseData = (error as { response?: { data?: unknown } } | null)
    ?.response?.data
  if (!responseData || typeof responseData !== 'object') return null
  const record = responseData as Record<string, unknown>
  if (typeof record.error === 'string' && record.error) return record.error
  if (typeof record.message === 'string' && record.message) {
    return record.message
  }
  return null
}

function isSameMigrationTargetError(message: string): boolean {
  return message.includes('相同')
}

type MigrationResultSummaryProps = {
  result: MigrationTaskResult
  finishedAt?: string | null
}

function MigrationResultSummary({
  result,
  finishedAt,
}: MigrationResultSummaryProps) {
  const { t, i18n } = useTranslation()
  const locale = toBcp47(i18n.language || 'en')
  const tableEntries = Object.entries(result.rows).sort(([left], [right]) =>
    left.localeCompare(right)
  )
  const totalRows = tableEntries.reduce(
    (total, [, rowCount]) => total + rowCount,
    0
  )

  return (
    <div className='space-y-2'>
      <p className='text-sm font-medium'>
        {t('settings.systemInfo.database.migration.status.resultTitle')}
      </p>
      <dl className='text-muted-foreground grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-xs'>
        <dt>{t('settings.systemInfo.database.migration.status.target')}</dt>
        <dd className='font-mono break-all'>{result.connection}</dd>
        <dt>{t('settings.systemInfo.database.migration.status.version')}</dt>
        <dd className='font-mono'>{result.version}</dd>
        {finishedAt ? (
          <>
            <dt>
              {t('settings.systemInfo.database.migration.status.finishedAt')}
            </dt>
            <dd>{formatDateTime(finishedAt, locale)}</dd>
          </>
        ) : null}
      </dl>
      {tableEntries.length > 0 ? (
        <div className='space-y-1'>
          <p className='text-muted-foreground text-xs'>
            {t('settings.systemInfo.database.migration.status.rowsTitle')} ·{' '}
            {t('settings.systemInfo.database.migration.status.rowsTotal', {
              count: totalRows,
            })}
          </p>
          <div className='bg-muted/40 max-h-40 overflow-auto rounded-md border px-2 py-1'>
            {tableEntries.map(([tableName, rowCount]) => (
              <div
                key={tableName}
                className='flex items-center justify-between gap-4 border-b border-dashed py-1 text-xs last:border-b-0'
              >
                <code className='text-muted-foreground'>{tableName}</code>
                <span className='font-mono tabular-nums'>{rowCount}</span>
              </div>
            ))}
          </div>
        </div>
      ) : null}
    </div>
  )
}

export function DatabaseMigrationSection() {
  const { t } = useTranslation()
  const [migrationTaskId, setMigrationTaskId] = useState<string | null>(null)
  const [confirmOpen, setConfirmOpen] = useState(false)
  // Guards the terminal-status toasts: the task query refetches every poll,
  // so each new task object would otherwise re-fire the effect.
  const handledTerminalRef = useRef<string | null>(null)

  const configQuery = useQuery<RuntimeDatabaseConfig>({
    queryKey: runtimeDatabaseQueryKeys.all,
    queryFn: async () =>
      (await api.getRuntimeDatabaseConfig()) as RuntimeDatabaseConfig,
    staleTime: 30 * 1000,
  })

  const form = useForm<MigrationFormValues>({
    resolver: zodResolver(migrationSchema) as never,
    defaultValues: DEFAULT_VALUES,
  })

  const dialect = form.watch('dialect')

  // Sensible default target: migrate away from the live dialect. Skipped once
  // the user has touched the form so their edits are never clobbered.
  useEffect(() => {
    if (!configQuery.data || form.formState.isDirty) return
    const liveDialect = configQuery.data.active?.dialect
    const targetDialect = liveDialect === 'postgres' ? 'sqlite' : 'postgres'
    form.setValue('dialect', targetDialect, { shouldDirty: false })
    if (targetDialect === 'sqlite') {
      form.setValue('ssl', false, { shouldDirty: false })
    }
  }, [configQuery.data, form])

  const taskQuery = useQuery<DatabaseMigrationTask>({
    queryKey: ['background-task', migrationTaskId],
    queryFn: async () => {
      // `enabled` guarantees a taskId; the guard keeps the type checker happy.
      if (!migrationTaskId) {
        throw new Error('Background task polling requires a task id')
      }
      const response = (await api.getTask(migrationTaskId)) as GetTaskResponse
      return response.task
    },
    enabled: Boolean(migrationTaskId),
    refetchInterval: (query) =>
      query.state.data && TERMINAL_TASK_STATUSES.has(query.state.data.status)
        ? false
        : MIGRATION_POLL_INTERVAL_MS,
  })

  const task = taskQuery.data
  // A task is "active" from the moment its id is known until the polled task
  // reaches a terminal status — this covers the window between the 202
  // response and the first poll result, where `task` is still undefined.
  // A poll error (transient network hiccup) un-blocks the form: the interval
  // keeps retrying while no data has arrived, and the backend dedupes
  // concurrent migrations on the same task anyway.
  const polledStatus = taskQuery.data?.status
  const taskIsActive =
    Boolean(migrationTaskId) &&
    !taskQuery.isError &&
    (polledStatus === undefined || !TERMINAL_TASK_STATUSES.has(polledStatus))

  useEffect(() => {
    if (!task || !TERMINAL_TASK_STATUSES.has(task.status)) return
    const terminalKey = `${task.id}:${task.status}`
    if (handledTerminalRef.current === terminalKey) return
    handledTerminalRef.current = terminalKey
    if (task.status === 'succeeded') {
      toast.success(t('settings.systemInfo.database.migration.toast.completed'))
    } else {
      toast.error(
        task.error ?? t('settings.systemInfo.database.migration.toast.failed')
      )
    }
  }, [task, t])

  const testMutation = useMutation({
    mutationFn: async (values: MigrationFormValues) =>
      api.testExternalDatabaseConnection({
        dialect: values.dialect,
        connectionString: values.connectionString.trim(),
        ssl: values.ssl,
      }),
    onSuccess: () =>
      toast.success(t('settings.systemInfo.database.toast.testOk')),
    onError: () =>
      toast.error(t('settings.systemInfo.database.toast.testFailed')),
  })

  const startMutation = useMutation({
    mutationFn: async (values: MigrationFormValues) =>
      api.startDatabaseMigration({
        dialect: values.dialect,
        connectionString: values.connectionString.trim(),
        overwrite: values.overwrite,
        ssl: values.ssl,
      }),
    onSuccess: (response) => {
      handledTerminalRef.current = null
      setMigrationTaskId(response.taskId)
      setConfirmOpen(false)
      toast.info(t('settings.systemInfo.database.migration.toast.started'))
    },
    onError: (error) => {
      setConfirmOpen(false)
      const serverMessage = extractServerErrorMessage(error)
      if (serverMessage && isSameMigrationTargetError(serverMessage)) {
        toast.error(
          t('settings.systemInfo.database.migration.toast.sameTarget')
        )
      } else if (serverMessage) {
        toast.error(serverMessage)
      } else {
        toast.error(
          t('settings.systemInfo.database.migration.toast.startFailed')
        )
      }
    },
  })

  function requireConnection(messageKey: string): string | null {
    const connection = form.getValues('connectionString').trim()
    if (connection) return connection
    form.setError('connectionString', { message: messageKey })
    return null
  }

  function handleTestConnection() {
    const connection = requireConnection(
      'settings.systemInfo.database.migration.schema.testConnectionRequired'
    )
    if (!connection) return
    testMutation.mutate({
      ...form.getValues(),
      connectionString: connection,
    })
  }

  function handleStartMigration() {
    const connection = requireConnection(
      'settings.systemInfo.database.migration.schema.connectionRequired'
    )
    if (!connection) return
    setConfirmOpen(true)
  }

  const busy = taskIsActive || startMutation.isPending || testMutation.isPending
  const activeConfig = configQuery.data?.active

  if (configQuery.isLoading) {
    return <SettingsSectionSkeleton />
  }
  if (configQuery.isError || !configQuery.data) {
    return (
      <SettingsSectionError
        title={t('settings.systemInfo.database.migration.title')}
        onRetry={() => void configQuery.refetch()}
      />
    )
  }

  return (
    <SettingsSectionCard
      title={t('settings.systemInfo.database.migration.title')}
      description={t('settings.systemInfo.database.migration.description')}
    >
      <div className='space-y-5'>
        {activeConfig ? (
          <div className='bg-muted/25 rounded-lg border p-3'>
            <p className='text-muted-foreground text-xs font-medium'>
              {t('settings.systemInfo.database.migration.sourceLabel')}
            </p>
            <code className='text-foreground mt-1 block text-xs break-all'>
              {activeConfig.dialect} · {activeConfig.connection}
              {activeConfig.ssl ? ' · SSL' : ''}
            </code>
          </div>
        ) : null}

        <Form {...form}>
          <form className='space-y-4'>
            <FormField
              control={form.control}
              name='dialect'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {t('settings.systemInfo.database.migration.fields.dialect')}
                  </FormLabel>
                  <Select
                    value={field.value}
                    disabled={busy}
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
                      'settings.systemInfo.database.migration.fields.connectionString'
                    )}
                  </FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      value={field.value ?? ''}
                      className='font-mono'
                      autoComplete='off'
                      spellCheck={false}
                      disabled={busy}
                      placeholder={
                        dialect === 'postgres'
                          ? 'postgres://user:pass@host:5432/db'
                          : './data/target.db'
                      }
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'settings.systemInfo.database.migration.fields.connectionStringHint'
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
                        disabled={busy}
                        onCheckedChange={field.onChange}
                      />
                    </FormControl>
                    <div className='space-y-1'>
                      <FormLabel className='cursor-pointer'>
                        {t('settings.systemInfo.database.migration.fields.ssl')}
                      </FormLabel>
                      <FormDescription>
                        {t(
                          'settings.systemInfo.database.migration.fields.sslHint'
                        )}
                      </FormDescription>
                    </div>
                  </FormItem>
                )}
              />
            ) : null}
            <FormField
              control={form.control}
              name='overwrite'
              render={({ field }) => (
                <FormItem className='flex flex-row items-start gap-3 rounded-lg border p-3'>
                  <FormControl>
                    <Checkbox
                      checked={field.value}
                      disabled={busy}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                  <div className='space-y-1'>
                    <FormLabel className='cursor-pointer'>
                      {t(
                        'settings.systemInfo.database.migration.fields.overwrite'
                      )}
                    </FormLabel>
                    <FormDescription>
                      {t(
                        'settings.systemInfo.database.migration.fields.overwriteHint'
                      )}
                    </FormDescription>
                  </div>
                </FormItem>
              )}
            />

            <div className='flex flex-col gap-3 border-t pt-4 sm:flex-row sm:items-center sm:justify-between'>
              <div className='flex flex-wrap gap-2'>
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  disabled={busy}
                  onClick={handleTestConnection}
                >
                  {testMutation.isPending
                    ? t('settings.common.testing')
                    : t(
                        'settings.systemInfo.database.migration.testConnection'
                      )}
                </Button>
                <Button
                  type='button'
                  variant='destructive'
                  size='sm'
                  disabled={busy}
                  onClick={handleStartMigration}
                >
                  {startMutation.isPending || taskIsActive
                    ? t('settings.systemInfo.database.migration.migrating')
                    : t(
                        'settings.systemInfo.database.migration.startMigration'
                      )}
                </Button>
              </div>
            </div>
          </form>
        </Form>

        {task ? (
          <div className='space-y-3 rounded-lg border p-3'>
            <div className='flex flex-wrap items-center gap-2'>
              <Badge variant={taskStatusBadgeVariant[task.status]}>
                {t(
                  `settings.systemInfo.database.migration.status.${task.status}`
                )}
              </Badge>
              <p className='text-muted-foreground min-w-0 flex-1 truncate text-xs'>
                {task.message}
              </p>
            </div>
            {task.logs && task.logs.length > 0 ? (
              <div className='space-y-1'>
                <p className='text-muted-foreground text-xs'>
                  {t('settings.systemInfo.database.migration.status.logsTitle')}
                </p>
                <pre className='bg-muted/40 text-muted-foreground max-h-40 overflow-auto rounded-md border p-2 font-mono text-xs whitespace-pre-wrap'>
                  {task.logs
                    .slice(-LOG_TAIL_LINES)
                    .map((entry) => entry.message)
                    .join('\n')}
                </pre>
              </div>
            ) : null}
            {task.status === 'succeeded' && task.result ? (
              <MigrationResultSummary
                result={task.result}
                finishedAt={task.finishedAt}
              />
            ) : null}
            {task.status === 'failed' && task.error ? (
              <p className='text-destructive text-sm break-all'>{task.error}</p>
            ) : null}
          </div>
        ) : null}
      </div>

      <ConfirmDialog
        open={confirmOpen}
        title={t('settings.systemInfo.database.migration.confirm.title')}
        description={t(
          'settings.systemInfo.database.migration.confirm.description'
        )}
        confirmLabel={t(
          'settings.systemInfo.database.migration.confirm.confirmLabel'
        )}
        cancelLabel={t('settings.common.cancel')}
        destructive
        onCancel={() => setConfirmOpen(false)}
        onConfirm={() => startMutation.mutate(form.getValues())}
      />

      <FormNavigationGuard enabled={form.formState.isDirty} />
    </SettingsSectionCard>
  )
}
