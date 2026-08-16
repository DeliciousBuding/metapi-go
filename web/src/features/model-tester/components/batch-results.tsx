// metapi-go/features/model-tester — batch latency comparison results table.
//
// Renders one row per channel with channel/site identity, success/failure
// status, observed latency, and a concise error. Rows are pre-sorted by the
// parent (successes by ascending latency, then failures in input order); the
// summary line reports "N succeeded / M failed".

import { useTranslation } from 'react-i18next'

import type { ChannelRow } from '@/features/channels'
import { cn } from '@/lib/utils'

import type { BatchProbeResult } from '../types'

type BatchResultsProps = {
  results: BatchProbeResult[]
  channels: ChannelRow[]
  isRunning: boolean
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
      <div>
        <h2 className='text-base font-normal'>
          {t('modelTester.compare.title')}
        </h2>
        <p className='text-muted-foreground text-sm' aria-live='polite'>
          {t(summaryKey, { succeeded, failed, aborted })}
        </p>
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
              </tr>
            </thead>
            <tbody>
              {results.map((result) => {
                const channel = channelById.get(result.channelId)
                return (
                  <tr key={result.channelId} className='border-t'>
                    <td className='px-3 py-2'>
                      {channel?.name ?? result.channelId}
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
                      {result.latencyMs != null
                        ? `${result.latencyMs} ms`
                        : '—'}
                    </td>
                    <td className='text-muted-foreground max-w-56 px-3 py-2 break-words'>
                      {result.error || '—'}
                    </td>
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
