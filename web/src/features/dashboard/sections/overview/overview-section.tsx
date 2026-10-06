import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { CalendarDays, RefreshCw } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { QueryErrorBanner } from '@/components/common/query-error-banner'
import { Button } from '@/components/ui/button'
import { api, type OverviewPeriod } from '@/lib/api'
import { formatCurrency, formatInt } from '@/lib/format'

import { AnnouncementBanner } from '../../components/announcement-banner'
import { OnboardingChecklist } from '../../components/onboarding-checklist'
import { AttentionPanel } from './components/attention-panel'
import { MaintenancePanel } from './components/maintenance-panel'
import { ModelUsagePanel } from './components/model-usage-panel'
import { RequestMetrics } from './components/request-metrics'
import { RequestTrend } from './components/request-trend'
import { UpstreamHealthPanel } from './components/upstream-health-panel'
import { useOverviewReport } from './use-overview-report'

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

const PERIOD_KEY = 'metapi.overview.period'
const PERIODS: OverviewPeriod[] = ['24h', '7d', '30d', 'all']

function readPeriod(): OverviewPeriod {
  try {
    const saved = localStorage.getItem(PERIOD_KEY) as OverviewPeriod | null
    if (saved && PERIODS.includes(saved)) return saved
  } catch {
    /* Storage can be unavailable; range selection still works. */
  }
  return '7d'
}

export function OverviewSection() {
  const { t } = useTranslation()
  const [period, setPeriod] = useState<OverviewPeriod>(readPeriod)
  function selectPeriod(value: OverviewPeriod) {
    setPeriod(value)
    try {
      localStorage.setItem(PERIOD_KEY, value)
    } catch {
      /* Keep the current in-memory selection. */
    }
  }
  const report = useOverviewReport(period)
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
      <div className='flex flex-wrap items-center justify-between gap-3'>
        <div className='text-muted-foreground flex items-center gap-2 text-sm'>
          <CalendarDays className='size-4' aria-hidden='true' />
          <span>{t('dashboard.operations.rangeLabel')}</span>
          <span className='text-foreground font-medium'>
            {t(`dashboard.operations.period.${period}`)}
          </span>
        </div>
        <div className='flex flex-wrap items-center gap-2'>
          <div
            role='group'
            aria-label={t('dashboard.operations.rangeLabel')}
            className='bg-muted flex items-center gap-1 rounded-lg p-1'
          >
            {PERIODS.map((value) => (
              <Button
                key={value}
                size='sm'
                variant={period === value ? 'secondary' : 'ghost'}
                aria-pressed={period === value}
                onClick={() => selectPeriod(value)}
                className='h-7 px-3'
              >
                {t(`dashboard.operations.period.${value}`)}
              </Button>
            ))}
          </div>
          <Button
            variant='outline'
            size='icon'
            aria-label={t('dashboard.operations.refresh')}
            disabled={report.isFetching}
            onClick={() => void report.refetch()}
          >
            <RefreshCw
              className={report.isFetching ? 'size-4 animate-spin' : 'size-4'}
            />
          </Button>
        </div>
      </div>
      <RequestMetrics period={period} />
      <div className='grid min-w-0 gap-4 xl:grid-cols-[minmax(0,1.7fr)_minmax(320px,1fr)]'>
        <RequestTrend period={period} />
        <ModelUsagePanel period={period} />
      </div>
      <div className='grid min-w-0 gap-4 xl:grid-cols-[minmax(0,1.7fr)_minmax(320px,1fr)]'>
        <UpstreamHealthPanel period={period} />
        <AttentionPanel />
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
