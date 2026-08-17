// metapi-go/features/dashboard/sections/overview — overview section.
//
// Plan §5.5.1 overview: core metric cards (accounts / sites / today's checkin
// success rate / today's proxy requests) + AnnouncementBanner. The legacy
// SchedulerStatusPanel is merged in here (a compact scheduled-tasks card).
//
// Phase 3: wires api.getDashboardSnapshot() (view=summary) for the live stat
// numbers, api.getBalanceHistory(0, 8) for the account sparkline (aggregate
// balance over the last 8 captured points), and api.getSchedulerStatus() for
// the scheduled-tasks table.

import { useQuery } from '@tanstack/react-query'
import {
  Activity,
  CalendarCheck,
  ClipboardList,
  Globe,
  RefreshCw,
  Users,
} from 'lucide-react'
import { useMemo, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { api } from '@/lib/api'
import { formatInt, formatRatio } from '@/lib/format'

import { AnnouncementBanner } from '../../components/announcement-banner'
import { StatCard } from '../../components/stat-card'
import { TodaySnapshotStrip } from '../../components/today-snapshot'

/** Summary-view dashboard snapshot (GET /api/stats/dashboard?view=summary). */
type DashboardSnapshot = {
  siteCount?: number
  accountCount?: number
  totalAccounts?: number
  activeAccounts?: number
  todayCheckin?: {
    total: number
    success: number
    skipped: number
    failed: number
  }
  proxy24h?: {
    total: number
    success: number
    totalTokens: number
    totalCost: number
  }
  performance?: { requestsPerMinute: number; tokensPerMinute: number }
}

/** Aggregate balance history (GET /api/stats/balance-history?accountId=0). */
type BalanceHistoryResponse = {
  series: Array<{
    accountId: number
    points: Array<{ day: string; balance: number }>
  }>
  days: number
}

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

export function OverviewSection() {
  const { t } = useTranslation()

  const { data: snapshot, isLoading: snapshotLoading } = useQuery({
    queryKey: ['dashboard-snapshot'],
    queryFn: () => api.getDashboardSnapshot() as Promise<DashboardSnapshot>,
    // Keep the QPS / 24h-proxy stat cards fresh without a manual refresh.
    refetchInterval: 10 * 1000,
  })

  const { data: balanceHistory } = useQuery({
    queryKey: ['dashboard-balance-spark', 0, 8],
    queryFn: () =>
      api.getBalanceHistory(0, 8) as Promise<BalanceHistoryResponse>,
  })

  const {
    data: schedulerStatus,
    isLoading: schedulerLoading,
    error: schedulerError,
    refetch: refetchScheduler,
  } = useQuery({
    queryKey: ['scheduler-status'],
    queryFn: () => api.getSchedulerStatus(),
  })

  const accountSpark = useMemo(() => {
    const series = balanceHistory?.series
    if (!series || series.length === 0) return undefined
    return series[0].points.map((point) => point.balance)
  }, [balanceHistory])

  const schedulerRows = schedulerStatus?.items ?? []

  const totalAccounts = snapshot?.totalAccounts ?? snapshot?.accountCount
  const activeAccounts = snapshot?.activeAccounts
  const siteCount = snapshot?.siteCount
  const checkin = snapshot?.todayCheckin
  const proxy = snapshot?.proxy24h
  const performance = snapshot?.performance

  // Animate integer KPI values; round during easing so grouping stays clean.
  const animateInt = (n: number) => formatInt(Math.round(n))

  const accountHint = t('dashboard.overview.statCards.accountCountHint', {
    active: activeAccounts !== undefined ? formatInt(activeAccounts) : '—',
  })
  const proxyHint = t('dashboard.overview.statCards.proxy24hHint', {
    success: proxy?.success ?? 0,
    rpm: performance?.requestsPerMinute ?? 0,
  })

  const checkinDetails = useMemo(() => {
    if (!checkin) return undefined
    return [
      {
        label: t('dashboard.overview.statCards.checkinSucceeded'),
        value: formatInt(checkin.success),
      },
      {
        label: t('dashboard.overview.statCards.checkinSkipped'),
        value: formatInt(checkin.skipped),
      },
    ]
  }, [checkin, t])

  const renderSchedulerBody = (): ReactNode => {
    if (schedulerLoading) {
      return <Skeleton className='h-40 w-full rounded-md' />
    }
    if (schedulerError) {
      return (
        <div className='border-destructive/40 bg-destructive/10 flex min-h-24 flex-col items-center justify-center gap-3 rounded-lg border py-8 text-center'>
          <p className='text-destructive text-sm'>
            {t('dashboard.overview.scheduledTasks.loadError')}
          </p>
          <Button
            variant='outline'
            size='sm'
            onClick={() => void refetchScheduler()}
          >
            <RefreshCw className='mr-1 size-3.5' />
            {t('dashboard.overview.scheduledTasks.retry')}
          </Button>
        </div>
      )
    }
    if (schedulerRows.length === 0) {
      return (
        <div className='flex min-h-24 flex-col items-center justify-center gap-2 rounded-lg border border-dashed py-8 text-center'>
          <ClipboardList className='text-muted-foreground/60 size-5' />
          <p className='text-muted-foreground text-sm'>
            {t('dashboard.overview.scheduledTasks.empty')}
          </p>
        </div>
      )
    }
    return (
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
            return (
              <TableRow key={row.job}>
                <TableCell className='font-medium'>{row.job}</TableCell>
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
              </TableRow>
            )
          })}
        </TableBody>
      </Table>
    )
  }

  return (
    <div className='flex flex-col gap-4'>
      <AnnouncementBanner />

      <TodaySnapshotStrip />

      <div className='grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4'>
        <StatCard
          title={t('dashboard.overview.statCards.accountCount')}
          value={formatInt(totalAccounts ?? null)}
          valueNumber={totalAccounts ?? undefined}
          valueFormat={animateInt}
          hint={accountHint}
          spark={accountSpark}
          icon={Users}
          loading={snapshotLoading}
        />
        <StatCard
          title={t('dashboard.overview.statCards.siteCount')}
          value={formatInt(siteCount ?? null)}
          valueNumber={siteCount ?? undefined}
          valueFormat={animateInt}
          hint={t('dashboard.overview.statCards.siteCountHint')}
          icon={Globe}
          loading={snapshotLoading}
        />
        <StatCard
          title={t('dashboard.overview.statCards.todayCheckin')}
          value={!checkin ? '—' : formatRatio(checkin.success, checkin.total)}
          icon={CalendarCheck}
          tone='success'
          details={checkinDetails}
          loading={snapshotLoading}
        />
        <StatCard
          title={t('dashboard.overview.statCards.proxy24h')}
          value={!proxy ? '—' : formatInt(proxy.total)}
          valueNumber={proxy ? proxy.total : undefined}
          valueFormat={animateInt}
          hint={proxyHint}
          icon={Activity}
          loading={snapshotLoading}
        />
      </div>

      <Card>
        <CardHeader>
          <CardTitle className='flex items-center gap-2 text-sm font-medium'>
            <Activity className='size-4' />
            {t('dashboard.overview.scheduledTasks.title')}
          </CardTitle>
          <CardDescription className='text-xs'>
            {t('dashboard.overview.scheduledTasks.description')}
          </CardDescription>
        </CardHeader>
        <CardContent>{renderSchedulerBody()}</CardContent>
      </Card>
    </div>
  )
}
