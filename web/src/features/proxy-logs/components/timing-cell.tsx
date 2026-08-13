// metapi-go/features/proxy-logs/components — timing cell.
//
// Two-segment latency bar: the left segment is time-to-first-byte and the
// right segment is the remaining transfer time, so an operator sees both
// "first token" and "total" in one compact cell. A Slow marker (text, not
// color-only) appears when total latency crosses the slow threshold. All
// segment colors are token-based (success/destructive/muted-foreground).

import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

const SLOW_THRESHOLD_MS = 2000

function formatDuration(ms: number): string {
  if (ms < 1000) return `${Math.round(ms)} ms`
  const seconds = ms / 1000
  return seconds >= 100 ? `${seconds.toFixed(0)} s` : `${seconds.toFixed(2)} s`
}

export type TimingCellProps = {
  latencyMs: number | null | undefined
  firstByteLatencyMs?: number | null
  className?: string
}

export function TimingCell({
  latencyMs,
  firstByteLatencyMs,
  className,
}: TimingCellProps) {
  const { t } = useTranslation()
  if (latencyMs === null || latencyMs === undefined || latencyMs < 0) {
    return (
      <span className={cn('text-muted-foreground text-sm', className)}>—</span>
    )
  }

  const total = Math.max(1, latencyMs)
  const hasFirstByte =
    firstByteLatencyMs !== null &&
    firstByteLatencyMs !== undefined &&
    firstByteLatencyMs >= 0
  const firstByte = hasFirstByte ? Math.min(firstByteLatencyMs, total) : 0
  const firstBytePct = (firstByte / total) * 100
  const isSlow = latencyMs >= SLOW_THRESHOLD_MS

  const ariaLabel = hasFirstByte
    ? t('proxyLogs.timing.ariaFirstByte', {
        firstByte: formatDuration(firstByte),
        total: formatDuration(latencyMs),
      })
    : t('proxyLogs.timing.ariaTotal', { total: formatDuration(latencyMs) })

  return (
    <div className={cn('flex w-fit items-center gap-2', className)}>
      <span className='text-xs font-medium tabular-nums whitespace-nowrap'>
        {formatDuration(latencyMs)}
      </span>
      <span
        role='img'
        aria-label={ariaLabel}
        className={cn(
          'inline-flex h-1.5 w-16 shrink-0 items-stretch overflow-hidden rounded-full',
          'bg-muted'
        )}
      >
        <span
          className={cn('h-full', isSlow ? 'bg-destructive' : 'bg-success')}
          style={{ width: `${firstBytePct}%` }}
        />
        <span
          className={cn(
            'h-full flex-1',
            isSlow ? 'bg-destructive/40' : 'bg-muted-foreground/30'
          )}
        />
      </span>
      {isSlow ? (
        <span
          className={cn(
            'border-destructive/30 bg-destructive/10 text-destructive-soft-fg',
            'rounded-sm border px-1 py-px text-[10px] font-medium whitespace-nowrap'
          )}
        >
          {t('proxyLogs.timing.slow')}
        </span>
      ) : null}
    </div>
  )
}