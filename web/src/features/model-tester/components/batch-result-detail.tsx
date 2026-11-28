import { useTranslation } from 'react-i18next'

import { DetailField } from '@/components/common/detail-field'
import { Badge } from '@/components/ui/badge'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import type { ChannelRow } from '@/features/channels'
import { formatInt, formatLatency } from '@/lib/format'

import type { BatchProbeResult } from '../types'
import { statusKeyFor } from './batch-result-status'

export function BatchResultDetail({
  detailResult,
  detailChannel,
  onClose,
}: {
  detailResult: BatchProbeResult | null
  detailChannel?: ChannelRow
  onClose: () => void
}) {
  const { t } = useTranslation()
  let detailBadgeVariant: 'success' | 'destructive' | 'secondary' = 'secondary'
  if (detailResult?.status === 'success') detailBadgeVariant = 'success'
  else if (detailResult?.status === 'failure')
    detailBadgeVariant = 'destructive'

  return (
    <Sheet
      open={detailResult !== null}
      onOpenChange={(open) => {
        if (!open) onClose()
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
              {detailResult.firstByteLatencyMs !== undefined && (
                <DetailField label={t('modelTester.compare.firstByte')}>
                  {detailResult.firstByteLatencyMs != null
                    ? formatLatency(detailResult.firstByteLatencyMs, {
                        autoSeconds: true,
                        spaced: true,
                      })
                    : '—'}
                </DetailField>
              )}
              {detailResult.upstreamReportedModel && (
                <DetailField
                  label={t('modelTester.compare.upstreamReportedModel')}
                  full
                >
                  {detailResult.upstreamReportedModel}
                </DetailField>
              )}
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
  )
}
