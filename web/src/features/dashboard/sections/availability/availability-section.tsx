/* eslint-disable no-nested-ternary -- connection-tone uses chained ternaries */
// metapi-go/features/dashboard/sections/availability — availability section.
//
// Plan §5.5.1 availability: RealtimeOpsPanel（WebSocket 实时）+ an
// actionable-items surface (the legacy Monitors iframe is retired; phase 3
// renders api.getAttention() as a severity-ranked list of items needing
// operator eyes — expired accounts, low balances, disabled sites, events).

import { useQuery } from '@tanstack/react-query'
import { Inbox, Radio, ShieldCheck, TriangleAlert } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { api } from '@/lib/api'
import { cn } from '@/lib/utils'

import { useRealtimeOps } from '../../hooks/use-realtime-ops'
import type { RealtimeOpsSamplePoint } from '../../types'

const SPARK_BARS = 60

/** Attention response (GET /api/stats/attention). */
type AttentionResponse = {
  items: Array<{
    severity: 'critical' | 'warning' | 'info'
    category: string
    label: string
    target: string
    createdAt: string
  }>
  total: number
}

const SEVERITY_TONE: Record<
  'critical' | 'warning' | 'info',
  { dot: string; variant: 'destructive' | 'warning' | 'info' }
> = {
  critical: {
    dot: 'bg-destructive',
    variant: 'destructive',
  },
  warning: {
    dot: 'bg-warning',
    variant: 'warning',
  },
  info: {
    dot: 'bg-info',
    variant: 'info',
  },
}

function formatRate(rate: number): string {
  return `${(rate * 100).toFixed(1)}%`
}

// Health bands for the realtime sparkline. A second with no traffic is idle
// (neutral); otherwise the success fraction at that second maps to a band:
// healthy >= 0.95, degraded >= 0.80, unhealthy < 0.80. The bars stay shaped
// by qps, so volume + health read together — a slow degradation (success
// falling while volume holds) shifts the bars green → amber → red.
const HEALTHY_SUCCESS_THRESHOLD = 0.95
const DEGRADED_SUCCESS_THRESHOLD = 0.8

type SparkHealth = 'healthy' | 'degraded' | 'unhealthy' | 'idle'

const SPARK_HEALTH_TONE: Record<SparkHealth, string> = {
  healthy: 'bg-success/70',
  degraded: 'bg-warning/70',
  unhealthy: 'bg-destructive/70',
  idle: 'bg-muted-foreground/30',
}

const SPARK_HEALTH_LABEL_KEY: Record<SparkHealth, string> = {
  healthy: 'dashboard.availability.realtime.healthHealthy',
  degraded: 'dashboard.availability.realtime.healthDegraded',
  unhealthy: 'dashboard.availability.realtime.healthUnhealthy',
  idle: 'dashboard.availability.realtime.healthIdle',
}

const IDLE_SPARK_POINT: RealtimeOpsSamplePoint = { qps: 0, successRate: 0 }

function sparkHealthFor(point: RealtimeOpsSamplePoint): SparkHealth {
  if (point.qps <= 0) return 'idle'
  if (point.successRate >= HEALTHY_SUCCESS_THRESHOLD) return 'healthy'
  if (point.successRate >= DEGRADED_SUCCESS_THRESHOLD) return 'degraded'
  return 'unhealthy'
}

function RealtimeSparkline({ points }: { points: RealtimeOpsSamplePoint[] }) {
  const { t } = useTranslation()
  const max = Math.max(1, ...points.map((point) => point.qps))
  const bars =
    points.length > 0
      ? points
      : Array.from({ length: SPARK_BARS }, () => IDLE_SPARK_POINT)
  const latestHealth = sparkHealthFor(points.at(-1) ?? IDLE_SPARK_POINT)

  return (
    <div
      role='img'
      aria-label={t(SPARK_HEALTH_LABEL_KEY[latestHealth])}
      className='flex h-16 w-full items-end gap-px'
    >
      {bars.map((point, index) => {
        const ratio = Math.max(0.04, point.qps / max)
        const health = sparkHealthFor(point)
        return (
          <div
            // eslint-disable-next-line react/no-array-index-key
            key={index}
            className={cn(
              'min-w-0 flex-1 rounded-sm',
              SPARK_HEALTH_TONE[health]
            )}
            style={{ height: `${ratio * 100}%` }}
          />
        )
      })}
    </div>
  )
}

