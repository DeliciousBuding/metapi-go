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

  // One line carries the two numbers an operator scans for: the total, then
  // the perceived first byte (streams: first generated output). The full
  // three-way breakdown stays in the tooltip/aria label and the detail sheet.
  const secondaryLabel = props.isStream
    ? t('proxyLogs.timing.firstOutput')
    : t('proxyLogs.timing.firstByte')
  const secondaryValue = props.isStream ? outputLabel : firstLabel

  return (
    <div
      role='group'
      title={
        props.isStream
          ? `${t('proxyLogs.timing.firstOutput')} ${outputLabel} · ${ariaLabel}`
          : ariaLabel
      }
      aria-label={
        props.isStream
          ? `${t('proxyLogs.timing.firstOutput')} ${outputLabel}, ${ariaLabel}`
          : ariaLabel
      }
      className={cn(
        'flex min-w-0 items-baseline gap-1.5 border-s-2 ps-2 text-xs leading-4 tabular-nums',
        timingTone,
        props.className
      )}
    >
      <span className='font-medium whitespace-nowrap'>{totalLabel}</span>
      {secondaryValue !== '—' && (
        <span
          className='text-muted-foreground text-2xs whitespace-nowrap'
          title={
            props.isStream
              ? t('proxyLogs.timing.outputHint')
              : t('proxyLogs.timing.headerHint')
          }
        >
          {secondaryLabel} {secondaryValue}
        </span>
      )}
    </div>
  )
}
