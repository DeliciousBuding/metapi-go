// metapi-go/features/dashboard/sections/traffic — traffic section.
//
// Plan §5.5.1 traffic:流量趋势图（IncomeOutcome / SiteTrend）+
// SiteDistribution 饼图. Three VChart charts wired with useChartColors +
// ChartShell. Phase 2 feeds empty arrays (VChart renders an empty canvas);
// phase 3 reshapes api.getBalanceIncomeOutcome / getSiteTrend /
// getSiteDistribution responses into the chart data types.

import { useMemo } from 'react'
import { VChart } from '@visactor/react-vchart'

import { useTheme } from '@/context/theme-provider'

import { ChartShell } from '../../components/chart-shell'
import { useChartColors, useThemeLabelColor } from '../../hooks/use-chart-colors'
import {
  VCHART_OPTION,
  buildIncomeOutcomeSpec,
  buildSiteDistributionSpec,
  buildSiteTrendSpec,
} from '../../lib/chart-specs'
import type {
  IncomeOutcomePoint,
  SiteDistributionSlice,
  SiteTrendPoint,
} from '../../types'

// TODO phase 3: replace these empty arrays with TanStack Query data:
//   api.getBalanceIncomeOutcome(days) → flatten to IncomeOutcomePoint[]
//   api.getSiteTrend(days)            → flatten to SiteTrendPoint[]
//   api.getSiteDistribution()         → map  to SiteDistributionSlice[]
const INCOME_OUTCOME_DATA: IncomeOutcomePoint[] = []
const SITE_TREND_DATA: SiteTrendPoint[] = []
const SITE_DISTRIBUTION_DATA: SiteDistributionSlice[] = []

export function TrafficSection() {
  const colors = useChartColors()
  const labelColor = useThemeLabelColor()
  const { resolvedTheme } = useTheme()

  const incomeOutcomeSpec = useMemo(
    () => buildIncomeOutcomeSpec(colors, INCOME_OUTCOME_DATA),
    [colors],
  )
  const siteTrendSpec = useMemo(
    () => buildSiteTrendSpec(colors, SITE_TREND_DATA),
    [colors],
  )
  const siteDistributionSpec = useMemo(
    () => buildSiteDistributionSpec(colors, labelColor, SITE_DISTRIBUTION_DATA),
    [colors, labelColor],
  )

  // Force a remount on theme flip so VChart redraws with the fresh palette
  // (matches newapi's key-with-theme approach — avoids stale canvas state).
  const remountKey = `traffic-${resolvedTheme}`

  return (
    <div className='grid grid-cols-1 gap-4 lg:grid-cols-2'>
      <ChartShell
        title='Income vs outcome'
        description='Daily balance inflow vs spend (30d).'
        height={300}
      >
        <VChart
          key={`${remountKey}-income`}
          spec={incomeOutcomeSpec as never}
          option={VCHART_OPTION}
          style={{ width: '100%', height: '100%' }}
        />
      </ChartShell>

      <ChartShell
        title='Site trend'
        description='Per-site spend over time.'
        height={300}
      >
        <VChart
          key={`${remountKey}-site-trend`}
          spec={siteTrendSpec as never}
          option={VCHART_OPTION}
          style={{ width: '100%', height: '100%' }}
        />
      </ChartShell>

      <ChartShell
        title='Site distribution'
        description='Balance share by site.'
        height={300}
        className='lg:col-span-2'
      >
        <VChart
          key={`${remountKey}-site-dist`}
          spec={siteDistributionSpec as never}
          option={VCHART_OPTION}
          style={{ width: '100%', height: '100%' }}
        />
      </ChartShell>
    </div>
  )
}
