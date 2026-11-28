import { Link } from '@tanstack/react-router'
import { ChevronDown, ChevronUp, FlaskConical } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

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
import { buildModelTesterSearch } from '@/lib/helpers/model-tester-link'

import { useProxyLog } from '../api'
import { useDetailSheetKeyboardNav } from '../lib/detail-navigation'
import { useGatewayRequest } from '../lib/gateway-requests'
import { GatewayRequestAttempts } from './gateway-request-attempts'
import { GatewayRequestSummary } from './gateway-request-summary'
import { CurrentRouteDiagnosis } from './proxy-log-detail-sheet'

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
              <GatewayRequestSummary detail={detail} locale={locale} />
              <CurrentRouteDiagnosis
                model={detail.modelRequested}
                endpoint={endpoint}
                keyId={detail.downstreamApiKeyId}
              />
              <GatewayRequestAttempts
                detail={detail}
                locale={locale}
                selectedId={selectedAttempt?.id}
                setAttemptId={setAttemptId}
                attemptQuery={attemptQuery}
              />
            </>
          )}
        </div>
      </SheetContent>
    </Sheet>
  )
}
