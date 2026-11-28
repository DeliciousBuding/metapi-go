// metapi-go/features/model-tester — batch latency comparison results table.
//
// Renders one row per channel with channel/site identity, success/failure
// status, observed latency, and a concise error. Rows are pre-sorted by the
// parent (successes by ascending latency, then failures in input order); the
// summary line reports "N succeeded / M failed". Every row carries a
// re-run action (when the parent wires onRerunRow) and a read-only detail
// drawer showing full error text, rawEvents, and response summary.

import { Ban } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import type { ChannelRow } from '@/features/channels'

import type { BatchProbeResult } from '../types'
import { BatchResultDetail } from './batch-result-detail'
import { summarizeBatch } from './batch-result-status'
import { BatchResultsTable } from './batch-results-table'

type BatchResultsProps = {
  results: BatchProbeResult[]
  channels: ChannelRow[]
  isRunning: boolean
  /** Requested model name for this batch comparison (detects upstream model mismatch). */
  requestedModel?: string
  /** Total channels queued for comparison (shows incremental progress). */
  totalCount?: number
  /** Channels whose single-row re-run probe is in flight (row spinner). */
  rerunningChannelIds?: ReadonlySet<number>
  /** Re-run one channel's probe with the comparison's original payload. */
  onRerunRow?: (channelId: number) => void
  /** One-click disable of every failed channel (opens a confirmation). */
  onDisableFailed?: () => void
  /** The bulk disable request is in flight (button pending/disabled). */
  isDisablingFailed?: boolean
}

export function BatchResults({
  results,
  channels,
  isRunning,
  requestedModel,
  totalCount,
  rerunningChannelIds,
  onRerunRow,
  onDisableFailed,
  isDisablingFailed = false,
}: BatchResultsProps) {
  const { t } = useTranslation()
  const [detailChannelId, setDetailChannelId] = useState<number | null>(null)
  const channelById = new Map(channels.map((channel) => [channel.id, channel]))
  const { summaryKey, counts } = summarizeBatch(results)

  const effectiveTotal =
    typeof totalCount === 'number' && totalCount > 0 ? totalCount : 0
  const progressPct =
    effectiveTotal > 0
      ? Math.min(100, Math.round((results.length / effectiveTotal) * 100))
      : 0

  const detailResult =
    detailChannelId !== null
      ? (results.find((item) => item.channelId === detailChannelId) ?? null)
      : null
  const detailChannel =
    detailResult !== null ? channelById.get(detailResult.channelId) : undefined
  return (
    <div className='flex h-full flex-col gap-3 p-4'>
      <div className='flex items-start justify-between gap-3'>
        <div className='min-w-0 flex-1 space-y-1.5'>
          <h2 className='text-base font-medium'>
            {t('modelTester.compare.title')}
          </h2>
          <p className='text-muted-foreground text-sm' aria-live='polite'>
            {t(summaryKey, counts)}
          </p>
          {isRunning && effectiveTotal > 0 && (
            <div className='max-w-sm space-y-1'>
              <div className='text-muted-foreground flex items-center justify-between text-xs tabular-nums'>
                <span>
                  {t('modelTester.compare.progress', {
                    completed: results.length,
                    total: effectiveTotal,
                  })}
                </span>
                <span>{progressPct}%</span>
              </div>
              <div
                role='progressbar'
                aria-valuemin={0}
                aria-valuemax={effectiveTotal}
                aria-valuenow={results.length}
                className='bg-muted h-1.5 w-full overflow-hidden rounded-full'
              >
                <div
                  className='bg-primary h-full transition-[width] duration-150'
                  style={{ width: `${progressPct}%` }}
                />
              </div>
            </div>
          )}
        </div>
        {counts.failed > 0 && onDisableFailed && (
          <Button
            type='button'
            variant='destructive'
            size='sm'
            onClick={onDisableFailed}
            disabled={isRunning || isDisablingFailed}
          >
            {isDisablingFailed ? <Spinner /> : <Ban className='size-3.5' />}
            {t('modelTester.compare.disableFailed')}
          </Button>
        )}
      </div>

      {results.length === 0 ? (
        <p
          className='text-muted-foreground flex min-h-32 items-center justify-center rounded-lg border border-dashed p-4 text-center text-sm'
          aria-live='polite'
        >
          {isRunning
            ? t('modelTester.viewer.awaitingContent')
            : t('modelTester.compare.emptyHint')}
        </p>
      ) : (
        <BatchResultsTable
          results={results}
          channelById={channelById}
          isRunning={isRunning}
          requestedModel={requestedModel}
          rerunningChannelIds={rerunningChannelIds}
          onRerunRow={onRerunRow}
          onShowDetail={setDetailChannelId}
        />
      )}
      <BatchResultDetail
        detailResult={detailResult}
        detailChannel={detailChannel}
        onClose={() => setDetailChannelId(null)}
      />
    </div>
  )
}
