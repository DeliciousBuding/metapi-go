import { useMemo, type Dispatch, type SetStateAction } from 'react'
import { useTranslation } from 'react-i18next'

import { ExpandableText } from '@/components/common/expandable-text'
import { HttpStatusBadge } from '@/components/common/http-status-badge'
import { ModelPill } from '@/components/common/model-pill'
import { QueryErrorBanner } from '@/components/common/query-error-banner'
import { Skeleton } from '@/components/ui/skeleton'
import type { GatewayRequestDetail } from '@/lib/api'
import { formatCurrency, formatDateTime, formatLatency } from '@/lib/format'
import { parseProxyLogPathMeta } from '@/lib/helpers/proxyLogPathMeta'

import type { useProxyLog } from '../api'
import { AttemptWaterfall } from './attempt-waterfall'
import { BillingSection } from './proxy-log-detail-sheet'
import { RelayBridgeTraceCard } from './relay-bridge-trace'
import { StreamTimingCard } from './stream-timing-card'
import { TimingCell } from './timing-cell'
import { UsageCell } from './usage-cell'

export function GatewayRequestAttempts({
  detail,
  locale,
  selectedId,
  setAttemptId,
  attemptQuery,
}: {
  detail: GatewayRequestDetail
  locale: string
  selectedId?: number
  setAttemptId: Dispatch<SetStateAction<number | null>>
  attemptQuery: ReturnType<typeof useProxyLog>
}) {
  const { t } = useTranslation()
  const scaleMaxMs = useMemo(() => {
    const values = [
      detail.latencyMs,
      ...detail.attempts.map((attempt) => attempt.latencyMs ?? 0),
    ].filter((v) => typeof v === 'number' && Number.isFinite(v) && v > 0)
    return values.length > 0 ? Math.max(...values) : 0
  }, [detail])
  return (
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
      {(detail.partial || detail.attemptCount !== detail.attempts.length) && (
        <p role='status' className='text-warning text-sm'>
          {t('requestLogs.partialAttempts', {
            recorded: detail.attempts.length,
            expected: detail.attemptCount,
          })}
        </p>
      )}
      <ol className='space-y-2'>
        {detail.attempts.map((attempt) => {
          const isSelectedAttempt = selectedId === attempt.id
          const effectiveStreamTiming =
            attempt.streamTiming ??
            (isSelectedAttempt
              ? (attemptQuery.data?.streamTiming ?? null)
              : null)
          const effectiveConversion =
            attempt.conversion ??
            (isSelectedAttempt ? (attemptQuery.data?.conversion ?? null) : null)

          return (
            <li key={attempt.id} className='rounded-lg border'>
              <details
                open={isSelectedAttempt}
                onToggle={(event) => {
                  if (event.currentTarget.open) setAttemptId(attempt.id)
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
                      firstOutputLatencyMs={attempt.firstOutputLatencyMs}
                      isStream={attempt.isStream}
                      streamTiming={effectiveStreamTiming}
                      scaleMaxMs={scaleMaxMs}
                      compact={
                        isSelectedAttempt && effectiveStreamTiming !== null
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
                      .join(' · ') || t('requestLogs.unknownCandidate')}{' '}
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
                        parseProxyLogPathMeta(attempt.errorMessage).errorMessage
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
                    <RelayBridgeTraceCard conversion={effectiveConversion} />
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
  )
}
