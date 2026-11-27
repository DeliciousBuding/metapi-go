import { Link } from '@tanstack/react-router'
import { ChevronDown, ChevronUp, Copy, FlaskConical } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { DetailField } from '@/components/common/detail-field'
import { ExpandableText } from '@/components/common/expandable-text'
import { HttpStatusBadge } from '@/components/common/http-status-badge'
import { ModelPill } from '@/components/common/model-pill'
import { QueryErrorBanner } from '@/components/common/query-error-banner'
import { Button, buttonVariants } from '@/components/ui/button'
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
import { buildModelTesterSearch } from '@/lib/helpers/model-tester-link'
import { parseProxyLogPathMeta } from '@/lib/helpers/proxyLogPathMeta'
import { toast } from '@/lib/toast'

import { useProxyLog } from '../api'
import { useDetailSheetKeyboardNav } from '../lib/detail-navigation'
import { useGatewayRequest } from '../lib/gateway-requests'
import { AttemptWaterfall } from './attempt-waterfall'
import { BillingSection, CurrentRouteDiagnosis } from './proxy-log-detail-sheet'
import { RelayBridgeTraceCard } from './relay-bridge-trace'
import { RequestOutcome } from './request-outcome'
import { StreamTimingCard } from './stream-timing-card'
import { TimingCell } from './timing-cell'
import { UsageCell } from './usage-cell'

