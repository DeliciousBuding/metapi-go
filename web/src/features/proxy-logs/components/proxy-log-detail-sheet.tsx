// metapi-go/features/proxy-logs/components — proxy log detail Sheet.
// i18n: all user-visible strings migrated to t() calls.

import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { Sheet, SheetContent } from '@/components/ui/sheet'
import { Spinner } from '@/components/ui/spinner'
import type { ProxyLogAttempt } from '@/lib/api'

import { useProxyLog } from '../api'
import { useDetailSheetKeyboardNav } from '../lib/detail-navigation'
import type { ProxyLog, ProxyLogDetail } from '../types'
import { DetailDiagnostics } from './proxy-log-detail-diagnostics'
import { DetailPayload } from './proxy-log-detail-payload'
import {
  DetailHeader,
  DetailOverview,
  type DetailNavigation,
} from './proxy-log-detail-summary'

export { BillingSection } from './billing-section'
export { CurrentRouteDiagnosis } from './current-route-diagnosis'

type ProxyLogDetailSheetProps = DetailNavigation & {
  log: ProxyLog | null
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Switch the sheet to another attempt of the same request. */
  onViewAttempt?: (attempt: ProxyLogAttempt) => void
}

export function ProxyLogDetailSheet({
  log,
  open,
  onOpenChange,
  onViewAttempt,
  hasPrev,
  hasNext,
  onNavigatePrev,
  onNavigateNext,
}: ProxyLogDetailSheetProps) {
  const { t } = useTranslation()
  const detailQuery = useProxyLog(log?.id ?? null)
  const detail: ProxyLogDetail | null =
    detailQuery.data ?? (log ? ({ ...log } as unknown as ProxyLogDetail) : null)
  useDetailSheetKeyboardNav({
    open: open && log !== null,
    hasPrev,
    hasNext,
    onPrev: onNavigatePrev,
    onNext: onNavigateNext,
  })

  if (!log) {
    return (
      <Sheet open={open} onOpenChange={onOpenChange}>
        <SheetContent side='right' className='sm:max-w-lg' />
      </Sheet>
    )
  }

  const isDetailLoading = detailQuery.isLoading && detailQuery.isFetching

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side='right'
        className='flex w-full flex-col gap-0 sm:max-w-lg'
      >
        <DetailHeader
          log={log}
          detail={detail}
          hasPrev={hasPrev}
          hasNext={hasNext}
          onNavigatePrev={onNavigatePrev}
          onNavigateNext={onNavigateNext}
        />

        <div className='flex-1 space-y-4 overflow-y-auto p-4'>
          {isDetailLoading && (
            <div className='text-muted-foreground flex items-center justify-center gap-2 py-6 text-sm'>
              <Spinner className='size-4' />
              {t('proxyLogs.detail.loading')}
            </div>
          )}
          {detail && (
            <>
              {detail.gatewayRequestId && (
                <Link
                  to='/proxy-logs'
                  search={{
                    view: 'requests',
                    requestId: detail.gatewayRequestId,
                  }}
                  className='text-primary text-sm hover:underline'
                >
                  {t('requestLogs.backToRequests')}
                </Link>
              )}
              <DetailOverview detail={detail} />
              <DetailDiagnostics
                detail={detail}
                onViewAttempt={onViewAttempt}
              />
              <DetailPayload detail={detail} />
            </>
          )}
        </div>
      </SheetContent>
    </Sheet>
  )
}
