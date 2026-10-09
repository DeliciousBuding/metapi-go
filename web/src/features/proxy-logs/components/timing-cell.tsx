// Proxy-attempt timing. First-byte latency measures response headers (TTFB),
// not a generated token; total latency includes reading/relaying the body.
import { useTranslation } from 'react-i18next'

import { formatLatency } from '@/lib/format'
import { cn } from '@/lib/utils'

const LATENCY_FORMAT = {
  autoSeconds: true,
  spaced: true,
  secondsDigits: 2,
  wholeSecondsThreshold: 100,
} as const

export type TimingCellProps = {
  latencyMs: number | null | undefined
  firstByteLatencyMs?: number | null
  firstOutputLatencyMs?: number | null
  isStream?: boolean | null
  className?: string
}

export function TimingCell(props: TimingCellProps) {
  const { t } = useTranslation()
  const total = props.latencyMs
  const first = props.firstByteLatencyMs
  const hasTotal =
    typeof total === 'number' && Number.isFinite(total) && total >= 0
  const hasFirst =
    typeof first === 'number' &&
    Number.isFinite(first) &&
    first >= 0 &&
    (!hasTotal || first <= total)
  const totalLabel = hasTotal ? formatLatency(total, LATENCY_FORMAT) : '—'
  const firstLabel = hasFirst ? formatLatency(first, LATENCY_FORMAT) : '—'
  const output = props.firstOutputLatencyMs
  const hasOutput =
    typeof output === 'number' &&
    Number.isFinite(output) &&
    output >= 0 &&
    (!hasTotal || output <= total) &&
    (!hasFirst || output >= first)
  const outputLabel = hasOutput ? formatLatency(output, LATENCY_FORMAT) : '—'
  let timingTone = 'border-muted-foreground/30'
  let perceivedLatency: number | undefined
  if (props.isStream) {
    if (hasOutput) perceivedLatency = output
  } else if (hasFirst) {
    perceivedLatency = first
  }
  if (perceivedLatency !== undefined) {
    timingTone = perceivedLatency >= 5000 ? 'border-warning' : 'border-success'
  }
  const ariaLabel = hasFirst
    ? t('proxyLogs.timing.ariaFirstByte', {
        firstByte: firstLabel,
        total: totalLabel,
      })
    : t('proxyLogs.timing.ariaTotal', { total: totalLabel })

  return (
    <div
      role='group'
      aria-label={
        props.isStream
          ? `${t('proxyLogs.timing.firstOutput')} ${outputLabel}, ${ariaLabel}`
          : ariaLabel
      }
      className={cn(
        'grid w-fit grid-cols-[auto_auto] gap-x-3 gap-y-0.5 border-s-2 ps-2 text-xs leading-4 tabular-nums',
        timingTone,
        props.className
      )}
    >
      {props.isStream && (
        <>
          <span
            className='text-muted-foreground'
            title={t('proxyLogs.timing.outputHint')}
          >
            {t('proxyLogs.timing.firstOutput')}
          </span>
          <span className='text-end font-medium whitespace-nowrap'>
            {outputLabel}
          </span>
        </>
      )}
      <span
        className='text-muted-foreground'
        title={t('proxyLogs.timing.headerHint')}
      >
        {t('proxyLogs.timing.firstByte')}
      </span>
      <span className='text-end font-medium whitespace-nowrap'>
        {firstLabel}
      </span>
      <span className='text-muted-foreground'>
        {t('proxyLogs.timing.total')}
      </span>
      <span className='text-end font-medium whitespace-nowrap'>
        {totalLabel}
      </span>
    </div>
  )
}
