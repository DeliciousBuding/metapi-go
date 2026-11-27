// metapi-go/features/model-tester — batch latency comparison results table.
//
// Renders one row per channel with channel/site identity, success/failure
// status, observed latency, and a concise error. Rows are pre-sorted by the
// parent (successes by ascending latency, then failures in input order); the
// summary line reports "N succeeded / M failed". Every row carries a
// re-run action (when the parent wires onRerunRow) and a read-only detail
// drawer showing full error text, rawEvents, and response summary.

import { Link } from '@tanstack/react-router'
import { Ban, Eye, RefreshCw } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { DetailField } from '@/components/common/detail-field'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Spinner } from '@/components/ui/spinner'
import type { ChannelRow } from '@/features/channels'
import { formatInt, formatLatency } from '@/lib/format'
import { cn } from '@/lib/utils'

import type { BatchProbeResult } from '../types'

type BatchResultsProps = {
  results: BatchProbeResult[]
  channels: ChannelRow[]
  isRunning: boolean
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

function statusKeyFor(result: BatchProbeResult): string {
  if (result.status === 'success') return 'modelTester.compare.statusSuccess'
  if (result.status === 'failure') return 'modelTester.compare.statusFailure'
  return 'modelTester.compare.statusAborted'
}

export function BatchResults({
  results,
  channels,
  isRunning,
  totalCount,
  rerunningChannelIds,
  onRerunRow,
  onDisableFailed,
  isDisablingFailed = false,
}: BatchResultsProps) {
  const { t } = useTranslation()
  const [detailChannelId, setDetailChannelId] = useState<number | null>(null)
  const channelById = new Map(channels.map((channel) => [channel.id, channel]))
  const succeeded = results.filter(
    (result) => result.status === 'success'
  ).length
  const failed = results.filter((result) => result.status === 'failure').length
  const aborted = results.filter((result) => result.status === 'aborted').length
  const summaryKey =
    aborted > 0
      ? 'modelTester.compare.summaryWithAborted'
      : 'modelTester.compare.summary'

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
  let detailBadgeVariant: 'success' | 'destructive' | 'secondary' = 'secondary'
  if (detailResult?.status === 'success') detailBadgeVariant = 'success'
  else if (detailResult?.status === 'failure')
    detailBadgeVariant = 'destructive'

  return (
    <div className='flex h-full flex-col gap-3 p-4'>
      <div className='flex items-start justify-between gap-3'>
        <div className='min-w-0 flex-1 space-y-1.5'>
          <h2 className='text-base font-medium'>
            {t('modelTester.compare.title')}
          </h2>
          <p className='text-muted-foreground text-sm' aria-live='polite'>
            {t(summaryKey, { succeeded, failed, aborted })}
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
        {failed > 0 && onDisableFailed && (
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
        <div className='min-h-0 flex-1 overflow-y-auto rounded-lg border'>
          <table className='w-full text-sm'>
            <thead className='bg-muted/50 sticky top-0 text-left'>
              <tr>
                <th className='px-3 py-2 font-medium'>
                  {t('modelTester.compare.channel')}
                </th>
                <th className='px-3 py-2 font-medium'>
                  {t('modelTester.compare.site')}
                </th>
                <th className='px-3 py-2 font-medium'>
                  {t('modelTester.compare.status')}
                </th>
                <th className='px-3 py-2 font-medium'>
                  {t('modelTester.compare.latency')}
                </th>
                <th className='px-3 py-2 font-medium'>
                  {t('modelTester.compare.error')}
                </th>
                <th className='px-3 py-2 text-right'>
                  <span className='sr-only'>{t('common.actions')}</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {results.map((result) => {
                const channel = channelById.get(result.channelId)
                const isRowPending =
                  rerunningChannelIds?.has(result.channelId) ?? false
                const channelDisplayName =
                  channel?.name ?? String(result.channelId)
                return (
                  <tr key={result.channelId} className='border-t'>
                    <td className='px-3 py-2'>
                      <Link
                        to='/channels'
                        search={{ channelId: result.channelId }}
                        className='text-primary hover:underline'
                      >
                        {channel?.name ?? result.channelId}
                      </Link>
                    </td>
                    <td className='text-muted-foreground px-3 py-2'>
                      {channel?.site.name ?? '—'}
                    </td>
                    <td className='px-3 py-2'>
                      <span
                        className={cn(
                          'text-xs font-medium',
                          result.status === 'success' && 'text-success',
                          result.status === 'failure' && 'text-destructive',
                          result.status === 'aborted' && 'text-muted-foreground'
                        )}
                      >
                        {t(statusKeyFor(result))}
                      </span>
                    </td>
                    <td className='px-3 py-2 tabular-nums'>
                      {formatLatency(result.latencyMs, {
                        autoSeconds: true,
                        spaced: true,
                      })}
                    </td>
                    <td className='text-muted-foreground max-w-56 px-3 py-2 break-words'>
                      {result.error || '—'}
                    </td>
                    <td className='px-3 py-2 text-right'>
                      <div className='flex items-center justify-end gap-1'>
                        <Button
                          type='button'
                          variant='ghost'
                          size='xs'
                          aria-label={t('modelTester.compare.detailsAria', {
                            channel: channelDisplayName,
                          })}
                          onClick={() => setDetailChannelId(result.channelId)}
                        >
                          <Eye className='size-3.5' aria-hidden='true' />
                          {t('modelTester.compare.details')}
                        </Button>
                        {onRerunRow && (
                          <Button
                            variant='ghost'
                            size='icon-sm'
                            aria-label={t('modelTester.compare.rerunRow', {
                              channel: channelDisplayName,
                            })}
                            data-hit-area
                            disabled={isRowPending || isRunning}
                            onClick={() => onRerunRow(result.channelId)}
                          >
                            {isRowPending ? (
                              <Spinner />
                            ) : (
                              <RefreshCw className='size-4' />
                            )}
                          </Button>
                        )}
                      </div>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}

      <Sheet
        open={detailResult !== null}
        onOpenChange={(open) => {
          if (!open) setDetailChannelId(null)
        }}
      >
        <SheetContent className='flex w-full flex-col gap-0 sm:max-w-lg'>
          <SheetHeader>
            <SheetTitle>
              {t('modelTester.compare.detailTitle', {
                channel:
                  detailChannel?.name ??
                  (detailResult ? `#${detailResult.channelId}` : ''),
              })}
            </SheetTitle>
            <SheetDescription>
              {t('modelTester.compare.detailDescription')}
            </SheetDescription>
          </SheetHeader>
          {detailResult && (
            <div className='flex-1 space-y-4 overflow-y-auto p-4 text-sm'>
              <dl className='grid grid-cols-2 gap-x-4 gap-y-3'>
                <DetailField label={t('modelTester.compare.site')}>
                  {detailChannel?.site.name ?? '—'}
                </DetailField>
                <DetailField label={t('modelTester.compare.status')}>
                  <Badge variant={detailBadgeVariant}>
                    {t(statusKeyFor(detailResult))}
                  </Badge>
                </DetailField>
                <DetailField label={t('modelTester.compare.httpStatus')}>
                  {detailResult.statusCode ?? '—'}
                </DetailField>
                <DetailField label={t('modelTester.compare.latency')}>
                  {formatLatency(detailResult.latencyMs, {
                    autoSeconds: true,
                    spaced: true,
                  })}
                </DetailField>
                {detailResult.totalTokens !== undefined && (
                  <DetailField label={t('modelTester.viewer.usage')} full>
                    {t('modelTester.viewer.usage', {
                      total: formatInt(detailResult.totalTokens),
                      prompt: formatInt(detailResult.promptTokens ?? 0),
                      completion: formatInt(detailResult.completionTokens ?? 0),
                    })}
                  </DetailField>
                )}
              </dl>

              {detailResult.error && (
                <section className='space-y-1.5'>
                  <h3 className='text-destructive text-xs font-medium'>
                    {t('modelTester.compare.error')}
                  </h3>
                  <pre className='border-destructive/30 bg-destructive/5 text-destructive-soft-fg max-h-48 overflow-auto rounded-lg border p-3 font-mono text-xs break-all whitespace-pre-wrap'>
                    {detailResult.error}
                  </pre>
                </section>
              )}

              <section className='space-y-1.5'>
                <h3 className='text-muted-foreground text-xs font-medium'>
                  {t('modelTester.compare.responseSummary')}
                </h3>
                {detailResult.content ? (
                  <pre className='bg-muted/40 max-h-48 overflow-auto rounded-lg border p-3 font-sans text-xs break-words whitespace-pre-wrap'>
                    {detailResult.content}
                  </pre>
                ) : (
                  <p className='text-muted-foreground text-xs'>
                    {t('modelTester.viewer.noContent')}
                  </p>
                )}
                {detailResult.reasoningContent && (
                  <details className='text-xs'>
                    <summary className='text-muted-foreground cursor-pointer py-1'>
                      {t('modelTester.viewer.tabReasoning')}
                    </summary>
                    <pre className='bg-muted/30 max-h-40 overflow-auto rounded-lg border p-2.5 text-xs break-words whitespace-pre-wrap'>
                      {detailResult.reasoningContent}
                    </pre>
                  </details>
                )}
              </section>

              <section className='space-y-1.5'>
                <h3 className='text-muted-foreground text-xs font-medium'>
                  {t('modelTester.compare.rawEvents')}
                </h3>
                {detailResult.bodyTruncated && (
                  <p className='text-warning-soft-fg text-xs'>
                    {t('modelTester.viewer.truncated')}
                  </p>
                )}
                {detailResult.rawEvents && detailResult.rawEvents.length > 0 ? (
                  <pre className='bg-muted max-h-60 overflow-auto rounded-lg border p-3 font-mono text-xs break-all whitespace-pre-wrap'>
                    {detailResult.rawEvents.join('\n')}
                  </pre>
                ) : (
                  <p className='text-muted-foreground text-xs'>
                    {t('modelTester.viewer.noRaw')}
                  </p>
                )}
              </section>
            </div>
          )}
        </SheetContent>
      </Sheet>
    </div>
  )
}
