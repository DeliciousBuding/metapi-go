import { useMutation, useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import {
  Activity,
  ChevronDown,
  ClipboardList,
  Play,
  RefreshCw,
} from 'lucide-react'
import { Fragment, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button, buttonVariants } from '@/components/ui/button'
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
} from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { toBcp47 } from '@/i18n/languages'
import { api } from '@/lib/api'
import type { SchedulerProbeRunSummary } from '@/lib/api/types'
import { formatInt, formatRelativeTime } from '@/lib/format'
import { toast } from '@/lib/toast'
import { cn } from '@/lib/utils'

/** Tone + label for a scheduler job last-status value. */
const SCHEDULER_STATUS_BADGE: Record<
  string,
  { variant: 'success' | 'destructive' | 'info' | 'secondary'; key: string }
> = {
  success: {
    variant: 'success',
    key: 'dashboard.overview.scheduledTasks.statusSuccess',
  },
  failed: {
    variant: 'destructive',
    key: 'dashboard.overview.scheduledTasks.statusFailed',
  },
  running: {
    variant: 'info',
    key: 'dashboard.overview.scheduledTasks.statusRunning',
  },
  never: {
    variant: 'secondary',
    key: 'dashboard.overview.scheduledTasks.statusNever',
  },
}

/** Compact list of the model-probe scheduler's last completed passes. */
function ProbeRecentRuns({
  runs,
  locale,
}: {
  runs: SchedulerProbeRunSummary[]
  locale: string
}) {
  const { t } = useTranslation()
  if (runs.length === 0) {
    return (
      <p className='text-muted-foreground text-xs'>
        {t('dashboard.overview.scheduledTasks.runsEmpty')}
      </p>
    )
  }
  return (
    <ul className='space-y-1.5'>
      {runs.map((run) => {
        // Pass-level verdict stays honest: any failed target makes the pass
        // a failure even when others succeeded.
        const failed = run.failed > 0
        return (
          <li
            key={`${run.startedAt ?? ''}-${run.completedAt ?? ''}`}
            className='flex flex-wrap items-center gap-x-2 gap-y-1 text-xs'
          >
            <Badge variant={failed ? 'destructive' : 'success'}>
              {failed
                ? t('dashboard.overview.scheduledTasks.runFailed')
                : t('dashboard.overview.scheduledTasks.runSuccess')}
            </Badge>
            <span className='text-muted-foreground tabular-nums'>
              {run.completedAt
                ? formatRelativeTime(run.completedAt, locale)
                : '—'}
            </span>
            <span className='text-muted-foreground tabular-nums'>
              {t('dashboard.overview.scheduledTasks.runTargets', {
                count: run.targetsScanned,
              })}
            </span>
            <span className='text-muted-foreground tabular-nums'>
              {t('dashboard.overview.scheduledTasks.runCounts', {
                success: run.success,
                failed: run.failed,
                inconclusive: run.inconclusive,
                skipped: run.skipped,
              })}
            </span>
          </li>
        )
      })}
    </ul>
  )
}

