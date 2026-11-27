import { useTranslation } from 'react-i18next'

import {
  SoftToneBadge,
  type SoftTone,
} from '@/components/common/soft-tone-badge'
import { toBcp47 } from '@/i18n/languages'
import { formatInt, formatLatency } from '@/lib/format'

import type { ProxyLogStreamTiming } from '../types'

type StreamTimingCardProps = {
  streamTiming: ProxyLogStreamTiming
  latencyMs?: number | null
}

function isValidNonNeg(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0
}

function isPositiveFinite(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value) && value > 0
}

function getDecodeStatusMeta(decodeStatus: string | null | undefined): {
  tone: SoftTone
  labelKey: string
} {
  switch (decodeStatus) {
    case 'measured':
      return {
        tone: 'success',
        labelKey: 'proxyLogs.telemetry.streamTiming.status.measured',
      }
    case 'bufferedBurst':
      return {
        tone: 'warning',
        labelKey: 'proxyLogs.telemetry.streamTiming.status.bufferedBurst',
      }
    case 'hiddenReasoning':
      return {
        tone: 'info',
        labelKey: 'proxyLogs.telemetry.streamTiming.status.hiddenReasoning',
      }
    case 'interrupted':
      return {
        tone: 'destructive',
        labelKey: 'proxyLogs.telemetry.streamTiming.status.interrupted',
      }
    case 'insufficientEvidence':
      return {
        tone: 'neutral',
        labelKey:
          'proxyLogs.telemetry.streamTiming.status.insufficientEvidence',
      }
    default:
      return {
        tone: 'neutral',
        labelKey: 'proxyLogs.telemetry.streamTiming.status.unknown',
      }
  }
}

function getReasoningEvidenceKey(
  evidence: string | null | undefined
): string | null {
  switch (evidence) {
    case 'visible':
      return 'proxyLogs.telemetry.streamTiming.reasoningEvidence.visible'
    case 'hidden':
      return 'proxyLogs.telemetry.streamTiming.reasoningEvidence.hidden'
    case 'summaryOnly':
      return 'proxyLogs.telemetry.streamTiming.reasoningEvidence.summaryOnly'
    case 'none':
      return null
    default:
      return null
  }
}

export type StreamTimingStage = {
  key: 'header' | 'reasoning' | 'textWait' | 'stream'
  labelKey: string
  colorClass: string
  durationMs: number
  timestampMs: number
}

export function buildStreamTimingStages(
  streamTiming: ProxyLogStreamTiming,
  latencyMs?: number | null
): { stages: StreamTimingStage[]; totalMs: number } | null {
  const headerMs = isValidNonNeg(streamTiming.headerMs)
    ? streamTiming.headerMs
    : null
  const firstReasoningMs = isValidNonNeg(streamTiming.firstReasoningMs)
    ? streamTiming.firstReasoningMs
    : null
  const firstTextMs = isValidNonNeg(streamTiming.firstTextMs)
    ? streamTiming.firstTextMs
    : null
  const lastContentMs = isValidNonNeg(streamTiming.lastContentMs)
    ? streamTiming.lastContentMs
    : null
  const fallbackTotal = isPositiveFinite(latencyMs) ? latencyMs : null

  const endMs = Math.max(
    lastContentMs ?? 0,
    firstTextMs ?? 0,
    firstReasoningMs ?? 0,
    headerMs ?? 0,
    fallbackTotal ?? 0
  )
  if (endMs <= 0) return null

  const milestones: Array<{
    key: StreamTimingStage['key']
    labelKey: string
    colorClass: string
    atMs: number
  }> = []

  if (headerMs !== null) {
    milestones.push({
      key: 'header',
      labelKey: 'proxyLogs.telemetry.streamTiming.stage.header',
      colorClass: 'bg-info',
      atMs: headerMs,
    })
  }
  if (firstReasoningMs !== null) {
    if (headerMs !== null && firstReasoningMs < headerMs) return null
    milestones.push({
      key: 'reasoning',
      labelKey: 'proxyLogs.telemetry.streamTiming.stage.firstReasoning',
      colorClass: 'bg-chart-3',
      atMs: firstReasoningMs,
    })
  }
  if (firstTextMs !== null) {
    const prevAnchor = firstReasoningMs ?? headerMs
    if (prevAnchor !== null && firstTextMs < prevAnchor) return null
    milestones.push({
      key: 'textWait',
      labelKey: 'proxyLogs.telemetry.streamTiming.stage.firstText',
      colorClass: 'bg-warning',
      atMs: firstTextMs,
    })
  }

  const streamEnd = lastContentMs ?? endMs
  const lastMilestoneMs = milestones.at(-1)?.atMs ?? 0
  if (lastContentMs !== null && lastContentMs < lastMilestoneMs) return null

  if (milestones.length === 0) return null

  const stages: StreamTimingStage[] = []
  let cursor = 0
  for (const milestone of milestones) {
    const delta = Math.max(0, milestone.atMs - cursor)
    stages.push({
      key: milestone.key,
      labelKey: milestone.labelKey,
      colorClass: milestone.colorClass,
      durationMs: delta,
      timestampMs: milestone.atMs,
    })
    cursor = milestone.atMs
  }

  const streamDelta = Math.max(0, streamEnd - cursor)
  if (streamDelta > 0 || lastContentMs !== null) {
    stages.push({
      key: 'stream',
      labelKey: 'proxyLogs.telemetry.streamTiming.stage.streaming',
      colorClass: 'bg-success',
      durationMs: streamDelta,
      timestampMs: streamEnd,
    })
  }

  return { stages, totalMs: endMs }
}

