// metapi-go/features/dashboard/components — recharts-based dashboard charts.
//
// Replaces the legacy @visactor/react-vchart charts (income/outcome bars,
// site trend lines, site / model distribution donuts, latency histogram and
// latency trend lines) with recharts primitives wrapped in the shared shadcn
// ChartContainer. recharts renders to SVG, so it resolves CSS var() theme
// tokens directly — no canvas colour extraction (useChartColors) and no
// theme-flip remount are needed; the palette comes from --chart-1..5, the same
// tokens the stat-card sparkline already uses. Tooltip / legend styling comes
// from ChartTooltipContent / ChartLegendContent so the look matches stat-card.

import { useMemo } from 'react'
import {
  Bar,
  BarChart,
  CartesianGrid,
  Cell,
  Line,
  LineChart,
  Pie,
  PieChart,
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

import type {
  IncomeOutcomePoint,
  ModelCostRow,
  SiteDistributionSlice,
  SiteTrendPoint,
} from '../types'

/** Number of ordinal colours exposed by the theme (--chart-1..5). */
const PALETTE_SIZE = 5

/** CSS var() reference for the i-th ordinal chart colour (cycles 1..5). */
function chartColor(index: number): string {
  return `var(--chart-${(index % PALETTE_SIZE) + 1})`
}

/** Safe, stable dataKey for the i-th dynamic series (keeps CSS-var names valid). */
function seriesKey(index: number): string {
  return `s${index}`
}

/** Adaptive currency formatting — mirrors the legacy VChart tooltip format. */
function formatCurrency(value: number): string {
  if (value >= 1000) return `$${value.toFixed(2)}`
  if (value >= 1) return `$${value.toFixed(3)}`
  return `$${value.toFixed(6)}`
}

/** Percent-of-total string for the donut tooltip share row. */
function percentOf(value: number, total: number): string {
  return `${total > 0 ? ((value / total) * 100).toFixed(1) : '0.0'}%`
}

// ---------------------------------------------------------------------------
// Long → wide data pivots. recharts categorical charts take one row per X
// category with one field per series, while the API / section layer emits
// long-format rows (one row per series per category). These pivots reshape
// that long data into the wide rows recharts expects, without touching the
// fetching or business logic above.
// ---------------------------------------------------------------------------

type IncomeOutcomeRow = { day: string; income: number; outcome: number }

function pivotIncomeOutcome(data: IncomeOutcomePoint[]): IncomeOutcomeRow[] {
  const byDay = new Map<string, IncomeOutcomeRow>()
  for (const point of data) {
    let row = byDay.get(point.day)
    if (!row) {
      row = { day: point.day, income: 0, outcome: 0 }
      byDay.set(point.day, row)
    }
    if (point.type === 'income') {
      row.income = point.value
    } else if (point.type === 'outcome') {
      row.outcome = point.value
    }
  }
  return [...byDay.values()]
}

function pivotSiteTrend(data: SiteTrendPoint[]): {
  rows: Array<Record<string, string | number>>
  sites: string[]
} {
  const siteIndex = new Map<string, number>()
  for (const point of data) {
    if (!siteIndex.has(point.site)) {
      siteIndex.set(point.site, siteIndex.size)
    }
  }
  const sites = [...siteIndex.keys()]
  const byDate = new Map<string, Record<string, string | number>>()
  for (const point of data) {
    let row = byDate.get(point.date)
    if (!row) {
      row = { date: point.date }
      byDate.set(point.date, row)
    }
    row[seriesKey(siteIndex.get(point.site) ?? 0)] = point.spend
  }
  return { rows: [...byDate.values()], sites }
}

type LatencyTrendRow = { date: string; s0?: number; s1?: number }

function pivotLatencyTrend(
  data: Array<{ date: string; metric: string; latency: number }>,
  avgLabel: string,
  p95Label: string
): LatencyTrendRow[] {
  const byDate = new Map<string, LatencyTrendRow>()
  for (const point of data) {
    let row = byDate.get(point.date)
    if (!row) {
      row = { date: point.date }
      byDate.set(point.date, row)
    }
    if (point.metric === avgLabel) {
      row.s0 = point.latency
    } else if (point.metric === p95Label) {
      row.s1 = point.latency
    }
  }
  return [...byDate.values()]
}

// ---------------------------------------------------------------------------
// Donut tooltip + legend. The donut charts carry extra per-slice fields
// (accountCount / totalSpend / calls / tokens / share) that the default
// recharts tooltip can't surface, so a small bespoke tooltip renders the
// multi-row content. The bespoke legend reads each slice's name + fill
// directly (no ChartConfig key plumbing, which keeps it robust against slice
// names containing spaces / special characters).
// ---------------------------------------------------------------------------

type DonutTooltipRender = (
  datum: Record<string, unknown>,
  value: number
) => { title: string; rows: Array<{ label: string; value: string }> }

type DonutTooltipPayload = {
  payload?: Record<string, unknown>
  value?: number | string
}

function DonutTooltip({
  active,
  payload,
  render,
}: {
  active?: boolean
  payload?: DonutTooltipPayload[]
  render: DonutTooltipRender
}) {
  if (!active || !payload?.length) {
    return null
  }
  const item = payload[0]
  const datum = item.payload ?? {}
  const rawValue = item.value
  const value = typeof rawValue === 'number' ? rawValue : Number(rawValue ?? 0)
  const { title, rows } = render(datum, value)
  return (
    <div className='border-border/50 bg-background grid min-w-32 gap-1.5 rounded-lg border px-2.5 py-1.5 text-xs shadow-xl'>
      <div className='font-medium'>{title}</div>
      <div className='grid gap-1.5'>
        {rows.map((row) => (
          <div
            key={row.label}
            className='flex justify-between gap-3 leading-none'
          >
            <span className='text-muted-foreground'>{row.label}</span>
            <span className='font-mono font-medium tabular-nums'>
              {row.value}
            </span>
          </div>
        ))}
      </div>
    </div>
  )
}

type DonutLegendPayload = {
  value?: string | number
  name?: string | number
  color?: string
  type?: string
}

function DonutLegend({ payload }: { payload?: DonutLegendPayload[] }) {
  if (!payload?.length) {
    return null
  }
  const items = payload.filter((item) => item.type !== 'none')
  return (
    <div className='flex flex-wrap items-center justify-center gap-x-4 gap-y-1 pt-3 text-xs'>
      {items.map((item, index) => (
        <div
          key={String(item.value ?? item.name ?? index)}
          className='flex items-center gap-1.5'
        >
          <span
            className='h-2 w-2 shrink-0 rounded-sm'
            style={{ backgroundColor: item.color }}
          />
          <span className='text-muted-foreground'>
            {item.value ?? item.name}
          </span>
        </div>
      ))}
    </div>
  )
}

// ---------------------------------------------------------------------------
// Chart components.
// ---------------------------------------------------------------------------

const INCOME_OUTCOME_CONFIG: ChartConfig = {
  income: { label: 'Income', color: chartColor(0) },
  outcome: { label: 'Outcome', color: chartColor(1) },
}

/** Daily balance inflow vs spend — grouped bar chart (30d). */
export function IncomeOutcomeChart({ data }: { data: IncomeOutcomePoint[] }) {
  const rows = useMemo(() => pivotIncomeOutcome(data), [data])
  return (
    <ChartContainer config={INCOME_OUTCOME_CONFIG} className='h-full w-full'>
      <BarChart data={rows} margin={{ top: 8, right: 16, bottom: 0, left: 0 }}>
        <CartesianGrid vertical={false} strokeDasharray='4 4' />
        <XAxis dataKey='day' tickMargin={8} minTickGap={24} />
        <YAxis axisLine={false} tickLine={false} width={48} tickMargin={4} />
        <ChartLegend
          content={<ChartLegendContent />}
          verticalAlign='top'
          height={36}
        />
        <ChartTooltip content={<ChartTooltipContent />} />
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
  const { rows, sites } = useMemo(() => pivotSiteTrend(data), [data])
  const config = useMemo<ChartConfig>(() => {
    const cfg: ChartConfig = {}
    sites.forEach((site, index) => {
      cfg[seriesKey(index)] = { label: site, color: chartColor(index) }
    })
    return cfg
  }, [sites])
  return (
    <ChartContainer config={config} className='h-full w-full'>
      <LineChart data={rows} margin={{ top: 8, right: 16, bottom: 0, left: 0 }}>
        <CartesianGrid vertical={false} strokeDasharray='4 4' />
        <XAxis dataKey='date' tickMargin={8} minTickGap={24} />
        <YAxis axisLine={false} tickLine={false} width={48} tickMargin={4} />
        <ChartLegend
          content={<ChartLegendContent />}
          verticalAlign='bottom'
          height={36}
        />
        <ChartTooltip content={<ChartTooltipContent />} />
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

/** Balance share by site — donut. */
export function SiteDistributionChart({
  data,
  labels,
}: {
  data: SiteDistributionSlice[]
  labels: { balance: string; accounts: string; share: string }
}) {
  const pieData = useMemo(
    () =>
      data.map((slice, index) => ({
        siteName: slice.siteName,
        value: slice.totalBalance,
        accountCount: slice.accountCount,
        totalSpend: slice.totalSpend,
        key: seriesKey(index),
      })),
    [data]
  )
  const total = useMemo(
    () => pieData.reduce((sum, slice) => sum + slice.value, 0),
    [pieData]
  )
  const config = useMemo<ChartConfig>(() => {
    const cfg: ChartConfig = {}
    pieData.forEach((slice, index) => {
      cfg[slice.key] = { label: slice.siteName, color: chartColor(index) }
    })
    return cfg
  }, [pieData])
  const render: DonutTooltipRender = (datum, value) => ({
    title: String(datum.siteName ?? '-'),
    rows: [
      { label: labels.balance, value: formatCurrency(value) },
      { label: labels.accounts, value: String(datum.accountCount ?? 0) },
      { label: labels.share, value: percentOf(value, total) },
    ],
  })
  return (
    <ChartContainer config={config} className='h-full w-full'>
      <PieChart>
        <ChartLegend
          content={<DonutLegend />}
          verticalAlign='bottom'
          height={36}
        />
        <ChartTooltip content={<DonutTooltip render={render} />} />
        <Pie
          data={pieData}
          dataKey='value'
          nameKey='siteName'
          innerRadius='55%'
          outerRadius='80%'
          paddingAngle={2}
          stroke='var(--border)'
          strokeWidth={1}
          isAnimationActive={false}
        >
          {pieData.map((slice) => (
            <Cell key={slice.key} fill={`var(--color-${slice.key})`} />
          ))}
        </Pie>
      </PieChart>
    </ChartContainer>
  )
}

const LATENCY_HISTOGRAM_CONFIG: ChartConfig = {
  count: { label: '', color: chartColor(0) },
}

/** Request count by latency bucket — single-series bar chart (7d). */
export function LatencyHistogramChart({
  data,
}: {
  data: Array<{ label: string; count: number }>
}) {
  return (
    <ChartContainer config={LATENCY_HISTOGRAM_CONFIG} className='h-full w-full'>
      <BarChart data={data} margin={{ top: 8, right: 16, bottom: 0, left: 0 }}>
        <CartesianGrid vertical={false} strokeDasharray='4 4' />
        <XAxis dataKey='label' tickMargin={8} minTickGap={12} />
        <YAxis
          axisLine={false}
          tickLine={false}
          width={48}
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
  const rows = useMemo(
    () => pivotLatencyTrend(data, avgLabel, p95Label),
    [data, avgLabel, p95Label]
  )
  const config = useMemo<ChartConfig>(
    () => ({
      s0: { label: avgLabel, color: chartColor(0) },
      s1: { label: p95Label, color: chartColor(1) },
    }),
    [avgLabel, p95Label]
  )
  return (
    <ChartContainer config={config} className='h-full w-full'>
      <LineChart data={rows} margin={{ top: 8, right: 16, bottom: 0, left: 0 }}>
        <CartesianGrid vertical={false} strokeDasharray='4 4' />
        <XAxis dataKey='date' tickMargin={8} minTickGap={24} />
        <YAxis axisLine={false} tickLine={false} width={48} tickMargin={4} />
        <ChartLegend
          content={<ChartLegendContent />}
          verticalAlign='bottom'
          height={36}
        />
        <ChartTooltip content={<ChartTooltipContent />} />
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

/** Spend share by model — donut (30d). */
export function ModelCostChart({
  data,
  labels,
}: {
  data: ModelCostRow[]
  labels: { cost: string; calls: string; tokens: string; share: string }
}) {
  const pieData = useMemo(
    () =>
      data.map((row, index) => ({
        model: row.label || row.model,
        value: row.cost,
        calls: row.calls,
        tokens: row.tokens,
        key: seriesKey(index),
      })),
    [data]
  )
  const total = useMemo(
    () => pieData.reduce((sum, slice) => sum + slice.value, 0),
    [pieData]
  )
  const config = useMemo<ChartConfig>(() => {
    const cfg: ChartConfig = {}
    pieData.forEach((slice, index) => {
      cfg[slice.key] = { label: slice.model, color: chartColor(index) }
    })
    return cfg
  }, [pieData])
  const render: DonutTooltipRender = (datum, value) => ({
    title: String(datum.model ?? '-'),
    rows: [
      { label: labels.cost, value: formatCurrency(value) },
      { label: labels.calls, value: String(datum.calls ?? 0) },
      {
        label: labels.tokens,
        value: Number(datum.tokens ?? 0).toLocaleString(),
      },
      { label: labels.share, value: percentOf(value, total) },
    ],
  })
  return (
    <ChartContainer config={config} className='h-full w-full'>
      <PieChart>
        <ChartLegend
          content={<DonutLegend />}
          verticalAlign='bottom'
          height={36}
        />
        <ChartTooltip content={<DonutTooltip render={render} />} />
        <Pie
          data={pieData}
          dataKey='value'
          nameKey='model'
          innerRadius='62%'
          outerRadius='85%'
          paddingAngle={2}
          stroke='var(--border)'
          strokeWidth={1}
          isAnimationActive={false}
        >
          {pieData.map((slice) => (
            <Cell key={slice.key} fill={`var(--color-${slice.key})`} />
          ))}
        </Pie>
      </PieChart>
    </ChartContainer>
  )
}
