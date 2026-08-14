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
  { dot: string; badge: string }
> = {
  critical: {
    dot: 'bg-destructive',
    badge: 'border-destructive/40 bg-destructive/10 text-destructive-soft-fg',
  },
  warning: {
    dot: 'bg-warning',
    badge: 'border-warning/40 bg-warning/10 text-warning',
  },
  info: {
    dot: 'bg-info',
    badge: 'border-info/40 bg-info/10 text-info',
  },
}

function formatRate(rate: number): string {
  return `${(rate * 100).toFixed(1)}%`
}

function RealtimeSparkline({ samples }: { samples: number[] }) {
  const max = Math.max(1, ...samples)
  const bars =
    samples.length > 0 ? samples : Array.from({ length: SPARK_BARS }, () => 0)

  return (
    <div className='flex h-16 w-full items-end gap-px' aria-hidden='true'>
      {bars.map((sample, index) => {
        const ratio = Math.max(0.04, sample / max)
        return (
          <div
            // eslint-disable-next-line react/no-array-index-key
            key={index}
            className='bg-primary/70 min-w-0 flex-1 rounded-sm'
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
        <RealtimeSparkline samples={sample.spark} />
      </CardContent>
    </Card>
  )
}

function AttentionPanel() {
  const { t } = useTranslation()
  const { data, isLoading, isError } = useQuery({
    queryKey: ['dashboard-attention', 20],
    queryFn: () => api.getAttention(20) as Promise<AttentionResponse>,
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
                  <span
                    className={cn(
                      'mt-1 inline-flex h-5 shrink-0 items-center gap-1 rounded-full border px-2 text-xs font-medium',
                      tone.badge
                    )}
                  >
                    <span className={cn('size-1.5 rounded-full', tone.dot)} />
                    {label}
                  </span>
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
