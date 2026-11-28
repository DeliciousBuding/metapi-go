/* eslint-disable no-nested-ternary -- connection-tone uses chained ternaries */
import { Radio, TriangleAlert } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Card,
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
import { KpiValue } from '@/components/ui/kpi-value'
import { toBcp47 } from '@/i18n/languages'
import { formatTimeOfDay } from '@/lib/format'
import { cn } from '@/lib/utils'

import { useRealtimeOps } from '../../hooks/use-realtime-ops'
import { RealtimeSparkline } from './realtime-sparkline'

function formatRate(rate: number): string {
  return `${(rate * 100).toFixed(1)}%`
}

/**
 * Escalate the realtime uptime from minutes to hours/days once the value
 * outgrows the unit (a multi-day session must not render "4320m").
 */
function formatUptime(
  seconds: number,
  t: (key: string, options?: Record<string, unknown>) => string
): string {
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) {
    return t('dashboard.availability.realtime.uptimeMinutes', {
      value: minutes,
    })
  }
  const hours = minutes / 60
  if (hours < 24) {
    return t('dashboard.availability.realtime.uptimeHours', {
      value: Number(hours.toFixed(1)),
    })
  }
  return t('dashboard.availability.realtime.uptimeDays', {
    value: Number((hours / 24).toFixed(1)),
  })
}

export function RealtimeOpsPanel() {
  const { t, i18n } = useTranslation()
  const locale = toBcp47(i18n.language || 'en')
  const { sample, reconnect, lastFrameAt } = useRealtimeOps()

  const tone = sample.gaveUp
    ? 'border-destructive/40 bg-destructive/5'
    : sample.connected
      ? 'border-success/40 bg-success/5'
      : 'border-border'

  return (
    <Card className={cn(tone)}>
      <CardHeader className='pb-2'>
        <CardTitle className='flex items-center justify-between'>
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
        {!sample.connected && lastFrameAt !== null ? (
          <p
            aria-live='polite'
            className='text-muted-foreground text-xs tabular-nums'
          >
            {t('dashboard.availability.realtime.dataAsOf', {
              time: formatTimeOfDay(lastFrameAt, locale),
            })}
          </p>
        ) : null}
      </CardHeader>
      <CardContent className='space-y-3'>
        {sample.gaveUp ? (
          <Empty className='border-destructive/40 min-h-32 border'>
            <EmptyHeader>
              <EmptyMedia
                variant='icon'
                className='bg-destructive/10 text-destructive-soft-fg'
              >
                <TriangleAlert />
              </EmptyMedia>
              <EmptyDescription className='text-destructive'>
                {t('dashboard.availability.realtime.connectionLost')}
              </EmptyDescription>
            </EmptyHeader>
            <EmptyContent>
              <Button variant='outline' size='sm' onClick={reconnect}>
                {t('dashboard.availability.realtime.reconnect')}
              </Button>
            </EmptyContent>
          </Empty>
        ) : (
          <>
            <div className='flex flex-wrap items-end justify-between gap-x-4 gap-y-2'>
              <div>
                <div className='text-muted-foreground text-xs'>
                  {t('dashboard.availability.realtime.metricQps')}
                </div>
                <KpiValue size='lg'>{sample.qps}</KpiValue>
              </div>
              <div>
                <div className='text-muted-foreground text-xs'>
                  {t('dashboard.availability.realtime.metricSuccess')}
                </div>
                <KpiValue size='lg'>{formatRate(sample.successRate)}</KpiValue>
              </div>
              <div>
                <div className='text-muted-foreground text-xs'>
                  {t('dashboard.availability.realtime.metricUptime')}
                </div>
                <KpiValue size='lg'>
                  {formatUptime(sample.uptimeSeconds, t)}
                </KpiValue>
              </div>
            </div>
            <RealtimeSparkline points={sample.spark} />
          </>
        )}
      </CardContent>
    </Card>
  )
}
