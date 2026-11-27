import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Bar,
  BarChart,
  CartesianGrid,
  Line,
  LineChart,
  XAxis,
  YAxis,
} from 'recharts'

import {
  ChartContainer,
  ChartLegend,
  ChartLegendContent,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from '@/components/ui/chart'

import type { IncomeOutcomePoint, SiteTrendPoint } from '../types'
import {
  useCurrencyAxisTicks,
  useDateTickFormatter,
  useDateTooltipFormatter,
} from './chart-axis-formatters'
import {
  chartColor,
  seriesKey,
  pivotIncomeOutcome,
  pivotSiteTrend,
} from './chart-series'
import { formatChartCurrency } from './charts-currency'

/** Daily balance inflow vs spend — grouped bar chart (30d). */
export function IncomeOutcomeChart({ data }: { data: IncomeOutcomePoint[] }) {
  const { t } = useTranslation()
  const rows = useMemo(() => pivotIncomeOutcome(data), [data])
  const formatCurrencyTick = useCurrencyAxisTicks(rows)
  const formatTick = useDateTickFormatter()
  const formatTooltipDate = useDateTooltipFormatter()
  const config = useMemo<ChartConfig>(
    () => ({
      income: {
        label: t('dashboard.charts.income'),
        color: chartColor(0),
      },
      outcome: {
        label: t('dashboard.charts.outcome'),
        color: chartColor(1),
      },
    }),
    [t]
  )
  return (
    <ChartContainer
      config={config}
      className='h-full w-full'
      aria-label={t('dashboard.charts.incomeOutcomeTitle')}
    >
      <BarChart data={rows} margin={{ top: 8, right: 16, bottom: 0, left: 0 }}>
        <CartesianGrid vertical={false} strokeDasharray='4 4' />
        <XAxis
          dataKey='day'
          tickMargin={8}
          minTickGap={24}
          tickFormatter={formatTick}
        />
        <YAxis
          axisLine={false}
          tickLine={false}
          width={56}
          tickMargin={4}
          tickFormatter={formatCurrencyTick}
        />
        <ChartLegend
          content={<ChartLegendContent />}
          verticalAlign='bottom'
          height={36}
        />
        <ChartTooltip
          content={
            <ChartTooltipContent
              labelFormatter={(label) => formatTooltipDate(String(label))}
              formatter={(value) => formatChartCurrency(Number(value))}
            />
          }
        />
        <Bar
          dataKey='income'
          fill='var(--color-income)'
          maxBarSize={14}
          isAnimationActive={false}
        />
        <Bar
          dataKey='outcome'
          fill='var(--color-outcome)'
          maxBarSize={14}
          isAnimationActive={false}
        />
      </BarChart>
    </ChartContainer>
  )
}

/** Per-site spend over time — one line per site. */
export function SiteTrendChart({ data }: { data: SiteTrendPoint[] }) {
  const { t } = useTranslation()
  const { rows, sites } = useMemo(() => pivotSiteTrend(data), [data])
  const formatCurrencyTick = useCurrencyAxisTicks(rows)
  const formatTick = useDateTickFormatter()
  const formatTooltipDate = useDateTooltipFormatter()
  const config = useMemo<ChartConfig>(() => {
    const cfg: ChartConfig = {}
    sites.forEach((site, index) => {
      cfg[seriesKey(index)] = { label: site, color: chartColor(index) }
    })
    return cfg
  }, [sites])
  return (
    <ChartContainer
      config={config}
      className='h-full w-full'
      aria-label={t('dashboard.charts.siteTrendTitle')}
    >
      <LineChart data={rows} margin={{ top: 8, right: 16, bottom: 0, left: 0 }}>
        <CartesianGrid vertical={false} strokeDasharray='4 4' />
        <XAxis
          dataKey='date'
          tickMargin={8}
          minTickGap={24}
          tickFormatter={formatTick}
        />
        <YAxis
          axisLine={false}
          tickLine={false}
          width={56}
          tickMargin={4}
          tickFormatter={formatCurrencyTick}
        />
        <ChartLegend
          content={<ChartLegendContent />}
          verticalAlign='bottom'
          height={36}
        />
        <ChartTooltip
          content={
            <ChartTooltipContent
              labelFormatter={(label) => formatTooltipDate(String(label))}
              formatter={(value) => formatChartCurrency(Number(value))}
            />
          }
        />
        {sites.map((_, index) => {
          const key = seriesKey(index)
          return (
            <Line
              key={key}
              dataKey={key}
              type='monotone'
              stroke={`var(--color-${key})`}
              strokeWidth={2}
              dot={false}
              isAnimationActive={false}
            />
          )
        })}
      </LineChart>
    </ChartContainer>
  )
}
