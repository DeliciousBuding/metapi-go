// metapi-go/features/proxy-logs/components — latency badge.
//
// Colours the request latency by bucket so the list and the detail sheet
// share one visual vocabulary:
//   < 500ms  → success (green) — healthy
//   < 2000ms → warning (amber) — slow
//   >= 2000ms→ destructive (red) — unhealthy
// `null` / `undefined` / negative latencies render as a muted dash.

import { cn } from '@/lib/utils'

const LATENCY_TIERS = {
  fast: {
    thresholdMs: 500,
    className: 'bg-success/10 text-success border-success/30',
    dotClassName: 'bg-success',
  },
  slow: {
    thresholdMs: 2000,
    className: 'bg-warning/10 text-warning border-warning/30',
    dotClassName: 'bg-warning',
  },
  unhealthy: {
    thresholdMs: Number.POSITIVE_INFINITY,
    className: 'bg-destructive/10 text-destructive border-destructive/30',
    dotClassName: 'bg-destructive',
  },
} as const

function resolveTier(latencyMs: number) {
  if (latencyMs < LATENCY_TIERS.fast.thresholdMs) return LATENCY_TIERS.fast
  if (latencyMs < LATENCY_TIERS.slow.thresholdMs) return LATENCY_TIERS.slow
  return LATENCY_TIERS.unhealthy
}

function formatLatency(latencyMs: number): string {
  if (latencyMs < 1000) return `${Math.round(latencyMs)} ms`
  const seconds = latencyMs / 1000
  return seconds >= 100
    ? `${seconds.toFixed(0)} s`
    : `${seconds.toFixed(2)} s`
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
  if (latencyMs === null || latencyMs === undefined || latencyMs < 0) {
    return <span className={cn('text-muted-foreground text-sm', className)}>—</span>
  }

  const tier = resolveTier(latencyMs)
  const label = formatLatency(latencyMs)
  const title =
    firstByteLatencyMs !== null && firstByteLatencyMs !== undefined && firstByteLatencyMs >= 0
      ? `总延迟 ${label} · 首字节 ${formatLatency(firstByteLatencyMs)}`
      : `总延迟 ${label}`

  return (
    <span
      title={title}
      className={cn(
        'inline-flex w-fit items-center gap-1 rounded-4xl border px-1.5 py-0.5 text-xs font-medium tabular-nums whitespace-nowrap',
        tier.className,
        className,
      )}
    >
      {showDot && (
        <span className={cn('inline-block size-1.5 rounded-full', tier.dotClassName)} aria-hidden='true' />
      )}
      {label}
    </span>
  )
}
