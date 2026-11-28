import { Copy } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { DetailField } from '@/components/common/detail-field'
import { ModelPill } from '@/components/common/model-pill'
import { Button } from '@/components/ui/button'
import type { GatewayRequestDetail } from '@/lib/api'
import { copyText } from '@/lib/clipboard'
import { formatDateTime, formatLatency } from '@/lib/format'
import { toast } from '@/lib/toast'

import { RequestOutcome } from './request-outcome'

export function GatewayRequestSummary({
  detail,
  locale,
}: {
  detail: GatewayRequestDetail
  locale: string
}) {
  const { t } = useTranslation()
  return (
    <>
      <section className='bg-muted/30 space-y-3 rounded-xl border p-4'>
        <div className='flex flex-wrap items-start justify-between gap-3'>
          {detail.modelRequested ? (
            <ModelPill
              model={detail.modelRequested}
              className='[&>span:last-child]:break-all [&>span:last-child]:whitespace-normal'
            />
          ) : (
            <span className='text-muted-foreground text-sm'>
              {t('requestLogs.unknownModel')}
            </span>
          )}
          <RequestOutcome request={detail} />
        </div>
        <div className='flex min-w-0 items-center gap-2'>
          <span className='text-muted-foreground min-w-0 font-mono text-xs break-all'>
            {detail.id}
          </span>
          <Button
            size='icon-sm'
            variant='ghost'
            aria-label={t('requestLogs.copyId')}
            onClick={async () => {
              if (await copyText(detail.id)) toast.success(t('common.copied'))
              else toast.error(t('requestLogs.copyFailed'))
            }}
          >
            <Copy className='size-3.5' />
          </Button>
        </div>
      </section>
      <dl className='grid grid-cols-2 gap-x-4 gap-y-3'>
        <DetailField label={t('requestLogs.interface')}>
          {detail.downstreamPath ?? t('requestLogs.unknown')}
        </DetailField>
        <DetailField label={t('requestLogs.transport')}>
          {detail.transport === 'websocket' ? t('requestLogs.wsTurn') : 'HTTP'}
        </DetailField>
        <DetailField label={t('requestLogs.startedAt')}>
          {formatDateTime(detail.startedAt, locale)}
        </DetailField>
        <DetailField label={t('requestLogs.endedAt')}>
          {formatDateTime(detail.endedAt, locale)}
        </DetailField>
        <DetailField label={t('requestLogs.duration')}>
          {formatLatency(detail.latencyMs)}
        </DetailField>
        <DetailField label={t('requestLogs.outputStarted')}>
          {t(
            detail.outputStarted
              ? 'proxyLogs.detail.yes'
              : 'proxyLogs.detail.no'
          )}
        </DetailField>
        <DetailField label={t('proxyLogs.timing.firstByte')}>
          {detail.firstByteLatencyMs === null
            ? t('requestLogs.unknown')
            : formatLatency(detail.firstByteLatencyMs)}
        </DetailField>
        <DetailField label={t('proxyLogs.timing.firstOutput')}>
          {detail.firstOutputLatencyMs === null
            ? t('requestLogs.unknown')
            : formatLatency(detail.firstOutputLatencyMs)}
        </DetailField>
        <DetailField label={t('requestLogs.key')}>
          {detail.downstreamApiKeyId === null
            ? t('requestLogs.unknown')
            : `#${detail.downstreamApiKeyId}`}
        </DetailField>
        <DetailField label={t('requestLogs.externalId')}>
          {detail.externalRequestId ?? t('requestLogs.notProvided')}
        </DetailField>
        {detail.reasonCode && (
          <DetailField label={t('requestLogs.reason')} full>
            {detail.reasonCode}
          </DetailField>
        )}
      </dl>
    </>
  )
}
