import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { Activity, Coins, ShieldCheck, Zap } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { QueryErrorBanner } from '@/components/common/query-error-banner'
import { api } from '@/lib/api'
import { formatCurrency, formatInt } from '@/lib/format'

import { StatCard } from '../../../components/stat-card'

export function RequestMetrics() {
  const { t } = useTranslation()
  const query = useQuery({
    queryKey: ['dashboard', 'request-metrics', '24h'],
    queryFn: async () => {
      const end = new Date()
      const window = {
        from: new Date(end.getTime() - 24 * 60 * 60 * 1000).toISOString(),
        to: end.toISOString(),
      }
      // The unfiltered log aggregate includes disabled sites and unassigned
      // failures. Keep the returned window for exactly matching drill-downs.
      const data = await api.getProxyLogsMeta(window)
      return { ...data, window }
    },
    refetchInterval: 30_000,
  })
  const summary = query.data?.summary
  const hasRequests = summary !== undefined && summary.totalCount > 0
  const success = hasRequests
    ? `${((summary.successCount / summary.totalCount) * 100).toFixed(1)}%`
    : '—'
  const metrics = [
    {
      title: 'requests',
      value: summary ? formatInt(summary.totalCount) : '—',
      hint: t('dashboard.operations.recordedRequests'),
      icon: Activity,
      failed: false,
    },
    {
      title: 'success',
      value: success,
      hint: hasRequests
        ? t('dashboard.operations.failures', { count: summary.failedCount })
        : t('dashboard.operations.noRequests'),
      icon: ShieldCheck,
      failed: true,
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
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <h2 className='text-sm font-semibold'>
          {t('dashboard.operations.serviceOverview')}
        </h2>
        <span className='text-muted-foreground text-xs'>
          {t('dashboard.operations.rolling24h')}
        </span>
      </div>
      <QueryErrorBanner
        error={query.error}
        messageKey='dashboard.operations.loadError'
        onRetry={() => void query.refetch()}
        isRetrying={query.isFetching}
      />
      <div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-4'>
        {metrics.map((metric) => (
          <Link
            key={metric.title}
            to='/proxy-logs'
            search={{
              ...query.data?.window,
              ...(metric.failed ? { status: 'failed' as const } : {}),
            }}
            className='focus-visible:ring-ring block rounded-xl outline-none focus-visible:ring-2'
          >
            <StatCard
              title={t(`dashboard.operations.${metric.title}`)}
              value={metric.value}
              hint={metric.hint}
              icon={metric.icon}
              loading={query.isLoading}
              tone={
                metric.failed && hasRequests && summary.failedCount > 0
                  ? 'warning'
                  : 'default'
              }
            />
          </Link>
        ))}
      </div>
    </section>
  )
}
