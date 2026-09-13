// metapi-go/features/proxy-logs/components — latency badge.
// i18n: title attributes resolved via t().

import { Clock, TriangleAlert, Zap, type LucideIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import {
  SoftToneBadge,
  SoftToneDot,
  type SoftTone,
} from '@/components/common/soft-tone-badge'
import { formatLatency } from '@/lib/format'
import { cn } from '@/lib/utils'

const LATENCY_TIERS = {
  fast: {
    thresholdMs: 500,
    tone: 'success',
    icon: Zap,
  },
  slow: {
    thresholdMs: 2000,
    tone: 'warning',
    icon: Clock,
  },
  unhealthy: {
    thresholdMs: Number.POSITIVE_INFINITY,
    tone: 'destructive',
    icon: TriangleAlert,
  },
} as const satisfies Record<
  string,
  { thresholdMs: number; tone: SoftTone; icon: LucideIcon }
>

function resolveTier(latencyMs: number) {
  if (latencyMs < LATENCY_TIERS.fast.thresholdMs) return LATENCY_TIERS.fast
  if (latencyMs < LATENCY_TIERS.slow.thresholdMs) return LATENCY_TIERS.slow
  return LATENCY_TIERS.unhealthy
}

export type LatencyBadgeProps = {
  latencyMs: number | null | undefined
  firstByteLatencyMs?: number | null
  className?: string
  showDot?: boolean
}

export function LatencyBadge({
  latencyMs,
  firstByteLatencyMs,
  className,
  showDot = false,
}: LatencyBadgeProps) {
  const { t } = useTranslation()
  if (latencyMs === null || latencyMs === undefined || latencyMs < 0) {
    return (
      <span className={cn('text-muted-foreground text-sm', className)}>—</span>
    )
  }
  const tier = resolveTier(latencyMs)
  const LatencyIcon = tier.icon
  // Matches the legacy local formatter: ms below 1s, seconds above, dropping
  // decimals once the value reaches 100s (100.0 → "100 s").
  const secondsFormat = {
    autoSeconds: true,
    spaced: true,
    secondsDigits: 2,
    wholeSecondsThreshold: 100,
  }
  const label = formatLatency(latencyMs, secondsFormat)
  const title =
    firstByteLatencyMs !== null &&
    firstByteLatencyMs !== undefined &&
    firstByteLatencyMs >= 0
      ? t('proxyLogs.latency.totalAndFirstByte', {
          label,
          firstByte: formatLatency(firstByteLatencyMs, secondsFormat),
        })
      : t('proxyLogs.latency.totalLatency', { label })
  return (
    <SoftToneBadge
      tone={tier.tone}
      showDot={false}
      title={title}
      className={className}
    >
      <LatencyIcon className='size-3' aria-hidden='true' />
      {showDot && <SoftToneDot tone={tier.tone} />}
      {label}
    </SoftToneBadge>
  )
}
