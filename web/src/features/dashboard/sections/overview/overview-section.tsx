import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { QueryErrorBanner } from '@/components/common/query-error-banner'
import { api } from '@/lib/api'
import { formatCurrency, formatInt } from '@/lib/format'

import { AnnouncementBanner } from '../../components/announcement-banner'
import { OnboardingChecklist } from '../../components/onboarding-checklist'
import { AttentionPanel } from './components/attention-panel'
import { MaintenancePanel } from './components/maintenance-panel'
import { ModelUsagePanel } from './components/model-usage-panel'
import { RequestMetrics } from './components/request-metrics'
import { RequestTrend } from './components/request-trend'
import { UpstreamHealthPanel } from './components/upstream-health-panel'

type ResourceSnapshot = {
  siteCount?: number | null
  accountCount?: number | null
  totalAccounts?: number | null
  activeAccounts?: number | null
  totalBalance?: number | null
  todayCheckin?: {
    total: number
    success: number
    skipped: number
    failed: number
  } | null
}

export function OverviewSection() {
  const { t } = useTranslation()
  const snapshot = useQuery({
    queryKey: ['dashboard-snapshot'],
    queryFn: () => api.getDashboardSnapshot() as Promise<ResourceSnapshot>,
    refetchInterval: 30_000,
  })
  const data = snapshot.data
  const accounts = data?.totalAccounts ?? data?.accountCount
  return (
    <div className='flex flex-col gap-4'>
      <AnnouncementBanner />
      <OnboardingChecklist
        siteCount={data?.siteCount ?? undefined}
        accountCount={accounts ?? undefined}
      />
      <RequestMetrics />
      <div className='grid min-w-0 gap-4 xl:grid-cols-[minmax(0,1.7fr)_minmax(320px,1fr)]'>
        <RequestTrend />
        <AttentionPanel />
      </div>
      <div className='grid min-w-0 gap-4 xl:grid-cols-[minmax(0,1.7fr)_minmax(320px,1fr)]'>
        <UpstreamHealthPanel />
        <ModelUsagePanel />
      </div>
      <section
        className='space-y-2 rounded-lg border px-4 py-3'
        aria-label={t('dashboard.operations.resources')}
      >
        <QueryErrorBanner
          error={snapshot.error}
          messageKey='dashboard.overview.error.snapshotLoad'
          onRetry={() => void snapshot.refetch()}
          isRetrying={snapshot.isFetching}
        />
        <div className='text-muted-foreground flex flex-wrap items-center gap-x-6 gap-y-2 text-xs'>
          <span className='text-foreground font-medium'>
            {t('dashboard.operations.resources')}
          </span>
          <Link to='/sites' className='hover:text-primary'>
            {t('dashboard.operations.siteCount', {
              value: formatInt(data?.siteCount ?? null),
            })}
          </Link>
          <Link to='/accounts' className='hover:text-primary'>
            {t('dashboard.operations.accountCount', {
              value: formatInt(accounts ?? null),
              active: formatInt(data?.activeAccounts ?? null),
            })}
          </Link>
          <Link to='/accounts' className='hover:text-primary'>
            {t('dashboard.operations.balance', {
              value:
                data?.totalBalance == null
                  ? '—'
                  : formatCurrency(data.totalBalance),
            })}
          </Link>
          <Link to='/checkin' className='hover:text-primary'>
            {t('dashboard.operations.checkin', {
              success: formatInt(data?.todayCheckin?.success ?? null),
              total: formatInt(data?.todayCheckin?.total ?? null),
            })}
          </Link>
        </div>
      </section>
      <MaintenancePanel />
    </div>
  )
}
