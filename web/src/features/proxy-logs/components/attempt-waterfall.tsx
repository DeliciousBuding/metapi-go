import { useTranslation } from 'react-i18next'

import { formatLatency } from '@/lib/format'

type AttemptWaterfallProps = {
  latencyMs: number | null | undefined
  firstByteLatencyMs?: number | null
  firstOutputLatencyMs?: number | null
  isStream?: boolean | null
  scaleMaxMs: number
}

function isValidNonNeg(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0
}

export function AttemptWaterfall({
  latencyMs,
  firstByteLatencyMs,
  firstOutputLatencyMs,
  isStream,
  scaleMaxMs,
}: AttemptWaterfallProps) {
  const { t } = useTranslation()

  const total = isValidNonNeg(latencyMs) && latencyMs > 0 ? latencyMs : null
  if (total === null || scaleMaxMs <= 0) {
    return (
      <div
        className='bg-muted/30 text-muted-foreground rounded-md border border-dashed px-2.5 py-1.5 text-xs'
        data-testid='attempt-waterfall-unknown'
      >
        {t('requestLogs.waterfall.unknownTotal')}
      </div>
    )
  }

  const firstByteValid =
    isValidNonNeg(firstByteLatencyMs) && firstByteLatencyMs <= total
  const firstOutputProvided =
    firstOutputLatencyMs !== null && firstOutputLatencyMs !== undefined
  const firstOutputValid =
    isValidNonNeg(firstOutputLatencyMs) &&
    firstOutputLatencyMs <= total &&
    (!firstByteValid || firstOutputLatencyMs >= firstByteLatencyMs)

  const hasInconsistentBoundary =
    (firstByteLatencyMs !== null &&
      firstByteLatencyMs !== undefined &&
      !firstByteValid) ||
    (firstOutputProvided && !firstOutputValid)

  const widthPct = (ms: number) =>
    `${Math.max(2, Math.min(100, (ms / scaleMaxMs) * 100)).toFixed(2)}%`

  // Never split segments when boundaries are missing or inverted.
  if (!firstByteValid || hasInconsistentBoundary) {
    return (
      <div className='space-y-1.5' data-testid='attempt-waterfall'>
        <div className='bg-muted h-2.5 w-full overflow-hidden rounded-sm'>
          <div
            className='bg-neutral h-full'
            style={{ width: widthPct(total) }}
          />
        </div>
        <p className='text-muted-foreground text-xs'>
          {t('requestLogs.waterfall.unknownBoundary')} ·{' '}
          <span className='tabular-nums'>{formatLatency(total)}</span>
        </p>
      </div>
    )
  }

  const ttfbMs = firstByteLatencyMs
  const totalWidth = Math.max(2, Math.min(100, (total / scaleMaxMs) * 100))

  if (firstOutputValid) {
    const waitOutputMs = Math.max(0, firstOutputLatencyMs - ttfbMs)
    const bodyMs = Math.max(0, total - firstOutputLatencyMs)
    const ttfbShare = (ttfbMs / total) * totalWidth
    const waitShare = (waitOutputMs / total) * totalWidth
    const bodyShare = Math.max(0, totalWidth - ttfbShare - waitShare)

    return (
      <div className='space-y-1.5' data-testid='attempt-waterfall'>
        <div className='bg-muted flex h-2.5 w-full overflow-hidden rounded-sm'>
          {ttfbShare > 0 && (
            <div
              className='bg-info h-full'
              style={{ width: `${ttfbShare.toFixed(2)}%` }}
            />
          )}
          {waitShare > 0 && (
            <div
              className='bg-warning h-full'
              style={{ width: `${waitShare.toFixed(2)}%` }}
            />
          )}
          {bodyShare > 0 && (
            <div
              className='bg-success h-full'
              style={{ width: `${bodyShare.toFixed(2)}%` }}
            />
          )}
        </div>
        <div className='text-muted-foreground flex flex-wrap gap-x-3 gap-y-1 text-xs tabular-nums'>
          <span className='inline-flex items-center gap-1'>
            <span className='bg-info size-2 rounded-xs' aria-hidden='true' />
            {t('requestLogs.waterfall.ttfb')}: {formatLatency(ttfbMs)}
          </span>
          <span className='inline-flex items-center gap-1'>
            <span className='bg-warning size-2 rounded-xs' aria-hidden='true' />
            {t('requestLogs.waterfall.firstOutput')}:{' '}
            {formatLatency(firstOutputLatencyMs)}
          </span>
          <span className='inline-flex items-center gap-1'>
            <span className='bg-success size-2 rounded-xs' aria-hidden='true' />
            {t('requestLogs.waterfall.generation')}: {formatLatency(bodyMs)}
          </span>
        </div>
      </div>
    )
  }

  // firstByte is valid, firstOutput was not recorded (null/undefined).
  const restMs = Math.max(0, total - ttfbMs)
  const ttfbShare = (ttfbMs / total) * totalWidth
  const restShare = Math.max(0, totalWidth - ttfbShare)

  return (
    <div className='space-y-1.5' data-testid='attempt-waterfall'>
      <div className='bg-muted flex h-2.5 w-full overflow-hidden rounded-sm'>
        {ttfbShare > 0 && (
          <div
            className='bg-info h-full'
            style={{ width: `${ttfbShare.toFixed(2)}%` }}
          />
        )}
        {restShare > 0 && (
          <div
            className={
              isStream === false ? 'bg-success h-full' : 'bg-neutral h-full'
            }
            style={{ width: `${restShare.toFixed(2)}%` }}
          />
        )}
      </div>
      <div className='text-muted-foreground flex flex-wrap gap-x-3 gap-y-1 text-xs tabular-nums'>
        <span className='inline-flex items-center gap-1'>
          <span className='bg-info size-2 rounded-xs' aria-hidden='true' />
          {t('requestLogs.waterfall.ttfb')}: {formatLatency(ttfbMs)}
        </span>
        {isStream !== false && (
          <span>
            {t('requestLogs.waterfall.firstOutput')}: {t('requestLogs.unknown')}
          </span>
        )}
        <span className='inline-flex items-center gap-1'>
          <span
            className={
              isStream === false
                ? 'bg-success size-2 rounded-xs'
                : 'bg-neutral size-2 rounded-xs'
            }
            aria-hidden='true'
          />
          {t('requestLogs.waterfall.generation')}: {formatLatency(restMs)}
        </span>
      </div>
    </div>
  )
}