export function MaintenancePanel() {
  const { t, i18n } = useTranslation()
  const locale = toBcp47(i18n.language || 'en')
  const [expandedJob, setExpandedJob] = useState<string | null>(null)
  const {
    data: schedulerStatus,
    isLoading: schedulerLoading,
    error: schedulerError,
    refetch: refetchScheduler,
  } = useQuery({
    queryKey: ['scheduler-status'],
    queryFn: () => api.getSchedulerStatus(),
  })

  const schedulerRows = schedulerStatus?.items ?? []

  const triggerProbeMutation = useMutation({
    mutationFn: () => api.probeModelsNow(),
    onSuccess: () => {
      toast.success(t('dashboard.overview.scheduledTasks.triggerQueued'))
      // The pass runs asynchronously server-side; a delayed refetch picks up
      // the finished run for the recent-runs view.
      window.setTimeout(() => {
        void refetchScheduler()
      }, 5000)
    },
    onError: (error: unknown) => {
      const message = error instanceof Error ? error.message : String(error)
      toast.error(
        t('dashboard.overview.scheduledTasks.triggerFailed', { message })
      )
    },
  })

  const renderSchedulerBody = (): ReactNode => {
    if (schedulerLoading) {
      return <Skeleton className='h-40 w-full rounded-md' />
    }
    if (schedulerError) {
      return (
        <Empty className='border-destructive/40 bg-destructive/10 min-h-24 border'>
          <EmptyHeader>
            <EmptyDescription className='text-destructive'>
              {t('dashboard.overview.scheduledTasks.loadError')}
            </EmptyDescription>
          </EmptyHeader>
          <EmptyContent>
            <Button
              variant='outline'
              size='sm'
              onClick={() => void refetchScheduler()}
            >
              <RefreshCw className='size-3.5' />
              {t('dashboard.overview.scheduledTasks.retry')}
            </Button>
          </EmptyContent>
        </Empty>
      )
    }
    if (schedulerRows.length === 0) {
      return (
        <Empty className='min-h-24 border'>
          <EmptyHeader>
            <EmptyMedia variant='icon'>
              <ClipboardList />
            </EmptyMedia>
            <EmptyDescription>
              {t('dashboard.overview.scheduledTasks.empty')}
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      )
    }
    return (
      <div className='overflow-x-auto rounded-lg border'>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className='w-1/3'>
                {t('dashboard.overview.scheduledTasks.colJob')}
              </TableHead>
              <TableHead>
                {t('dashboard.overview.scheduledTasks.colEnabled')}
              </TableHead>
              <TableHead>
                {t('dashboard.overview.scheduledTasks.colLastStatus')}
              </TableHead>
              <TableHead className='text-right'>
                {t('dashboard.overview.scheduledTasks.colRuns24h')}
              </TableHead>
              <TableHead className='text-right'>
                {t('dashboard.overview.scheduledTasks.colSuccess24h')}
              </TableHead>
              <TableHead className='text-right'>
                {t('dashboard.overview.scheduledTasks.colActions')}
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {schedulerRows.map((row) => {
              const status =
                SCHEDULER_STATUS_BADGE[row.lastStatus ?? ''] ??
                SCHEDULER_STATUS_BADGE.never
              const enabledLabel = row.enabled
                ? t('dashboard.overview.scheduledTasks.enabled')
                : t('dashboard.overview.scheduledTasks.disabled')
              const canTrigger =
                row.job === 'model-probe' && row.enabled === true
              const expanded = expandedJob === row.job
              const showRunsToggle =
                row.job === 'model-probe' && (row.recentRuns?.length ?? 0) > 0
              const jobLabel = t(
                `dashboard.overview.scheduledTasks.jobLabels.${row.job}`,
                { defaultValue: row.job }
              )
              const showCheckinLogsLink =
                row.job === 'checkin' && row.lastStatus === 'failed'
              return (
                <Fragment key={row.job}>
                  <TableRow>
                    <TableCell className='font-medium'>{jobLabel}</TableCell>
                    <TableCell>
                      <Badge variant={row.enabled ? 'success' : 'secondary'}>
                        {enabledLabel}
                      </Badge>
                    </TableCell>
                    <TableCell>
                      <Badge variant={status.variant}>{t(status.key)}</Badge>
                    </TableCell>
                    <TableCell className='text-right tabular-nums'>
                      {formatInt(row.runs24h)}
                    </TableCell>
                    <TableCell className='text-right tabular-nums'>
                      {formatInt(row.success24h)}
                    </TableCell>
                    <TableCell className='text-right'>
                      <div className='flex items-center justify-end gap-1'>
                        {showCheckinLogsLink && (
                          <Button
                            variant='outline'
                            size='sm'
                            render={
                              <Link
                                to='/checkin'
                                search={{ status: 'failed' }}
                              />
                            }
                          >
                            {t(
                              'dashboard.overview.scheduledTasks.viewCheckinLogs'
                            )}
                          </Button>
                        )}
                        {canTrigger && (
                          <Button
                            variant='outline'
                            size='sm'
                            disabled={triggerProbeMutation.isPending}
                            onClick={() => triggerProbeMutation.mutate()}
                          >
                            <Play className='size-3.5' />
                            {t(
                              'dashboard.overview.scheduledTasks.triggerButton'
                            )}
                          </Button>
                        )}
                        {showRunsToggle && (
                          <Button
                            variant='ghost'
                            size='sm'
                            aria-expanded={expanded}
                            aria-label={t(
                              'dashboard.overview.scheduledTasks.latestRuns'
                            )}
                            onClick={() =>
                              setExpandedJob(expanded ? null : row.job)
                            }
                          >
                            <ChevronDown
                              className={cn(
                                'size-3.5 transition-transform',
                                expanded && 'rotate-180'
                              )}
                            />
                          </Button>
                        )}
                      </div>
                    </TableCell>
                  </TableRow>
                  {expanded && row.recentRuns && (
                    <TableRow className='bg-muted/30'>
                      <TableCell colSpan={6} className='px-3 py-2'>
                        <ProbeRecentRuns
                          runs={row.recentRuns}
                          locale={locale}
                        />
                      </TableCell>
                    </TableRow>
                  )}
                </Fragment>
              )
            })}
          </TableBody>
        </Table>
      </div>
    )
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className='flex items-center gap-2'>
          <Activity className='size-4' />
          {t('dashboard.overview.scheduledTasks.title')}
        </CardTitle>
        <CardDescription className='text-xs'>
          {t('dashboard.overview.scheduledTasks.description')}
        </CardDescription>
        <CardAction>
          <Link
            to='/settings/$subarea/$section'
            params={{ subarea: 'operations', section: 'scheduling' }}
            className={buttonVariants({ variant: 'ghost', size: 'sm' })}
          >
            {t('dashboard.overview.scheduledTasks.editSchedule')}
          </Link>
        </CardAction>
      </CardHeader>
      <CardContent>{renderSchedulerBody()}</CardContent>
    </Card>
  )
}
