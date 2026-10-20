import { Link } from '@tanstack/react-router'
import { Activity, Coins, Repeat, ShieldCheck, Zap } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { QueryErrorBanner } from '@/components/common/query-error-banner'
import { KpiValue } from '@/components/ui/kpi-value'
import { Skeleton } from '@/components/ui/skeleton'
import { toBcp47 } from '@/i18n/languages'
import type { OverviewPeriod } from '@/lib/api'
import { formatCurrency, formatDateTime, formatInt } from '@/lib/format'

import { useOverviewReport } from '../use-overview-report'

export function RequestMetrics({ period = '7d' }: { period?: OverviewPeriod }) {
  const { t, i18n } = useTranslation()
  const query = useOverviewReport(period)
  const summary = query.data?.summary
  const requestSummary = query.data?.requestSummary
  const gatewaySummary = query.data?.gatewayRequestSummary
  const useGatewayOutcomes = gatewaySummary !== undefined
  const displayedCount =
    gatewaySummary?.totalCount ?? requestSummary?.totalCount
  const completedCount =
    gatewaySummary?.completedCount ?? requestSummary?.successCount
  const hasRequests = displayedCount !== undefined && displayedCount > 0
  const success =
    displayedCount && completedCount !== undefined
      ? `${((completedCount / displayedCount) * 100).toFixed(1)}%`
      : '—'
  let successHint = '—'
  if (requestSummary) {
    successHint = hasRequests
      ? t('dashboard.operations.failures', {
          count: requestSummary.failedCount,
        })
      : t('dashboard.operations.noCorrelatedRequests')
  }
  if (gatewaySummary) {
    successHint = hasRequests
      ? t('requestLogs.outcomeCounts', {
          failed: gatewaySummary.failedCount,
          cancelled: gatewaySummary.cancelledCount,
          rejected: gatewaySummary.rejectedCount,
        })
      : t('requestLogs.noRequests')
  }
  const metrics = [
    {
      title: 'requests',
      value: displayedCount !== undefined ? formatInt(displayedCount) : '—',
      hint: useGatewayOutcomes ? null : t('dashboard.operations.requestHint'),
      icon: Activity,
      failed: false,
    },
    {
      title: 'requestSuccess',
      value: success,
      hint: successHint,
      icon: ShieldCheck,
      failed: true,
    },
    {
      title: 'attempts',
      value: summary ? formatInt(summary.totalCount) : '—',
      hint: t('dashboard.operations.attemptHint'),
      icon: Repeat,
      failed: false,
    },
    {
      title: 'tokens',
      value: summary ? formatInt(summary.totalTokensAll) : '—',
      hint: t('dashboard.operations.tokensHint'),
      icon: Zap,
      failed: false,
    },
    {
      title: 'cost',
      value: summary ? formatCurrency(summary.totalCost) : '—',
      hint: t('dashboard.operations.costHint'),
      icon: Coins,
      failed: false,
    },
  ]
  return (
    <section
      className='space-y-3'
      aria-label={t('dashboard.operations.serviceOverview')}
    >
      <QueryErrorBanner
        error={query.error}
        messageKey='dashboard.operations.loadError'
        onRetry={() => void query.refetch()}
        isRetrying={query.isFetching}
      />
      <div className='bg-card grid grid-cols-2 overflow-hidden rounded-xl border xl:grid-cols-5'>
        {metrics.map((metric) => {
          const Icon = metric.icon
          let label = `dashboard.operations.${metric.title}`
          if (useGatewayOutcomes && metric.title === 'requests')
            label = 'requestLogs.requestCount'
          if (useGatewayOutcomes && metric.failed)
            label = 'requestLogs.completionRate'
          return (
            <Link
              key={metric.title}
              to='/proxy-logs'
              search={{
                ...query.data?.window,
                ...(useGatewayOutcomes &&
                (metric.title === 'requests' || metric.failed)
                  ? { view: 'requests' as const }
                  : {
                      view: 'attempts' as const,
                      ...(metric.failed ? { status: 'failed' as const } : {}),
                    }),
              }}
              className='hover:bg-muted/40 focus-visible:ring-ring flex min-w-0 flex-col gap-2 border-b p-4 transition-colors outline-none last:col-span-2 last:border-r-0 last:border-b-0 odd:border-r focus-visible:ring-2 focus-visible:ring-inset xl:gap-3 xl:border-r xl:border-b-0 xl:p-5 xl:last:col-span-1'
            >
              <div className='flex min-h-7 items-center gap-2 text-xs font-medium sm:text-sm xl:min-h-8'>
                <Icon
                  aria-hidden='true'
                  className={
                    metric.failed &&
                    hasRequests &&
                    (gatewaySummary?.failedCount ?? requestSummary?.failedCount)
                      ? 'text-warning-soft-fg size-4 shrink-0'
                      : 'text-muted-foreground size-4 shrink-0'
                  }
                />
                {t(label)}
              </div>
              {query.isLoading ? (
                <Skeleton className='h-8 w-24' />
              ) : (
                <KpiValue className='text-2xl xl:text-3xl'>
                  {metric.value}
                </KpiValue>
              )}
              {metric.hint && (
                <span className='text-muted-foreground text-2xs leading-4'>
                  {metric.hint}
                </span>
              )}
            </Link>
          )
        })}
      </div>
      {gatewaySummary && (
        <p className='text-muted-foreground text-xs leading-5'>
          {t('requestLogs.observedHint')}{' '}
          {gatewaySummary.coverageStartedAt
            ? t('requestLogs.coverage', {
                time: formatDateTime(
                  gatewaySummary.coverageStartedAt,
                  toBcp47(i18n.language)
                ),
              })
            : t('requestLogs.noCoverage')}
        </p>
      )}
    </section>
  )
}
