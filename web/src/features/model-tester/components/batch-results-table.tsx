import { Link } from '@tanstack/react-router'
import { Eye, RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { SoftToneBadge } from '@/components/common/soft-tone-badge'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import type { ChannelRow } from '@/features/channels'
import { formatLatency } from '@/lib/format'
import { cn } from '@/lib/utils'

import type { BatchProbeResult } from '../types'
import { statusKeyFor } from './batch-result-status'

export function BatchResultsTable({
  results,
  channelById,
  requestedModel,
  isRunning,
  rerunningChannelIds,
  onRerunRow,
  onShowDetail,
}: {
  results: BatchProbeResult[]
  channelById: ReadonlyMap<number, ChannelRow>
  requestedModel?: string
  isRunning: boolean
  rerunningChannelIds?: ReadonlySet<number>
  onRerunRow?: (channelId: number) => void
  onShowDetail: (channelId: number) => void
}) {
  const { t } = useTranslation()
  return (
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
            const channelDisplayName = channel?.name ?? String(result.channelId)
            const reportedModel = result.upstreamReportedModel?.trim() || null
            const normalizedRequested = requestedModel?.trim() || null
            const hasModelMismatch =
              Boolean(reportedModel) &&
              Boolean(normalizedRequested) &&
              reportedModel !== normalizedRequested
            return (
              <tr key={result.channelId} className='border-t'>
                <td className='px-3 py-2'>
                  <div className='flex flex-col gap-1'>
                    <Link
                      to='/channels'
                      search={{ channelId: result.channelId }}
                      className='text-primary hover:underline'
                    >
                      {channel?.name ?? result.channelId}
                    </Link>
                    {hasModelMismatch && reportedModel && (
                      <SoftToneBadge
                        tone='info'
                        title={t(
                          'modelTester.viewer.upstreamReportedModelHint'
                        )}
                      >
                        {t('modelTester.viewer.upstreamReportedModel', {
                          model: reportedModel,
                        })}
                      </SoftToneBadge>
                    )}
                  </div>
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
                  <div className='flex flex-col gap-0.5'>
                    <span>
                      {formatLatency(result.latencyMs, {
                        autoSeconds: true,
                        spaced: true,
                      })}
                    </span>
                    {result.firstByteLatencyMs != null && (
                      <span className='text-muted-foreground text-2xs'>
                        {t('modelTester.viewer.firstByteLatency', {
                          value: formatLatency(result.firstByteLatencyMs, {
                            autoSeconds: true,
                            spaced: true,
                          }),
                        })}
                      </span>
                    )}
                  </div>
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
                      onClick={() => onShowDetail(result.channelId)}
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
  )
}
