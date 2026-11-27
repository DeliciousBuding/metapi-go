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

import {
  useDateTickFormatter,
  useDateTooltipFormatter,
} from './chart-axis-formatters'
import { chartColor, pivotLatencyTrend } from './chart-series'

const LATENCY_HISTOGRAM_CONFIG: ChartConfig = {
  count: { label: '', color: chartColor(0) },
}

/** Request count by latency bucket — single-series bar chart (7d). */
export function LatencyHistogramChart({
  data,
}: {
  data: Array<{ label: string; count: number }>
}) {
  const { t } = useTranslation()
  return (
    <ChartContainer
      config={LATENCY_HISTOGRAM_CONFIG}
      className='h-full w-full'
      aria-label={t('dashboard.charts.latencyHistogramTitle')}
    >
      <BarChart data={data} margin={{ top: 8, right: 16, bottom: 0, left: 0 }}>
        <CartesianGrid vertical={false} strokeDasharray='4 4' />
        <XAxis dataKey='label' tickMargin={8} minTickGap={12} />
        <YAxis
          axisLine={false}
          tickLine={false}
          width={56}
          tickMargin={4}
          allowDecimals={false}
        />
        <ChartTooltip content={<ChartTooltipContent />} />
        <Bar
          dataKey='count'
          fill='var(--color-count)'
          maxBarSize={24}
          isAnimationActive={false}
        />
      </BarChart>
    </ChartContainer>
  )
}

/** Average + p95 latency over time — dual-line chart (7d). */
export function LatencyTrendChart({
  data,
  avgLabel,
  p95Label,
}: {
  data: Array<{ date: string; metric: string; latency: number }>
  avgLabel: string
  p95Label: string
}) {
  const { t } = useTranslation()
  const rows = useMemo(
    () => pivotLatencyTrend(data, avgLabel, p95Label),
    [data, avgLabel, p95Label]
  )
  const formatTick = useDateTickFormatter()
  const formatTooltipDate = useDateTooltipFormatter()
  const config = useMemo<ChartConfig>(
    () => ({
      s0: { label: avgLabel, color: chartColor(0) },
      s1: { label: p95Label, color: chartColor(1) },
    }),
    [avgLabel, p95Label]
  )
  return (
    <ChartContainer
      config={config}
      className='h-full w-full'
      aria-label={t('dashboard.charts.latencyTrendTitle')}
    >
      <LineChart data={rows} margin={{ top: 8, right: 16, bottom: 0, left: 0 }}>
        <CartesianGrid vertical={false} strokeDasharray='4 4' />
        <XAxis
          dataKey='date'
          tickMargin={8}
          minTickGap={24}
          tickFormatter={formatTick}
        />
        <YAxis axisLine={false} tickLine={false} width={56} tickMargin={4} />
        <ChartLegend
          content={<ChartLegendContent />}
          verticalAlign='bottom'
          height={36}
        />
        <ChartTooltip
          content={
            <ChartTooltipContent
              labelFormatter={(label) => formatTooltipDate(String(label))}
            />
          }
        />
        <Line
          dataKey='s0'
          type='monotone'
          stroke='var(--color-s0)'
          strokeWidth={2}
          dot={false}
          isAnimationActive={false}
        />
        <Line
          dataKey='s1'
          type='monotone'
          stroke='var(--color-s1)'
          strokeWidth={2}
          dot={false}
          isAnimationActive={false}
        />
      </LineChart>
    </ChartContainer>
  )
}
