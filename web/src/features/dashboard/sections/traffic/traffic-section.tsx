// metapi-go/features/dashboard/sections/traffic — traffic section.
//
// Plan §5.5.1 traffic: traffic trend chart（IncomeOutcome / SiteTrend）+
// SiteDistribution donut. Three charts built on the shared recharts-based
// Chart components, fed by useChartColors-free CSS-var() theming. Phase 3
// reshapes api.getBalanceIncomeOutcome / getSiteTrend / getSiteDistribution
// responses into the chart data types.

import { useQuery } from '@tanstack/react-query'
import { Inbox, TriangleAlert } from 'lucide-react'
import { useMemo, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { api } from '@/lib/api'
import { formatInt } from '@/lib/format'

import { ChartDataTable } from '../../components/chart-data-table'
import { ChartShell } from '../../components/chart-shell'
import {
  IncomeOutcomeChart,
  SiteDistributionChart,
  SiteTrendChart,
} from '../../components/charts'
import { formatChartCurrency } from '../../components/charts-currency'
import type {
  IncomeOutcomePoint,
  SiteDistributionSlice,
  SiteTrendPoint,
} from '../../types'

/** Income vs outcome response (GET /api/stats/balance-income-outcome). */
type IncomeOutcomeResponse = {
  days: number
  points: Array<{ day: string; income: number; outcome: number; net: number }>
  summary: { totalIncome: number; totalOutcome: number; net: number }
}

/** Site trend response (GET /api/stats/site-trend). */
type SiteTrendResponse = {
  trend: Array<{
    date: string
    sites: Record<string, { spend: number; calls: number }>
  }>
}

/** Site distribution response (GET /api/stats/site-distribution). */
type SiteDistributionResponse = {
  distribution: Array<{
    siteId: number
    siteName: string
    platform: string
    /** null when the site has accounts but no known balance (never $0). */
    totalBalance: number | null
    totalSpend: number
    accountCount: number
  }>
}

function ChartError({
  message,
  onRetry,
}: {
  message: string
  onRetry?: () => void
}) {
  const { t } = useTranslation()
  return (
    <div className='flex h-full w-full flex-col items-center justify-center gap-1.5'>
      <TriangleAlert className='text-destructive/80 size-5' />
      <p className='text-destructive text-xs'>{message}</p>
      {onRetry ? (
        <Button type='button' variant='outline' size='sm' onClick={onRetry}>
          {t('common.retry')}
        </Button>
      ) : null}
    </div>
  )
}

function ChartEmpty({ message }: { message: string }) {
  return (
    <div className='flex h-full w-full flex-col items-center justify-center gap-1.5'>
      <Inbox className='text-muted-foreground/60 size-5' />
      <p className='text-muted-foreground text-xs'>{message}</p>
    </div>
  )
}

/** Gate the sr-only summary exactly like the chart body: no summary while
 * the owning query is loading or failed; the builders themselves return
 * undefined when the loaded data is empty (S10, #1035). */
function summaryWhenLoaded(
  query: { isLoading: boolean; isError: boolean },
  summaryNode: ReactNode
): ReactNode {
  if (query.isLoading || query.isError) return undefined
  return summaryNode
}

export function TrafficSection() {
  const { t } = useTranslation()

  const incomeOutcomeQuery = useQuery({
    queryKey: ['dashboard-balance-income-outcome', 30],
    queryFn: () =>
      api.getBalanceIncomeOutcome(30) as Promise<IncomeOutcomeResponse>,
  })

  const siteTrendQuery = useQuery({
    queryKey: ['dashboard-site-trend', 7],
    queryFn: () => api.getSiteTrend(7) as Promise<SiteTrendResponse>,
  })

  const siteDistributionQuery = useQuery({
    queryKey: ['dashboard-site-distribution'],
    queryFn: () =>
      api.getSiteDistribution() as Promise<SiteDistributionResponse>,
  })

  const incomeOutcomeData = useMemo<IncomeOutcomePoint[]>(() => {
    const points = incomeOutcomeQuery.data?.points
    if (!points) return []
    const rows: IncomeOutcomePoint[] = []
    for (const point of points) {
      rows.push({ day: point.day, type: 'income', value: point.income })
      rows.push({ day: point.day, type: 'outcome', value: point.outcome })
    }
    return rows
  }, [incomeOutcomeQuery.data])

  const siteTrendData = useMemo<SiteTrendPoint[]>(() => {
    const trend = siteTrendQuery.data?.trend
    if (!trend) return []
    const rows: SiteTrendPoint[] = []
    for (const entry of trend) {
      for (const [siteName, metrics] of Object.entries(entry.sites)) {
        rows.push({
          date: entry.date,
          site: siteName,
          spend: metrics.spend,
          calls: metrics.calls,
        })
      }
    }
    return rows
  }, [siteTrendQuery.data])

  const siteDistributionData = useMemo<SiteDistributionSlice[]>(() => {
    const distribution = siteDistributionQuery.data?.distribution
    if (!distribution) return []
    return distribution.map((slice) => ({
      siteName: slice.siteName,
      platform: slice.platform,
      // The donut renders balance share; an unknown balance contributes 0
      // to the share instead of NaN.
      totalBalance: slice.totalBalance ?? 0,
      totalSpend: slice.totalSpend,
      accountCount: slice.accountCount,
    }))
  }, [siteDistributionQuery.data])

  // Same all-zero guard as the model-cost donut: zero total balance leaves
  // the donut empty, so render an explicit empty state instead.
  const siteDistributionTotal = useMemo(
    () =>
      siteDistributionData.reduce((sum, slice) => sum + slice.totalBalance, 0),
    [siteDistributionData]
  )

  const siteDistributionLabels = useMemo(
    () => ({
      balance: t('dashboard.traffic.siteDistribution.tooltip.balance'),
      accounts: t('dashboard.traffic.siteDistribution.tooltip.accounts'),
      share: t('dashboard.traffic.siteDistribution.tooltip.share'),
    }),
    [t]
  )

  // ---------------------------------------------------------------------
  // S10 (#1035): sr-only data summary tables. recharts SVG is opaque to
  // screen readers, so each chart's already-loaded data is re-presented as
  // a simple series x key-points table. Read-only presentation layer — no
  // extra requests, no data-semantics changes.
  // ---------------------------------------------------------------------

  const incomeOutcomeSummary = useMemo(() => {
    const points = incomeOutcomeQuery.data?.points
    const totals = incomeOutcomeQuery.data?.summary
    const last = points?.at(-1)
    if (!points?.length || !totals || !last) return undefined
    return (
      <ChartDataTable
        caption={t('dashboard.traffic.incomeOutcome.title')}
        seriesLabel={t('dashboard.chartSummary.seriesColumn')}
        columns={[
          t('dashboard.chartSummary.totalColumn'),
          t('dashboard.chartSummary.latestColumn'),
        ]}
        rows={[
          {
            name: t('dashboard.charts.income'),
            values: [
              formatChartCurrency(totals.totalIncome),
              formatChartCurrency(last.income),
            ],
          },
          {
            name: t('dashboard.charts.outcome'),
            values: [
              formatChartCurrency(totals.totalOutcome),
              formatChartCurrency(last.outcome),
            ],
          },
          {
            name: t('dashboard.chartSummary.netRow'),
            values: [
              formatChartCurrency(totals.net),
              formatChartCurrency(last.net),
            ],
          },
        ]}
      />
    )
  }, [incomeOutcomeQuery.data, t])

  const siteTrendSummary = useMemo(() => {
    if (siteTrendData.length === 0) return undefined
    const bySite = new Map<string, { spend: number; calls: number }>()
    for (const point of siteTrendData) {
      const acc = bySite.get(point.site) ?? { spend: 0, calls: 0 }
      acc.spend += point.spend
      acc.calls += point.calls
      bySite.set(point.site, acc)
    }
    return (
      <ChartDataTable
        caption={t('dashboard.traffic.siteTrend.title')}
        seriesLabel={t('dashboard.chartSummary.seriesColumn')}
        columns={[
          t('dashboard.chartSummary.spendColumn'),
          t('dashboard.chartSummary.callsColumn'),
        ]}
        rows={[...bySite.entries()].map(([site, totals]) => ({
          name: site,
          values: [formatChartCurrency(totals.spend), formatInt(totals.calls)],
        }))}
      />
    )
  }, [siteTrendData, t])

  const siteDistributionSummary = useMemo(() => {
    if (siteDistributionData.length === 0 || siteDistributionTotal === 0) {
      return undefined
    }
    return (
      <ChartDataTable
        caption={t('dashboard.traffic.siteDistribution.title')}
        seriesLabel={t('dashboard.chartSummary.seriesColumn')}
        columns={[
          t('dashboard.traffic.siteDistribution.tooltip.balance'),
          t('dashboard.chartSummary.spendColumn'),
          t('dashboard.traffic.siteDistribution.tooltip.accounts'),
          t('dashboard.traffic.siteDistribution.tooltip.share'),
        ]}
        rows={siteDistributionData.map((slice) => ({
          name: slice.siteName,
          values: [
            formatChartCurrency(slice.totalBalance),
            formatChartCurrency(slice.totalSpend),
            formatInt(slice.accountCount),
            `${((slice.totalBalance / siteDistributionTotal) * 100).toFixed(1)}%`,
          ],
        }))}
      />
    )
  }, [siteDistributionData, siteDistributionTotal, t])

  const renderChartBody = (
    query: {
      isLoading: boolean
      isError: boolean
      refetch: () => void
    },
    isEmpty: boolean,
    emptyKey: string,
    chart: ReactNode
  ): ReactNode => {
    if (query.isLoading) return null
    if (query.isError) {
      return (
        <ChartError
          message={t('dashboard.traffic.loadError')}
          onRetry={() => void query.refetch()}
        />
      )
    }
    if (isEmpty) {
      return <ChartEmpty message={t(emptyKey)} />
    }
    return chart
  }

  return (
    <div className='grid grid-cols-1 gap-4 lg:grid-cols-2'>
      <ChartShell
        title={t('dashboard.traffic.incomeOutcome.title')}
        description={t('dashboard.traffic.incomeOutcome.description')}
        height={300}
        loading={incomeOutcomeQuery.isLoading}
        summary={summaryWhenLoaded(incomeOutcomeQuery, incomeOutcomeSummary)}
      >
        {renderChartBody(
          incomeOutcomeQuery,
          incomeOutcomeData.length === 0,
          'dashboard.traffic.incomeOutcome.empty',
          <IncomeOutcomeChart data={incomeOutcomeData} />
        )}
      </ChartShell>

      <ChartShell
        title={t('dashboard.traffic.siteTrend.title')}
        description={t('dashboard.traffic.siteTrend.description')}
        height={300}
        loading={siteTrendQuery.isLoading}
        summary={summaryWhenLoaded(siteTrendQuery, siteTrendSummary)}
      >
        {renderChartBody(
          siteTrendQuery,
          siteTrendData.length === 0,
          'dashboard.traffic.siteTrend.empty',
          <SiteTrendChart data={siteTrendData} />
        )}
      </ChartShell>

      <ChartShell
        title={t('dashboard.traffic.siteDistribution.title')}
        description={t('dashboard.traffic.siteDistribution.description')}
        height={300}
        className='lg:col-span-2'
        loading={siteDistributionQuery.isLoading}
        summary={summaryWhenLoaded(
          siteDistributionQuery,
          siteDistributionSummary
        )}
      >
        {renderChartBody(
          siteDistributionQuery,
          siteDistributionData.length === 0,
          'dashboard.traffic.siteDistribution.empty',
          siteDistributionTotal === 0 ? (
            <ChartEmpty
              message={t('dashboard.traffic.siteDistribution.zeroBalance')}
            />
          ) : (
            <SiteDistributionChart
              data={siteDistributionData}
              labels={siteDistributionLabels}
            />
          )
        )}
      </ChartShell>
    </div>
  )
}
