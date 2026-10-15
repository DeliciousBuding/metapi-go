import { Link } from '@tanstack/react-router'
import { Activity, Coins, Repeat, ShieldCheck, Zap } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { QueryErrorBanner } from '@/components/common/query-error-banner'
import { IconBadge } from '@/components/ui/icon-badge'
import { KpiValue } from '@/components/ui/kpi-value'
import { Skeleton } from '@/components/ui/skeleton'
import type { OverviewPeriod } from '@/lib/api'
import { formatCurrency, formatInt } from '@/lib/format'

import { useOverviewReport } from '../use-overview-report'

export function RequestMetrics({ period = '7d' }: { period?: OverviewPeriod }) {
  const { t } = useTranslation()
  const query = useOverviewReport(period)
  const summary = query.data?.summary
  const requestSummary = query.data?.requestSummary
  const hasRequests =
    requestSummary !== undefined && requestSummary.totalCount > 0
  const success =
    requestSummary && requestSummary.totalCount > 0
      ? `${((requestSummary.successCount / requestSummary.totalCount) * 100).toFixed(1)}%`
      : '—'
  let successHint = '—'
  if (requestSummary) {
    successHint = hasRequests
      ? t('dashboard.operations.failures', {
          count: requestSummary.failedCount,
        })
      : t('dashboard.operations.noCorrelatedRequests')
  }
  const metrics = [
    {
      title: 'requests',
      value: requestSummary ? formatInt(requestSummary.totalCount) : '—',
      hint: t('dashboard.operations.requestHint'),
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
      <div className='bg-card grid overflow-hidden rounded-xl border sm:grid-cols-2 xl:grid-cols-5'>
        {metrics.map((metric) => {
          const Icon = metric.icon
          return (
            <Link
              key={metric.title}
              to='/proxy-logs'
              search={{
                ...query.data?.window,
                ...(metric.failed ? { status: 'failed' as const } : {}),
              }}
              className='hover:bg-muted/40 focus-visible:ring-ring flex min-w-0 flex-col gap-3 border-b p-5 transition-colors outline-none last:border-b-0 focus-visible:ring-2 focus-visible:ring-inset sm:odd:border-r xl:border-r xl:border-b-0 xl:last:border-r-0'
            >
              <div className='flex items-center gap-2 text-sm font-medium'>
                <IconBadge
                  size='sm'
                  tone={
                    metric.failed && hasRequests && requestSummary?.failedCount
                      ? 'warning'
                      : 'info'
                  }
                >
                  <Icon />
                </IconBadge>
                {t(`dashboard.operations.${metric.title}`)}
              </div>
              {query.isLoading ? (
                <Skeleton className='h-8 w-24' />
              ) : (
                <KpiValue>{metric.value}</KpiValue>
              )}
              <span className='text-muted-foreground text-xs leading-5'>
                {metric.hint}
              </span>
            </Link>
          )
        })}
      </div>
      <p className='text-muted-foreground text-xs'>
        {t('dashboard.operations.requestOutcomeBasis')}
      </p>
    </section>
  )
}
