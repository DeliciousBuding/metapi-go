/* eslint-disable react/only-export-components -- summary helper + shared types are co-located with the component that owns them */
// metapi-go/components/common — row-level probe history health bar.
// Renders the recent background model-probe results of one channel/account
// as a compact strip of gapless non-empty chronological samples (one bar per
// real probe, oldest → newest left-to-right), with height/shape encoding so
// failure/inconclusive/skipped states stay distinguishable without relying on
// color alone, plus a detailed tooltip with window stats and latest probe
// time/latency. Purely presentational: batch data fetching lives in
// use-probe-history.ts.

import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { toBcp47 } from '@/i18n/languages'
import { formatAbsoluteDateTime, formatLatency } from '@/lib/format'
import { cn } from '@/lib/utils'

/** Shared probe vocabulary of scheduler/model_probe.go. */
type ProbeStatus = 'success' | 'failure' | 'inconclusive' | 'skipped'

export interface ProbeResult {
  /** model_probe_results primary key — stable React key for each bar. */
  id: number
  status: ProbeStatus
  latencyMs: number | null
  httpStatus: number | null
  errorText: string | null
  modelName: string
  createdAt: string
}

/** Entity id (channel or account) -> newest-first probe results. */
export type ProbeHistoryMap = Record<number, ProbeResult[]>

// Design-token colors + non-color height cues so each sample state remains
// readable in both light/dark OKLCH themes and monochrome views.
// `skipped` is deliberately faint and short — it carries no connectivity signal.
const STATUS_BAR_CLASS: Record<ProbeStatus, string> = {
  success: 'h-full bg-success',
  failure: 'h-full bg-destructive',
  inconclusive: 'h-3/4 bg-warning',
  skipped: 'h-1/2 bg-muted-foreground/40',
}

export interface ProbeSummary {
  total: number
  success: number
  failure: number
  inconclusive: number
  skipped: number
  /** Percentage with one decimal (e.g. 66.7). */
  successRatePct: number
  /** Mean of the results that recorded a latency; null when none did. */
  avgLatencyMs: number | null
  /** Newest sample timestamp (ISO) when available. */
  latestCreatedAt: string | null
  /** Newest sample latency (ms) when recorded. */
  latestLatencyMs: number | null
  /** Newest sample status when available. */
  latestStatus: ProbeStatus | null
}

function isValidProbeSample(result: ProbeResult | null | undefined): boolean {
  return Boolean(result && typeof result.status === 'string')
}

export function summarizeProbeResults(results: ProbeResult[]): ProbeSummary {
  const valid = results.filter(isValidProbeSample)
  const total = valid.length
  const success = valid.filter((r) => r.status === 'success').length
  const failure = valid.filter((r) => r.status === 'failure').length
  const inconclusive = valid.filter((r) => r.status === 'inconclusive').length
  const skipped = valid.filter((r) => r.status === 'skipped').length
  const latencies = valid.filter(
    (r) => typeof r.latencyMs === 'number' && Number.isFinite(r.latencyMs)
  )
  const avgLatencyMs =
    latencies.length > 0
      ? Math.round(
          latencies.reduce((sum, r) => sum + (r.latencyMs ?? 0), 0) /
            latencies.length
        )
      : null
  const latest = valid[0] ?? null
  const latestLatencyMs =
    latest &&
    typeof latest.latencyMs === 'number' &&
    Number.isFinite(latest.latencyMs)
      ? Math.round(latest.latencyMs)
      : null
  return {
    total,
    success,
    failure,
    inconclusive,
    skipped,
    successRatePct: total === 0 ? 0 : Math.round((success / total) * 1000) / 10,
    avgLatencyMs,
    latestCreatedAt: latest?.createdAt?.trim() || null,
    latestLatencyMs,
    latestStatus: latest?.status ?? null,
  }
}

export interface ProbeHealthBarProps {
  /** Newest-first probe results for this row (API order). */
  results?: ProbeResult[]
  /** True while the batch history query has not settled (loading or error). */
  pending?: boolean
}

/**
 * The health bar cell. States: pending (muted dash — history is secondary
 * decoration and must never block the row), empty ("No probes" label — an
 * honest empty state, never fake gray placeholder bars), or the gapless
 * non-empty sample strip + rich tooltip.
 */
export function ProbeHealthBar({
  results,
  pending = false,
}: ProbeHealthBarProps) {
  const { t, i18n } = useTranslation()
  const locale = toBcp47(i18n.language || 'en')

  const validResults = useMemo(
    () => (results ?? []).filter(isValidProbeSample),
    [results]
  )

  const summary = useMemo(
    () =>
      validResults.length > 0 ? summarizeProbeResults(validResults) : null,
    [validResults]
  )

  if (pending) {
    return (
      <span aria-hidden='true' className='text-muted-foreground text-xs'>
        —
      </span>
    )
  }

  if (validResults.length === 0 || !summary) {
    return (
      <span className='text-muted-foreground text-xs'>
        {t('probeHealth.empty')}
      </span>
    )
  }

  // The API returns newest-first; render chronologically left → right so the
  // rightmost bar is always the latest real sample (no fake gap slots).
  const chronological = [...validResults].reverse()
  const ariaLabel = t('probeHealth.ariaSummary', {
    success: summary.success,
    total: summary.total,
  })
  const latestTime = summary.latestCreatedAt
    ? formatAbsoluteDateTime(summary.latestCreatedAt, locale)
    : null
  const latestLatencyLabel =
    summary.latestLatencyMs !== null
      ? formatLatency(summary.latestLatencyMs, { autoSeconds: true })
      : null

  return (
    <TooltipProvider delay={200}>
      <Tooltip>
        <TooltipTrigger
          render={
            <span
              tabIndex={0}
              aria-label={ariaLabel}
              data-probe-count={summary.total}
              className='focus-visible:ring-focus-ring inline-flex h-4 items-end gap-[2px] rounded-sm outline-none focus-visible:ring-2'
            />
          }
        >
          {chronological.map((result) => (
            <span
              key={result.id}
              data-probe-status={result.status}
              className={cn(
                'w-[3px] rounded-[1px] transition-opacity',
                STATUS_BAR_CLASS[result.status] ??
                  'h-1/2 bg-muted-foreground/40'
              )}
            />
          ))}
        </TooltipTrigger>
        <TooltipContent
          side='top'
          className='flex-col items-start gap-0.5 tabular-nums'
        >
          <span className='font-medium'>
            {t('probeHealth.tooltip.window', { total: summary.total })}
          </span>
          <span>
            {t('probeHealth.tooltip.successRate', {
              rate: summary.successRatePct,
              success: summary.success,
              total: summary.total,
            })}
          </span>
          <span>
            {summary.avgLatencyMs === null
              ? t('probeHealth.tooltip.noLatency')
              : t('probeHealth.tooltip.avgLatency', {
                  value: formatLatency(summary.avgLatencyMs, {
                    autoSeconds: true,
                  }),
                })}
          </span>
          {(latestTime || latestLatencyLabel) && (
            <span className='border-border/40 text-background/80 text-2xs mt-0.5 border-t pt-0.5'>
              {[latestTime, latestLatencyLabel].filter(Boolean).join(' · ')}
            </span>
          )}
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}
