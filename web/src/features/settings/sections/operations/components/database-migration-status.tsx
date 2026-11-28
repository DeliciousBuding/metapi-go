import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { toBcp47 } from '@/i18n/languages'
import { formatDateTime } from '@/lib/format'

import type { DatabaseMigrationTask } from './use-database-migration'

const LOG_TAIL_LINES = 8
const taskStatusBadgeVariant: Record<
  DatabaseMigrationTask['status'],
  'secondary' | 'info' | 'success' | 'destructive'
> = {
  pending: 'secondary',
  running: 'info',
  succeeded: 'success',
  failed: 'destructive',
}
type MigrationResultSummaryProps = {
  result: NonNullable<DatabaseMigrationTask['result']>
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
      <h3 className='text-sm font-medium'>
        {t('settings.operations.database.migration.status.resultTitle')}
      </h3>
      <dl className='text-muted-foreground grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-xs'>
        <dt>{t('settings.operations.database.migration.status.target')}</dt>
        <dd className='font-mono break-all'>{result.connection}</dd>
        <dt>{t('settings.operations.database.migration.status.version')}</dt>
        <dd className='font-mono'>{result.version}</dd>
        {finishedAt ? (
          <>
            <dt>
              {t('settings.operations.database.migration.status.finishedAt')}
            </dt>
            <dd>{formatDateTime(finishedAt, locale)}</dd>
          </>
        ) : null}
      </dl>
      {tableEntries.length > 0 ? (
        <div className='space-y-1'>
          <p className='text-muted-foreground text-xs'>
            {t('settings.operations.database.migration.status.rowsTitle')} ·{' '}
            {t('settings.operations.database.migration.status.rowsTotal', {
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

export function DatabaseMigrationStatus({
  task,
}: {
  task: DatabaseMigrationTask
}) {
  const { t } = useTranslation()
  return (
    <div className='space-y-3 rounded-lg border p-3'>
      <div className='flex flex-wrap items-center gap-2'>
        <Badge variant={taskStatusBadgeVariant[task.status]}>
          {t(`settings.operations.database.migration.status.${task.status}`)}
        </Badge>
        <p className='text-muted-foreground min-w-0 flex-1 truncate text-xs'>
          {task.message}
        </p>
      </div>
      {task.logs && task.logs.length > 0 ? (
        <div className='space-y-1'>
          <p className='text-muted-foreground text-xs'>
            {t('settings.operations.database.migration.status.logsTitle')}
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
  )
}
