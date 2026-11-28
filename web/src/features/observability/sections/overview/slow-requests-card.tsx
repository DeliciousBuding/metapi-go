import { Link } from '@tanstack/react-router'
import { ArrowUpRight, Flame } from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { HttpStatusBadge } from '@/components/common/http-status-badge'
import { ModelPill } from '@/components/common/model-pill'
import { buttonVariants } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { toBcp47 } from '@/i18n/languages'
import { formatDateTime, formatLatency } from '@/lib/format'

import { useSlowRequests } from '../../api'
import { ObservabilityErrorBanner } from '../../components/observability-error-banner'
import type { SlowRequestItem } from '../../types'

export function SlowRequestsCard() {
  const { t, i18n } = useTranslation()
  const locale = toBcp47(i18n.language || 'en')
  const slow = useSlowRequests({ limit: 20, minLatencyMs: 1000, hours: 24 })
  const slowItems = slow.data?.items ?? []

  return (
    <Card>
      <CardHeader>
        <CardTitle className='flex items-center gap-2'>
          <Flame className='size-4' />
          {t('observability.overview.slowRequests.title')}
        </CardTitle>
        <CardDescription className='text-xs'>
          {t('observability.overview.slowRequests.description')}
        </CardDescription>
      </CardHeader>
      <CardContent>
        {slow.isError && !slow.isLoading ? (
          <ObservabilityErrorBanner
            messageKey='observability.overview.slowRequests.loadFailed'
            isRetrying={slow.isFetching && !slow.isLoading}
            onRetry={() => void slow.refetch()}
          />
        ) : (
          renderSlowRequestsBody(slow.isLoading, slowItems, t, locale)
        )}
      </CardContent>
    </Card>
  )
}

function renderSlowRequestsBody(
  loading: boolean,
  items: SlowRequestItem[],
  t: (key: string, options?: Record<string, string | number>) => string,
  locale: string
): ReactNode {
  if (loading) {
    return <Skeleton className='h-48 w-full rounded-md' />
  }
  if (items.length === 0) {
    return (
      <div className='flex min-h-24 flex-col items-center justify-center gap-2 rounded-lg border border-dashed py-8 text-center'>
        <p className='text-muted-foreground text-sm'>
          {t('observability.overview.slowRequests.empty')}
        </p>
      </div>
    )
  }
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>
            {t('observability.overview.slowRequests.colModel')}
          </TableHead>
          <TableHead>
            {t('observability.overview.slowRequests.colSite')}
          </TableHead>
          <TableHead className='text-right'>
            {t('observability.overview.slowRequests.colLatency')}
          </TableHead>
          <TableHead className='text-right'>
            {t('observability.overview.slowRequests.colFirstByte')}
          </TableHead>
          <TableHead className='text-right'>
            {t('observability.overview.slowRequests.colHttp')}
          </TableHead>
          <TableHead>
            {t('observability.overview.slowRequests.colStatus')}
          </TableHead>
          <TableHead className='text-right'>
            {t('observability.overview.slowRequests.colTime')}
          </TableHead>
          <TableHead className='text-right'>
            {t('observability.overview.slowRequests.colActions')}
          </TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {items.map((item) => {
          const gatewayRequestId = item.gatewayRequestId?.trim() || ''
          const queryTerm = item.requestId?.trim() || item.model?.trim() || ''
          const logSearch = gatewayRequestId
            ? {
                view: 'requests' as const,
                requestId: gatewayRequestId,
              }
            : {
                view: 'attempts' as const,
                ...(queryTerm ? { q: queryTerm } : {}),
                ...(item.siteId && item.siteId > 0
                  ? { siteId: item.siteId }
                  : {}),
              }
          const firstByteText =
            item.firstByteLatencyMs === null ||
            item.firstByteLatencyMs === undefined
              ? '—'
              : formatLatency(item.firstByteLatencyMs, {
                  autoSeconds: true,
                  secondsDigits: 1,
                  spaced: true,
                })
          const firstOutputText =
            item.firstOutputLatencyMs === null ||
            item.firstOutputLatencyMs === undefined
              ? null
              : formatLatency(item.firstOutputLatencyMs, {
                  autoSeconds: true,
                  secondsDigits: 1,
                  spaced: true,
                })
          return (
            <TableRow key={item.id}>
              <TableCell
                className='max-w-56 truncate font-medium'
                title={item.model || undefined}
              >
                {item.model ? <ModelPill model={item.model} /> : '—'}
              </TableCell>
              <TableCell
                className='max-w-40 truncate'
                title={item.siteName || undefined}
              >
                {item.siteName || '—'}
              </TableCell>
              <TableCell className='text-right tabular-nums'>
                {formatLatency(item.latencyMs, {
                  autoSeconds: true,
                  secondsDigits: 1,
                  spaced: true,
                })}
              </TableCell>
              <TableCell className='text-right tabular-nums'>
                <div className='flex flex-col items-end gap-0.5'>
                  <span>{firstByteText}</span>
                  {firstOutputText && (
                    <span
                      className='text-muted-foreground text-2xs'
                      title={t(
                        'observability.overview.slowRequests.firstOutputTooltip'
                      )}
                    >
                      {t('observability.overview.slowRequests.firstOutput', {
                        value: firstOutputText,
                      })}
                    </span>
                  )}
                </div>
              </TableCell>
              <TableCell className='text-right tabular-nums'>
                {item.httpStatus || '—'}
              </TableCell>
              <TableCell>
                <HttpStatusBadge
                  httpStatus={item.httpStatus ?? null}
                  status={item.status ?? null}
                />
              </TableCell>
              <TableCell className='text-right tabular-nums'>
                {formatDateTime(item.createdAt, locale)}
              </TableCell>
              <TableCell className='text-right'>
                <Link
                  to='/proxy-logs'
                  search={logSearch}
                  aria-label={t(
                    'observability.overview.slowRequests.viewInLogs'
                  )}
                  className={buttonVariants({
                    size: 'icon-xs',
                    variant: 'ghost',
                  })}
                >
                  <ArrowUpRight className='size-3.5' aria-hidden='true' />
                </Link>
              </TableCell>
            </TableRow>
          )
        })}
      </TableBody>
    </Table>
  )
}
