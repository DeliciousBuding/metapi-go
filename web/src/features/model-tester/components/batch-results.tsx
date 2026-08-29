// metapi-go/features/model-tester — batch latency comparison results table.
//
// Renders one row per channel with channel/site identity, success/failure
// status, observed latency, and a concise error. Rows are pre-sorted by the
// parent (successes by ascending latency, then failures in input order); the
// summary line reports "N succeeded / M failed". Every row carries a
// re-run action (when the parent wires onRerunRow) that re-probes that
// channel with the comparison's original payload — failed rows included —
// with a per-row pending spinner while the probe is in flight.

import { Link } from '@tanstack/react-router'
import { Ban, RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import type { ChannelRow } from '@/features/channels'
import { formatLatency } from '@/lib/format'
import { cn } from '@/lib/utils'

import type { BatchProbeResult } from '../types'

type BatchResultsProps = {
  results: BatchProbeResult[]
  channels: ChannelRow[]
  isRunning: boolean
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
  rerunningChannelIds,
  onRerunRow,
  onDisableFailed,
  isDisablingFailed = false,
}: BatchResultsProps) {
  const { t } = useTranslation()
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

  return (
    <div className='flex h-full flex-col gap-3 p-4'>
      <div className='flex items-start justify-between gap-3'>
        <div>
          <h2 className='text-base font-medium'>
            {t('modelTester.compare.title')}
          </h2>
          <p className='text-muted-foreground text-sm' aria-live='polite'>
            {t(summaryKey, { succeeded, failed, aborted })}
          </p>
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
                {onRerunRow && (
                  <th className='px-3 py-2'>
                    <span className='sr-only'>{t('common.actions')}</span>
                  </th>
                )}
              </tr>
            </thead>
            <tbody>
              {results.map((result) => {
                const channel = channelById.get(result.channelId)
                const isRowPending =
                  rerunningChannelIds?.has(result.channelId) ?? false
                return (
                  <tr key={result.channelId} className='border-t'>
                    <td className='px-3 py-2'>
                      {/* Every settled result carries its channelId, so the
                          channel identity deep-links into the channels page
                          detail sheet (one-shot `channelId` param) — failed
                          rows are no longer a dead end. */}
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
                    {onRerunRow && (
                      <td className='px-3 py-2'>
                        <Button
                          variant='ghost'
                          size='icon-sm'
                          aria-label={t('modelTester.compare.rerunRow', {
                            channel: channel?.name ?? String(result.channelId),
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
                      </td>
                    )}
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
