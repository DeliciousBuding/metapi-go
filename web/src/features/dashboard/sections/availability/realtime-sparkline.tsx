import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

import type { RealtimeOpsSamplePoint } from '../../types'

const SPARK_BARS = 60

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

export function RealtimeSparkline({
  points,
}: {
  points: RealtimeOpsSamplePoint[]
}) {
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
