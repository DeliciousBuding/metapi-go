import { Copy } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { DetailField } from '@/components/common/detail-field'
import { ExpandableText } from '@/components/common/expandable-text'
import { HttpStatusBadge } from '@/components/common/http-status-badge'
import { ModelPill } from '@/components/common/model-pill'
import { QueryErrorBanner } from '@/components/common/query-error-banner'
import { Button } from '@/components/ui/button'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Skeleton } from '@/components/ui/skeleton'
import { routeCheckEndpointFromPath } from '@/features/token-routes'
import { toBcp47 } from '@/i18n/languages'
import { copyText } from '@/lib/clipboard'
import { formatCurrency, formatDateTime, formatLatency } from '@/lib/format'
import { parseProxyLogPathMeta } from '@/lib/helpers/proxyLogPathMeta'
import { toast } from '@/lib/toast'

import { useProxyLog } from '../api'
import { useGatewayRequest } from '../lib/gateway-requests'
import { BillingSection, CurrentRouteDiagnosis } from './proxy-log-detail-sheet'
import { RequestOutcome } from './request-outcome'
import { TimingCell } from './timing-cell'
import { UsageCell } from './usage-cell'

export function GatewayRequestDetailSheet({
  id,
  onClose,
}: {
  id: string | null
  onClose: () => void
}) {
  const { t, i18n } = useTranslation()
  const locale = toBcp47(i18n.language)
  const query = useGatewayRequest(id)
  const detail = query.data
  const [attemptId, setAttemptId] = useState<number | null>(null)
  const selectedAttempt = detail?.attempts.find(
    (attempt) => attempt.id === attemptId
  )
  const attemptQuery = useProxyLog(selectedAttempt?.id ?? null)
  return (
    <Sheet
      open={id !== null}
      onOpenChange={(open) => {
        if (!open) onClose()
      }}
    >
      <SheetContent className='flex w-full flex-col gap-0 sm:max-w-xl'>
        <SheetHeader>
          <SheetTitle>{t('requestLogs.detailTitle')}</SheetTitle>
          <SheetDescription>{t('requestLogs.observedHint')}</SheetDescription>
        </SheetHeader>
        <div className='flex-1 space-y-5 overflow-y-auto p-4'>
          {query.isLoading && <Skeleton className='h-44 w-full' />}
          <QueryErrorBanner
            error={query.error}
            messageKey='requestLogs.detailError'
            onRetry={() => void query.refetch()}
            isRetrying={query.isFetching}
          />
          {detail && (
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
                      if (await copyText(detail.id))
                        toast.success(t('common.copied'))
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
                  {detail.transport === 'websocket'
                    ? t('requestLogs.wsTurn')
                    : 'HTTP'}
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
              <CurrentRouteDiagnosis
                model={detail.modelRequested}
                endpoint={routeCheckEndpointFromPath(detail.downstreamPath)}
                keyId={detail.downstreamApiKeyId}
              />
              <section className='space-y-3 border-t pt-4'>
                <h3 className='text-sm font-semibold'>
                  {t('requestLogs.attemptsTitle', {
                    count: detail.attemptCount,
                  })}
                </h3>
                <p className='text-muted-foreground text-xs leading-5'>
                  {t('requestLogs.attemptsHint')}
                </p>
                {detail.attemptCount === 0 && (
                  <p className='bg-muted/30 rounded-lg border p-4 text-sm'>
                    {t('requestLogs.noAttempts')}
                  </p>
                )}
                {(detail.partial ||
                  detail.attemptCount !== detail.attempts.length) && (
                  <p role='status' className='text-warning text-sm'>
                    {t('requestLogs.partialAttempts', {
                      recorded: detail.attempts.length,
                      expected: detail.attemptCount,
                    })}
                  </p>
                )}
                <ol className='space-y-2'>
                  {detail.attempts.map((attempt) => (
                    <li key={attempt.id} className='rounded-lg border'>
                      <details
                        open={selectedAttempt?.id === attempt.id}
                        onToggle={(event) => {
                          if (event.currentTarget.open) setAttemptId(attempt.id)
                          else
                            setAttemptId((current) =>
                              current === attempt.id ? null : current
                            )
                        }}
                      >
                        <summary className='focus-visible:ring-ring cursor-pointer rounded-lg p-3 outline-none focus-visible:ring-2'>
                          <span className='ms-1 inline-flex max-w-full flex-wrap items-center gap-2 text-sm'>
                            <span className='font-medium'>
                              {t('requestLogs.attempt', {
                                ordinal:
                                  attempt.retryCount === null
                                    ? t('requestLogs.unknown')
                                    : attempt.retryCount + 1,
                              })}
                            </span>
                            <HttpStatusBadge
                              status={attempt.status}
                              httpStatus={attempt.httpStatus}
                            />
                            <span className='text-muted-foreground'>
                              {attempt.latencyMs === null
                                ? t('requestLogs.unknown')
                                : formatLatency(attempt.latencyMs)}
                            </span>
                          </span>
                        </summary>
                        <div className='space-y-3 border-t p-3'>
                          {attempt.modelActual && (
                            <ModelPill model={attempt.modelActual} />
                          )}
                          <p className='text-muted-foreground text-xs break-all'>
                            {[attempt.siteName, attempt.username]
                              .filter(Boolean)
                              .join(' · ') ||
                              t('requestLogs.unknownCandidate')}{' '}
                            · {formatDateTime(attempt.createdAt, locale)}
                          </p>
                          <div className='grid grid-cols-2 gap-4'>
                            <TimingCell {...attempt} />
                            <UsageCell log={attempt} />
                          </div>
                          <p className='text-muted-foreground text-xs'>
                            {t('requestLogs.estimatedCost')}:{' '}
                            {attempt.estimatedCost == null
                              ? t('requestLogs.unknown')
                              : formatCurrency(attempt.estimatedCost, {
                                  fractionDigits: 6,
                                })}
                          </p>
                          {attempt.errorMessage && (
                            <ExpandableText
                              text={
                                parseProxyLogPathMeta(attempt.errorMessage)
                                  .errorMessage
                              }
                            />
                          )}
                          {selectedAttempt?.id === attempt.id && (
                            <>
                              <QueryErrorBanner
                                error={attemptQuery.error}
                                messageKey='requestLogs.attemptError'
                                onRetry={() => void attemptQuery.refetch()}
                                isRetrying={attemptQuery.isFetching}
                              />
                              {attemptQuery.isLoading && (
                                <Skeleton className='h-12 w-full' />
                              )}
                              {attemptQuery.data?.billingDetails && (
                                <BillingSection
                                  billing={attemptQuery.data.billingDetails}
                                />
                              )}
                            </>
                          )}
                        </div>
                      </details>
                    </li>
                  ))}
                </ol>
              </section>
            </>
          )}
        </div>
      </SheetContent>
    </Sheet>
  )
}