function RealtimeOpsPanel() {
  const { t } = useTranslation()
  const sample = useRealtimeOps()

  const tone = sample.gaveUp
    ? 'border-destructive/40 bg-destructive/5'
    : sample.connected
      ? 'border-success/40 bg-success/5'
      : 'border-border'

  return (
    <Card className={cn(tone)}>
      <CardHeader className='pb-2'>
        <CardTitle className='flex items-center justify-between text-sm font-medium'>
          <span className='flex items-center gap-2'>
            <Radio className='size-4' />
            {t('dashboard.availability.realtime.title')}
          </span>
          <span
            className={cn(
              'flex items-center gap-1.5 text-xs font-normal',
              sample.connected
                ? 'text-success'
                : sample.gaveUp
                  ? 'text-destructive'
                  : 'text-muted-foreground'
            )}
          >
            <span
              className={cn(
                'size-2 rounded-full',
                sample.connected
                  ? 'bg-success'
                  : sample.gaveUp
                    ? 'bg-destructive'
                    : 'animate-pulse bg-muted-foreground/50'
              )}
            />
            {sample.gaveUp
              ? t('dashboard.availability.realtime.statusDisconnected')
              : sample.connected
                ? t('dashboard.availability.realtime.statusLive')
                : t('dashboard.availability.realtime.statusConnecting')}
          </span>
        </CardTitle>
        <CardDescription className='text-xs'>
          {t('dashboard.availability.realtime.description')}
        </CardDescription>
      </CardHeader>
      <CardContent className='space-y-3'>
        <div className='flex flex-wrap items-end justify-between gap-x-4 gap-y-2'>
          <div>
            <div className='text-muted-foreground text-xs'>
              {t('dashboard.availability.realtime.metricQps')}
            </div>
            <div className='text-2xl font-semibold tabular-nums'>
              {sample.qps}
            </div>
          </div>
          <div>
            <div className='text-muted-foreground text-xs'>
              {t('dashboard.availability.realtime.metricSuccess')}
            </div>
            <div className='text-2xl font-semibold tabular-nums'>
              {formatRate(sample.successRate)}
            </div>
          </div>
          <div>
            <div className='text-muted-foreground text-xs'>
              {t('dashboard.availability.realtime.metricUptime')}
            </div>
            <div className='text-2xl font-semibold tabular-nums'>
              {Math.floor(sample.lifetime / 60)}m
            </div>
          </div>
        </div>
        <RealtimeSparkline points={sample.spark} />
      </CardContent>
    </Card>
  )
}

function AttentionPanel() {
  const { t } = useTranslation()
  const { data, isLoading, isError } = useQuery({
    queryKey: ['dashboard-attention', 20],
    queryFn: () => api.getAttention(20) as Promise<AttentionResponse>,
    // Auto-refresh the attention panel so expired accounts / low balances
    // surface without a manual reload (the realtime QPS panel uses the ops
    // WebSocket stream above; this covers the REST half of availability).
    refetchInterval: 10 * 1000,
  })

  const items = data?.items ?? []

  return (
    <Card>
      <CardHeader>
        <CardTitle className='flex items-center gap-2 text-sm font-medium'>
          <ShieldCheck className='size-4' />
          {t('dashboard.availability.monitors.title')}
        </CardTitle>
        <CardDescription className='text-xs'>
          {t('dashboard.availability.monitors.description')}
        </CardDescription>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <Skeleton className='h-48 w-full rounded-md' />
        ) : isError ? (
          <div className='flex min-h-48 flex-col items-center justify-center gap-2 rounded-lg border border-dashed py-8 text-center'>
            <TriangleAlert className='text-destructive/80 size-5' />
            <p className='text-destructive text-xs'>
              {t('dashboard.availability.monitors.loadError')}
            </p>
          </div>
        ) : items.length === 0 ? (
          <div className='flex min-h-48 flex-col items-center justify-center gap-2 rounded-lg border border-dashed py-8 text-center'>
            <Inbox className='text-muted-foreground/60 size-5' />
            <p className='text-muted-foreground text-sm'>
              {t('dashboard.availability.monitors.empty')}
            </p>
          </div>
        ) : (
          <ul className='space-y-2'>
            {items.map((item, index) => {
              const tone = SEVERITY_TONE[item.severity] ?? SEVERITY_TONE.info
              const label = t(
                `dashboard.availability.monitors.severity.${item.severity}`
              )
              return (
                <li
                  // eslint-disable-next-line react/no-array-index-key
                  key={`${item.target}-${index}`}
                  className='flex items-start gap-3'
                >
                  <Badge variant={tone.variant} className='mt-1 shrink-0'>
                    <span className={cn('size-1.5 rounded-full', tone.dot)} />
                    {label}
                  </Badge>
                  <div className='min-w-0 flex-1'>
                    {item.target ? (
                      <a
                        href={item.target}
                        className='block truncate text-sm hover:underline'
                      >
                        {item.label}
                      </a>
                    ) : (
                      <span className='block truncate text-sm'>
                        {item.label}
                      </span>
                    )}
                  </div>
                </li>
              )
            })}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}

export function AvailabilitySection() {
  return (
    <div className='grid grid-cols-1 gap-4 lg:grid-cols-2'>
      <RealtimeOpsPanel />
      <AttentionPanel />
    </div>
  )
}
