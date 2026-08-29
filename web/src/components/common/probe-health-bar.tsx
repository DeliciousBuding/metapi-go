/* eslint-disable react/only-export-components -- summary helper + shared types are co-located with the component that owns them */
// metapi-go/components/common — row-level probe history health bar (P0-2).
// Renders the recent background model-probe results of one channel/account
// as a compact strip of vertical bars (one per probe, chronological
// left-to-right), with a tooltip carrying the success rate and average
// latency so operators can spot a bad channel in seconds. Purely
// presentational: batch data fetching lives in use-probe-history.ts.

import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { formatLatency } from '@/lib/format'
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

// Design-token colors: readable in both light and dark OKLCH themes.
// `skipped` is deliberately faint — it carries no connectivity signal.
const STATUS_BAR_CLASS: Record<ProbeStatus, string> = {
  success: 'bg-success',
  failure: 'bg-destructive',
  inconclusive: 'bg-warning',
  skipped: 'bg-muted-foreground/40',
}

export interface ProbeSummary {
  total: number
  success: number
  /** Percentage with one decimal (e.g. 66.7). */
  successRatePct: number
  /** Mean of the results that recorded a latency; null when none did. */
  avgLatencyMs: number | null
}

export function summarizeProbeResults(results: ProbeResult[]): ProbeSummary {
  const total = results.length
  const success = results.filter((r) => r.status === 'success').length
  const latencies = results.filter(
    (r) => typeof r.latencyMs === 'number' && Number.isFinite(r.latencyMs)
  )
  const avgLatencyMs =
    latencies.length > 0
      ? Math.round(
          latencies.reduce((sum, r) => sum + (r.latencyMs ?? 0), 0) /
            latencies.length
        )
      : null
  return {
    total,
    success,
    successRatePct: total === 0 ? 0 : Math.round((success / total) * 1000) / 10,
    avgLatencyMs,
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
 * honest empty state, never a fake green bar), or the bar strip + tooltip.
 */
export function ProbeHealthBar({
  results,
  pending = false,
}: ProbeHealthBarProps) {
  const { t } = useTranslation()
  const summary = useMemo(
    () =>
      results && results.length > 0 ? summarizeProbeResults(results) : null,
    [results]
  )

  if (pending) {
    return (
      <span aria-hidden='true' className='text-muted-foreground text-xs'>
        —
      </span>
    )
  }

  if (!results || results.length === 0 || !summary) {
    return (
      <span className='text-muted-foreground text-xs'>
        {t('probeHealth.empty')}
      </span>
    )
  }

  // The API returns newest-first; render chronologically left → right so the
  // rightmost bar is always the latest probe.
  const chronological = [...results].reverse()
  const ariaLabel = t('probeHealth.ariaSummary', {
    success: summary.success,
    total: summary.total,
  })

  return (
    <TooltipProvider delay={200}>
      <Tooltip>
        <TooltipTrigger
          render={
            <span
              tabIndex={0}
              aria-label={ariaLabel}
              className='focus-visible:ring-focus-ring flex h-4 items-stretch gap-[2px] rounded-sm outline-none focus-visible:ring-2'
            />
          }
        >
          {chronological.map((result) => (
            <span
              key={result.id}
              className={cn(
                'w-[3px] rounded-[1px]',
                STATUS_BAR_CLASS[result.status] ?? 'bg-muted-foreground/40'
              )}
            />
          ))}
        </TooltipTrigger>
        <TooltipContent side='top' className='flex-col items-start gap-0.5'>
          <span>
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
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}
