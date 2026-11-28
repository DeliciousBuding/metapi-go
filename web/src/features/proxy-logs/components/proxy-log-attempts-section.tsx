import { useTranslation } from 'react-i18next'

import { HttpStatusBadge } from '@/components/common/http-status-badge'
import { Separator } from '@/components/ui/separator'
import type { ProxyLogAttempt } from '@/lib/api'
import { EM_DASH, formatInt } from '@/lib/format'

import type { ProxyLogDetail } from '../types'

/**
 * Request-level attempt view (route-decision D batch). Every dispatch attempt
 * persists its own log row; the backend correlates siblings by requestId and
 * reports ordinal gaps / missing correlation honestly via partialReasons.
 */
export function AttemptsSection({
  detail,
  onViewAttempt,
}: {
  detail: ProxyLogDetail
  onViewAttempt?: (attempt: ProxyLogAttempt) => void
}) {
  const { t } = useTranslation()
  const ra = detail.requestAttempts
  if (!ra) return null

  const partialReasonText = (reason: string): string => {
    switch (reason) {
      case 'ordinalGap':
        return t('proxyLogs.detail.attempts.partialOrdinalGap')
      case 'requestIdMissing':
        return t('proxyLogs.detail.attempts.partialRequestIdMissing')
      case 'attemptQueryFailed':
        return t('proxyLogs.detail.attempts.partialAttemptQueryFailed')
      default:
        return reason
    }
  }

  return (
    <>
      <Separator />
      <section>
        <h3 className='mb-2 text-sm font-medium'>
          {t('proxyLogs.detail.attempts.title')}
        </h3>
        <div className='space-y-2 text-sm'>
          <div className='flex flex-wrap items-center gap-x-3 gap-y-1'>
            <span>
              {t('proxyLogs.detail.attempts.summary', {
                count: ra.attempts.length,
              })}
            </span>
            {ra.lastAttempt ? (
              <span className='text-muted-foreground inline-flex items-center gap-1'>
                {t('proxyLogs.detail.attempts.lastAttempt')}
                <HttpStatusBadge
                  status={ra.lastAttempt.status}
                  httpStatus={ra.lastAttempt.httpStatus}
                />
              </span>
            ) : (
              <span className='text-muted-foreground'>
                {t('proxyLogs.detail.attempts.lastAttemptUnknown')}
              </span>
            )}
            {ra.partial ? (
              <span className='text-warning-soft-fg bg-warning/10 inline-flex items-center rounded px-1.5 py-0.5 text-xs'>
                {t('proxyLogs.detail.attempts.partial')}
              </span>
            ) : null}
          </div>
          {ra.partial && ra.partialReasons.length > 0 ? (
            <ul className='text-muted-foreground list-disc space-y-1 pl-5 text-xs'>
              {ra.partialReasons.map((reason) => (
                <li key={reason}>{partialReasonText(reason)}</li>
              ))}
            </ul>
          ) : null}
          {ra.attempts.length > 0 ? (
            <ol className='divide-border divide-y rounded-md border'>
              {ra.attempts.map((attempt) => (
                <li key={attempt.id}>
                  <button
                    type='button'
                    disabled={attempt.isCurrent || !onViewAttempt}
                    onClick={() => onViewAttempt?.(attempt)}
                    className='hover:bg-muted/50 flex w-full flex-wrap items-center gap-x-2 gap-y-0.5 px-2 py-1.5 text-left text-xs disabled:cursor-default disabled:hover:bg-transparent'
                  >
                    <span className='text-muted-foreground font-mono'>
                      {`#${attempt.ordinal + 1}`}
                    </span>
                    <HttpStatusBadge
                      status={attempt.status}
                      httpStatus={attempt.httpStatus}
                    />
                    <span className='min-w-0 flex-1 truncate'>
                      {attempt.siteName ||
                        attempt.accountUsername ||
                        attempt.modelActual ||
                        EM_DASH}
                    </span>
                    <span className='text-muted-foreground'>
                      {formatInt(attempt.latencyMs)} ms
                    </span>
                    {attempt.isCurrent ? (
                      <span className='text-primary'>
                        {t('proxyLogs.detail.attempts.current')}
                      </span>
                    ) : null}
                  </button>
                  {attempt.errorMessage ? (
                    <p className='text-destructive-soft-fg truncate px-2 pb-1.5 text-xs'>
                      {attempt.errorMessage}
                    </p>
                  ) : null}
                </li>
              ))}
            </ol>
          ) : (
            <p className='text-muted-foreground text-xs'>
              {t('proxyLogs.detail.attempts.uncorrelated')}
            </p>
          )}
        </div>
      </section>
    </>
  )
}
