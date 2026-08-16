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
}

function statusKeyFor(result: BatchProbeResult): string {
  if (result.status === 'success') return 'modelTester.compare.statusSuccess'
  if (result.status === 'failure') return 'modelTester.compare.statusFailure'
  return 'modelTester.compare.statusAborted'
}

export function BatchResults({ results, channels }: BatchResultsProps) {
  const { t } = useTranslation()
  const channelById = new Map(channels.map((channel) => [channel.id, channel]))
  const succeeded = results.filter(
    (result) => result.status === 'success'
  ).length
  const failed = results.length - succeeded

  return (
    <div className='flex h-full flex-col gap-3 p-4'>
      <div>
        <h2 className='text-base font-normal'>
          {t('modelTester.compare.title')}
        </h2>
        <p className='text-muted-foreground text-sm'>
          {t('modelTester.compare.summary', { succeeded, failed })}
        </p>
      </div>

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
                    {result.latencyMs != null ? `${result.latencyMs} ms` : '—'}
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
    </div>
  )
}