export function GatewayRequestDetailSheet({
  id,
  onClose,
  hasPrev,
  hasNext,
  onNavigatePrev,
  onNavigateNext,
}: {
  id: string | null
  onClose: () => void
  hasPrev?: boolean
  hasNext?: boolean
  onNavigatePrev?: () => void
  onNavigateNext?: () => void
}) {
  const { t, i18n } = useTranslation()
  const locale = toBcp47(i18n.language)
  const query = useGatewayRequest(id)
  const detail = query.data
  const [attemptId, setAttemptId] = useState<number | null>(null)
  useEffect(() => {
    setAttemptId(null)
  }, [id])
  const selectedAttempt = detail?.attempts.find(
    (attempt) => attempt.id === attemptId
  )
  const attemptQuery = useProxyLog(selectedAttempt?.id ?? null)

  useDetailSheetKeyboardNav({
    open: id !== null,
    hasPrev,
    hasNext,
    onPrev: onNavigatePrev,
    onNext: onNavigateNext,
  })

  const endpoint = routeCheckEndpointFromPath(detail?.downstreamPath)
  const reproSearch = useMemo(
    () =>
      buildModelTesterSearch({
        model: detail?.modelRequested,
        endpoint,
      }),
    [detail?.modelRequested, endpoint]
  )

  const scaleMaxMs = useMemo(() => {
    if (!detail) return 0
    const values = [
      detail.latencyMs,
      ...detail.attempts.map((attempt) => attempt.latencyMs ?? 0),
    ].filter((v) => typeof v === 'number' && Number.isFinite(v) && v > 0)
    return values.length > 0 ? Math.max(...values) : 0
  }, [detail])

  return (
    <Sheet
      open={id !== null}
      onOpenChange={(open) => {
        if (!open) onClose()
      }}
    >
      <SheetContent className='flex w-full flex-col gap-0 sm:max-w-xl'>
        <SheetHeader>
          <div className='flex items-center justify-between gap-2 pr-8'>
            <SheetTitle>{t('requestLogs.detailTitle')}</SheetTitle>
            {(onNavigatePrev || onNavigateNext) && (
              <div className='flex items-center gap-1'>
                <Button
                  type='button'
                  size='icon-xs'
                  variant='outline'
                  aria-label={t('requestLogs.prevRecord')}
                  disabled={!hasPrev}
                  onClick={onNavigatePrev}
                >
                  <ChevronUp className='size-3.5' />
                </Button>
                <Button
                  type='button'
                  size='icon-xs'
                  variant='outline'
                  aria-label={t('requestLogs.nextRecord')}
                  disabled={!hasNext}
                  onClick={onNavigateNext}
                >
                  <ChevronDown className='size-3.5' />
                </Button>
              </div>
            )}
          </div>
          <SheetDescription>{t('requestLogs.observedHint')}</SheetDescription>
          {reproSearch && (
            <div className='pt-1'>
              <Link
                to='/model-tester'
                search={reproSearch}
                className={buttonVariants({ size: 'xs', variant: 'outline' })}
              >
                <FlaskConical className='size-3.5' aria-hidden='true' />
                {t('requestLogs.reproduceInTester')}
              </Link>
            </div>
          )}
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
                endpoint={endpoint}
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
                  {detail.attempts.map((attempt) => {
                    const isSelectedAttempt = selectedAttempt?.id === attempt.id
                    const effectiveStreamTiming =
                      attempt.streamTiming ??
                      (isSelectedAttempt
                        ? (attemptQuery.data?.streamTiming ?? null)
                        : null)
                    const effectiveConversion =
                      attempt.conversion ??
                      (isSelectedAttempt
                        ? (attemptQuery.data?.conversion ?? null)
                        : null)

                    return (
                      <li key={attempt.id} className='rounded-lg border'>
                        <details
                          open={isSelectedAttempt}
                          onToggle={(event) => {
                            if (event.currentTarget.open)
                              setAttemptId(attempt.id)
                            else
                              setAttemptId((current) =>
                                current === attempt.id ? null : current
                              )
                          }}
                        >
                          <summary className='focus-visible:ring-ring cursor-pointer rounded-lg p-3 outline-none focus-visible:ring-2'>
                            <div className='space-y-2'>
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
                              <AttemptWaterfall
                                latencyMs={attempt.latencyMs}
                                firstByteLatencyMs={attempt.firstByteLatencyMs}
                                firstOutputLatencyMs={
                                  attempt.firstOutputLatencyMs
                                }
                                isStream={attempt.isStream}
                                streamTiming={effectiveStreamTiming}
                                scaleMaxMs={scaleMaxMs}
                                compact={
                                  isSelectedAttempt &&
                                  effectiveStreamTiming !== null
                                }
                              />
                            </div>
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
                            {effectiveStreamTiming ? (
                              <div className='flex flex-wrap items-center justify-between gap-3'>
                                <UsageCell log={attempt} />
                                <p className='text-muted-foreground text-xs tabular-nums'>
                                  {t('requestLogs.estimatedCost')}:{' '}
                                  {attempt.estimatedCost == null
                                    ? t('requestLogs.unknown')
                                    : formatCurrency(attempt.estimatedCost, {
                                        fractionDigits: 6,
                                      })}
                                </p>
                              </div>
                            ) : (
                              <>
                                <div className='grid grid-cols-2 gap-4'>
                                  <TimingCell {...attempt} />
                                  <UsageCell log={attempt} />
                                </div>
                                <p className='text-muted-foreground text-xs tabular-nums'>
                                  {t('requestLogs.estimatedCost')}:{' '}
                                  {attempt.estimatedCost == null
                                    ? t('requestLogs.unknown')
                                    : formatCurrency(attempt.estimatedCost, {
                                        fractionDigits: 6,
                                      })}
                                </p>
                              </>
                            )}
                            {attempt.errorMessage && (
                              <ExpandableText
                                text={
                                  parseProxyLogPathMeta(attempt.errorMessage)
                                    .errorMessage
                                }
                              />
                            )}
                            {effectiveStreamTiming && (
                              <StreamTimingCard
                                streamTiming={effectiveStreamTiming}
                                latencyMs={attempt.latencyMs}
                              />
                            )}
                            {effectiveConversion && (
                              <RelayBridgeTraceCard
                                conversion={effectiveConversion}
                              />
                            )}
                            {isSelectedAttempt && (
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
                    )
                  })}
                </ol>
              </section>
            </>
          )}
        </div>
      </SheetContent>
    </Sheet>
  )
}