export function StreamTimingCard({
  streamTiming,
  latencyMs,
}: StreamTimingCardProps) {
  const { t, i18n } = useTranslation()
  const locale = toBcp47(i18n.language || 'en')

  const statusMeta = getDecodeStatusMeta(streamTiming.decodeStatus)
  const reasoningEvidenceKey = getReasoningEvidenceKey(
    streamTiming.reasoningEvidence
  )
  const stageResult = buildStreamTimingStages(streamTiming, latencyMs)

  const hasMeasuredTps =
    streamTiming.decodeStatus === 'measured' &&
    typeof streamTiming.decodeTps === 'number' &&
    Number.isFinite(streamTiming.decodeTps) &&
    streamTiming.decodeTps >= 0

  const tpsTitle =
    hasMeasuredTps &&
    isValidNonNeg(streamTiming.decodeTokens) &&
    isValidNonNeg(streamTiming.decodeWindowMs)
      ? t('proxyLogs.telemetry.streamTiming.decodeDetailTooltip', {
          tokens: formatInt(streamTiming.decodeTokens, locale),
          window: formatLatency(streamTiming.decodeWindowMs),
        })
      : undefined

  const hasMaxGap = isValidNonNeg(streamTiming.maxContentGapMs)

  return (
    <section
      className='bg-muted/20 space-y-2.5 rounded-lg border p-3'
      data-testid='stream-timing-card'
    >
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <h4 className='text-xs font-medium'>
          {t('proxyLogs.telemetry.streamTiming.title')}
        </h4>
        <div className='flex flex-wrap items-center gap-1.5'>
          {reasoningEvidenceKey && (
            <span className='bg-muted text-muted-foreground rounded-4xl border px-1.5 py-0.5 text-xs'>
              {t(reasoningEvidenceKey)}
            </span>
          )}
          <SoftToneBadge tone={statusMeta.tone}>
            {t(statusMeta.labelKey)}
          </SoftToneBadge>
        </div>
      </div>

      {stageResult && stageResult.stages.length > 0 && (
        <div className='space-y-1.5'>
          <div className='bg-muted flex h-2.5 w-full overflow-hidden rounded-sm'>
            {stageResult.stages.map((stage) => {
              const pct =
                stageResult.totalMs > 0
                  ? (stage.durationMs / stageResult.totalMs) * 100
                  : 0
              if (pct <= 0) return null
              return (
                <div
                  key={stage.key}
                  data-stage={stage.key}
                  className={`${stage.colorClass} h-full`}
                  style={{ width: `${Math.max(1.5, pct).toFixed(2)}%` }}
                  title={`${t(stage.labelKey)}: ${formatLatency(stage.timestampMs)} (+${formatLatency(stage.durationMs)})`}
                />
              )
            })}
          </div>
          <div className='text-muted-foreground flex flex-wrap gap-x-3 gap-y-1 text-xs tabular-nums'>
            {stageResult.stages.map((stage) => (
              <span key={stage.key} className='inline-flex items-center gap-1'>
                <span
                  className={`${stage.colorClass} size-2 rounded-xs`}
                  aria-hidden='true'
                />
                {t(stage.labelKey)}:{' '}
                {stage.key === 'stream'
                  ? formatLatency(stage.durationMs)
                  : formatLatency(stage.timestampMs)}
              </span>
            ))}
          </div>
        </div>
      )}

      <div className='divide-border grid grid-cols-2 divide-x border-t pt-2 text-xs sm:grid-cols-3'>
        <div className='pr-2'>
          <span className='text-muted-foreground block'>
            {t('proxyLogs.telemetry.streamTiming.decodeTps')}
          </span>
          {hasMeasuredTps ? (
            <span
              className='font-medium tabular-nums'
              title={tpsTitle}
              data-testid='stream-timing-tps'
            >
              {t('proxyLogs.telemetry.streamTiming.tpsValue', {
                value: streamTiming.decodeTps?.toFixed(1),
              })}
            </span>
          ) : (
            <span
              className='text-muted-foreground tabular-nums'
              data-testid='stream-timing-tps-unavailable'
            >
              {t('requestLogs.unknown')}
            </span>
          )}
        </div>

        <div className='px-2'>
          <span className='text-muted-foreground block'>
            {t('proxyLogs.telemetry.streamTiming.decodeTokens')}
          </span>
          <span className='font-medium tabular-nums'>
            {isValidNonNeg(streamTiming.decodeTokens)
              ? formatInt(streamTiming.decodeTokens, locale)
              : t('requestLogs.unknown')}
            {isValidNonNeg(streamTiming.decodeWindowMs) ? (
              <span className='text-muted-foreground font-normal'>
                {' '}
                / {formatLatency(streamTiming.decodeWindowMs)}
              </span>
            ) : null}
          </span>
        </div>

        {hasMaxGap && (
          <div className='col-span-2 mt-1.5 border-t pt-1.5 sm:col-span-1 sm:mt-0 sm:border-t-0 sm:pt-0 sm:pl-2'>
            <span className='text-muted-foreground block'>
              {t('proxyLogs.telemetry.streamTiming.maxGap')}
            </span>
            <span
              className='font-medium tabular-nums'
              data-testid='stream-timing-max-gap'
            >
              {formatLatency(streamTiming.maxContentGapMs)}
            </span>
          </div>
        )}
      </div>
    </section>
  )
}
