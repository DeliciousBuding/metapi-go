import { useMutation, useQuery } from '@tanstack/react-query'
import { useEffect, useRef, useState } from 'react'
import type { UseFormReturn } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { useReauthReplay } from '@/components/common/reauth-provider'
import { api } from '@/lib/api'
import { extractApiErrorBody, resolveResponseMessage } from '@/lib/http-client'
import { toast } from '@/lib/toast'

import type { MigrationFormValues } from './use-database-migration-form'

const MIGRATION_POLL_INTERVAL_MS = 2000
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
export type DatabaseMigrationTask = {
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
/**
 * Registered errorCode for a migration whose target resolves to the live
 * runtime database (#1065; registry: docs/api.md, backend constant:
 * handler/admin/error_codes.go). This section used to substring-match the
 * Chinese rejection text, which never fired against the Go backend's
 * English body — the code is the contract now; the `error` message remains
 * the display fallback for bodies without a code.
 */
const ERROR_CODE_SAME_MIGRATION_TARGET = 'sameMigrationTarget'

export function useDatabaseMigration(form: UseFormReturn<MigrationFormValues>) {
  const { t } = useTranslation()
  const handleReauth = useReauthReplay()
  const [migrationTaskId, setMigrationTaskId] = useState<string | null>(null)
  const [confirmOpen, setConfirmOpen] = useState(false)
  // Guards the terminal-status toasts: the task query refetches every poll,
  // so each new task object would otherwise re-fire the effect.
  const handledTerminalRef = useRef<string | null>(null)
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
      toast.success(t('settings.operations.database.migration.toast.completed'))
    } else {
      toast.error(
        task.error ?? t('settings.operations.database.migration.toast.failed')
      )
    }
  }, [task, t])

  const testMutation = useMutation({
    mutationFn: async (input: {
      values: MigrationFormValues
      confirmToken?: string
    }) => {
      const data = {
        dialect: input.values.dialect,
        connectionString: input.values.connectionString.trim(),
        ssl: input.values.ssl,
      }
      return input.confirmToken
        ? api.testExternalDatabaseConnection(data, input.confirmToken)
        : api.testExternalDatabaseConnection(data)
    },
    onSuccess: () =>
      toast.success(t('settings.operations.database.toast.testOk')),
    onError: (error, input) => {
      if (
        handleReauth(
          error,
          t('reauth.operations.databaseTest'),
          (confirmToken) =>
            testMutation.mutateAsync({
              values: input.values,
              confirmToken,
            })
        )
      ) {
        return
      }
      toast.error(t('settings.operations.database.toast.testFailed'))
    },
  })

  const startMutation = useMutation({
    mutationFn: async (input: {
      values: MigrationFormValues
      confirmToken?: string
    }) => {
      const data = {
        dialect: input.values.dialect,
        connectionString: input.values.connectionString.trim(),
        overwrite: input.values.overwrite,
        ssl: input.values.ssl,
      }
      return input.confirmToken
        ? api.startDatabaseMigration(data, input.confirmToken)
        : api.startDatabaseMigration(data)
    },
    onSuccess: (response) => {
      handledTerminalRef.current = null
      setMigrationTaskId(response.taskId)
      setConfirmOpen(false)
      toast.info(t('settings.operations.database.migration.toast.started'))
    },
    onError: (error, input) => {
      setConfirmOpen(false)
      // The destructive confirmation already happened; a reauth demand only
      // adds the master-token step and replays the same job exactly once.
      if (
        handleReauth(
          error,
          t('reauth.operations.databaseMigrate'),
          (confirmToken) =>
            startMutation.mutateAsync({
              values: input.values,
              confirmToken,
            })
        )
      ) {
        return
      }
      const errorBody = extractApiErrorBody(error)
      if (errorBody?.errorCode === ERROR_CODE_SAME_MIGRATION_TARGET) {
        toast.error(
          t('settings.operations.database.migration.toast.sameTarget')
        )
        return
      }
      const serverMessage = resolveResponseMessage(errorBody)
      toast.error(
        serverMessage ??
          t('settings.operations.database.migration.toast.startFailed')
      )
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
      'settings.operations.database.migration.schema.testConnectionRequired'
    )
    if (!connection) return
    testMutation.mutate({
      values: { ...form.getValues(), connectionString: connection },
    })
  }

  function handleStartMigration() {
    const connection = requireConnection(
      'settings.operations.database.migration.schema.connectionRequired'
    )
    if (!connection) return
    setConfirmOpen(true)
  }

  const busy = taskIsActive || startMutation.isPending || testMutation.isPending
  return {
    task,
    busy,
    testing: testMutation.isPending,
    migrating: startMutation.isPending || taskIsActive,
    confirmOpen,
    setConfirmOpen,
    handleTestConnection,
    handleStartMigration,
    confirmMigration: () => startMutation.mutate({ values: form.getValues() }),
  }
}
