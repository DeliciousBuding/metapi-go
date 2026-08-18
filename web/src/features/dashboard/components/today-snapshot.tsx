// metapi-go/features/dashboard/components — today snapshot strip.
//
// The overview's one-screen aggregation row (audit ui-ux-2026-08: the home
// page lacked an at-a-glance "today" surface). Four cells:
//   - aggregate balance + 7-day delta (newest vs oldest captured point of the
//     8-day balance-history window; the backend serves points chronologically
//     ASC, so the oldest point is ~7 days ago),
//   - today's successful check-ins (dashboard snapshot),
//   - actionable attention count (deep-links to the availability section),
//   - a live availability dot backed by the realtime ops WebSocket.
// Missing data renders "—" — numbers are never invented.

import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { ArrowDownRight, ArrowUpRight, Minus } from 'lucide-react'
import { useMemo, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { api } from '@/lib/api'
import { formatInt } from '@/lib/format'
import { cn } from '@/lib/utils'

import { useRealtimeOps } from '../hooks/use-realtime-ops'

/** Summary-view dashboard snapshot (GET /api/stats/dashboard?view=summary). */
type DashboardSnapshot = {
  todayCheckin?: {
    total: number
    success: number
    skipped: number
    failed: number
  }
}

/** Aggregate balance history (GET /api/stats/balance-history?accountId=0). */
type BalanceHistoryResponse = {
  series: Array<{
    accountId: number
    points: Array<{ day: string; balance: number }>
  }>
  days: number
}

/** Attention response (GET /api/stats/attention?limit=20). */
type AttentionResponse = {
  total: number
}

type BalanceTrend = {
  total: number | undefined
  deltaPercent: number | undefined
}

/** Adaptive currency formatting (mirrors the legacy chart tooltips). */
function formatBalance(value: number | null | undefined): string {
  if (value === null || value === undefined || !Number.isFinite(value)) {
    return '—'
  }
  let fractionDigits = 6
  if (value >= 1) fractionDigits = 3
  if (value >= 1000) fractionDigits = 2
  return [
    '$',
    value.toLocaleString(undefined, {
      minimumFractionDigits: fractionDigits,
      maximumFractionDigits: fractionDigits,
    }),
  ].join('')
}

/**
 * Aggregate the balance across all account series. The delta compares the
 * newest captured point with the oldest point of the window (~7 days ago).
 * A zero previous total yields no delta — dividing by zero would invent one.
 */
function computeBalanceTrend(
  balanceHistory: BalanceHistoryResponse | undefined
): BalanceTrend {
  const series = balanceHistory?.series ?? []
  let latestTotal = 0
  let previousTotal = 0
  let hasPoints = false
  for (const entry of series) {
    const points = entry.points ?? []
    if (points.length === 0) continue
    hasPoints = true
    const latestPoint = points.at(-1)
    const earliestPoint = points.at(0)
    latestTotal += latestPoint ? latestPoint.balance : 0
    previousTotal += earliestPoint ? earliestPoint.balance : 0
  }
  if (!hasPoints) return { total: undefined, deltaPercent: undefined }
  if (!Number.isFinite(previousTotal) || previousTotal === 0) {
    return { total: latestTotal, deltaPercent: undefined }
  }
  const deltaPercent =
    ((latestTotal - previousTotal) / Math.abs(previousTotal)) * 100
  return { total: latestTotal, deltaPercent }
}

export function TodaySnapshotStrip() {
  const { t } = useTranslation()

  const { data: snapshot, isLoading: snapshotLoading } = useQuery({
    queryKey: ['dashboard-snapshot'],
    queryFn: () => api.getDashboardSnapshot() as Promise<DashboardSnapshot>,
  })

  const { data: balanceHistory, isLoading: balanceLoading } = useQuery({
    queryKey: ['dashboard-balance-spark', 0, 8],
    queryFn: () =>
      api.getBalanceHistory(0, 8) as Promise<BalanceHistoryResponse>,
  })

  const { data: attention, isLoading: attentionLoading } = useQuery({
    queryKey: ['dashboard-attention', 20],
    queryFn: () => api.getAttention(20) as Promise<AttentionResponse>,
  })

  const { sample: realtime } = useRealtimeOps()

  const trend = useMemo(
    () => computeBalanceTrend(balanceHistory),
    [balanceHistory]
  )
  const checkinSuccess = snapshot?.todayCheckin?.success
  const attentionTotal = attention?.total

  const renderBalanceValue = (): ReactNode => {
    if (balanceLoading) return <Skeleton className='h-7 w-28' />
    return (
      <div className='truncate text-xl font-semibold tabular-nums'>
        {formatBalance(trend.total)}
      </div>
    )
  }

  const renderBalanceDelta = (): ReactNode | null => {
    if (balanceLoading) return null
    const delta = trend.deltaPercent
    if (delta === undefined || !Number.isFinite(delta)) {
      return (
        <div className='text-muted-foreground flex items-center gap-1 text-xs'>
          <Minus className='size-3' />
          <span className='tabular-nums'>—</span>
          <span>{t('dashboard.overview.snapshot.vs7d')}</span>
        </div>
      )
    }
    let DeltaIcon = Minus
    let deltaClassName = 'text-muted-foreground'
    if (delta > 0) {
      DeltaIcon = ArrowUpRight
      deltaClassName = 'text-success'
    } else if (delta < 0) {
      DeltaIcon = ArrowDownRight
      deltaClassName = 'text-destructive'
    }
    const sign = delta > 0 ? '+' : ''
    return (
      <div className={cn('flex items-center gap-1 text-xs', deltaClassName)}>
        <DeltaIcon className='size-3' />
        <span className='tabular-nums'>
          {sign}
          {delta.toFixed(1)}%
        </span>
        <span className='text-muted-foreground'>
          {t('dashboard.overview.snapshot.vs7d')}
        </span>
      </div>
    )
  }

  let statusKey = 'dashboard.availability.realtime.statusConnecting'
  let statusDotClassName = 'animate-pulse bg-muted-foreground/50'
  if (realtime.gaveUp) {
    statusKey = 'dashboard.availability.realtime.statusDisconnected'
    statusDotClassName = 'bg-destructive'
  } else if (realtime.connected) {
    statusKey = 'dashboard.availability.realtime.statusLive'
    statusDotClassName = 'bg-success'
  }

  return (
    <Card>
      <CardHeader className='pb-2'>
        <CardTitle className='text-sm font-medium'>
          {t('dashboard.overview.snapshot.title')}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div className='grid grid-cols-2 gap-4 lg:grid-cols-4 lg:gap-6'>
          <div className='flex min-w-0 flex-col gap-1'>
            <div className='text-muted-foreground truncate text-xs'>
              {t('dashboard.overview.snapshot.totalBalance')}
            </div>
            {renderBalanceValue()}
            {renderBalanceDelta()}
          </div>

          <div className='flex min-w-0 flex-col gap-1'>
            <div className='text-muted-foreground truncate text-xs'>
              {t('dashboard.overview.snapshot.checkinSuccess')}
            </div>
            {snapshotLoading ? (
              <Skeleton className='h-7 w-12' />
            ) : (
              <div className='text-xl font-semibold tabular-nums'>
                {formatInt(checkinSuccess ?? null)}
              </div>
            )}
          </div>

          <Link
            to='/dashboard/$section'
            params={{ section: 'availability' }}
            aria-label={t('dashboard.overview.snapshot.attentionOpen')}
            className='group focus-visible:ring-ring/50 flex min-w-0 flex-col gap-1 rounded-lg outline-none focus-visible:ring-3'
          >
            <span className='text-muted-foreground truncate text-xs'>
              {t('dashboard.overview.snapshot.attention')}
            </span>
            {attentionLoading ? (
              <Skeleton className='h-7 w-12' />
            ) : (
              <span
                className={cn(
                  'text-xl font-semibold tabular-nums group-hover:underline',
                  attentionTotal === 0 ? 'text-success' : 'text-foreground'
                )}
              >
                {formatInt(attentionTotal ?? null)}
              </span>
            )}
          </Link>

          <div className='flex min-w-0 flex-col gap-1'>
            <div className='text-muted-foreground truncate text-xs'>
              {t('dashboard.overview.snapshot.availability')}
            </div>
            <div className='flex items-center gap-1.5 text-sm font-medium'>
              <span
                className={cn(
                  'size-2 shrink-0 rounded-full',
                  statusDotClassName
                )}
              />
              {t(statusKey)}
            </div>
          </div>
        </div>
      </CardContent>
    </Card>
  )
}
