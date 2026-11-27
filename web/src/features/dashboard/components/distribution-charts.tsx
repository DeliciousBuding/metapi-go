import { useMemo, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Cell, Pie, PieChart } from 'recharts'

import { ModelPill } from '@/components/common/model-pill'
import {
  ChartContainer,
  ChartLegend,
  ChartTooltip,
  type ChartConfig,
} from '@/components/ui/chart'
import { EM_DASH } from '@/lib/format'

import type { ModelCostRow, SiteDistributionSlice } from '../types'
import { chartColor, seriesKey } from './chart-series'
import { formatChartCurrency } from './charts-currency'

function percentOf(value: number, total: number): string {
  return `${total > 0 ? ((value / total) * 100).toFixed(1) : '0.0'}%`
}

type DonutTooltipRender = (
  datum: Record<string, unknown>,
  value: number
) => { title: ReactNode; rows: Array<{ label: string; value: string }> }

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
            <span className='font-medium tabular-nums'>{row.value}</span>
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

function DonutLegend({
  payload,
  renderLabel,
}: {
  payload?: DonutLegendPayload[]
  renderLabel?: (name: string) => ReactNode
}) {
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
            {renderLabel
              ? renderLabel(String(item.value ?? item.name ?? ''))
              : (item.value ?? item.name)}
          </span>
        </div>
      ))}
    </div>
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
  const { t } = useTranslation()
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
    title: String(datum.siteName ?? EM_DASH),
    rows: [
      { label: labels.balance, value: formatChartCurrency(value) },
      { label: labels.accounts, value: String(datum.accountCount ?? 0) },
      { label: labels.share, value: percentOf(value, total) },
    ],
  })
  return (
    <div className='flex h-full min-h-0 flex-col gap-3'>
      <ChartContainer
        config={config}
        className='h-[260px] w-full shrink-0'
        aria-label={t('dashboard.charts.siteDistributionTitle')}
      >
        <PieChart>
          <ChartTooltip content={<DonutTooltip render={render} />} />
          <Pie
            data={pieData}
            dataKey='value'
            nameKey='siteName'
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
      <ul
        className='grid min-h-0 flex-1 grid-cols-1 content-start gap-x-4 gap-y-2 overflow-y-auto pr-2 text-xs sm:grid-cols-2 lg:grid-cols-3'
        tabIndex={0}
        aria-label={t('dashboard.traffic.siteDistribution.title')}
      >
        {pieData.map((slice, index) => (
          <li key={slice.key} className='flex min-w-0 items-center gap-2'>
            <span
              className='h-2.5 w-2.5 shrink-0 rounded-sm'
              style={{ backgroundColor: chartColor(index) }}
              aria-hidden='true'
            />
            <span
              className='text-muted-foreground min-w-0 flex-1 truncate'
              title={slice.siteName}
            >
              {slice.siteName}
            </span>
            <span className='font-medium tabular-nums'>
              {percentOf(slice.value, total)}
            </span>
          </li>
        ))}
      </ul>
    </div>
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
  const { t } = useTranslation()
  const pieData = useMemo(
    () =>
      data.map((row, index) => ({
        model: row.label || row.model,
        rawModel: row.model,
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
    title:
      datum.rawModel && datum.rawModel !== 'other' ? (
        <ModelPill
          model={String(datum.rawModel)}
          label={String(datum.model)}
          variant='inline'
        />
      ) : (
        String(datum.model ?? EM_DASH)
      ),
    rows: [
      { label: labels.cost, value: formatChartCurrency(value) },
      { label: labels.calls, value: String(datum.calls ?? 0) },
      {
        label: labels.tokens,
        value: Number(datum.tokens ?? 0).toLocaleString(),
      },
      { label: labels.share, value: percentOf(value, total) },
    ],
  })
  return (
    <ChartContainer
      config={config}
      className='h-full w-full'
      aria-label={t('dashboard.charts.modelCostTitle')}
    >
      <PieChart>
        <ChartLegend
          content={
            <DonutLegend
              renderLabel={(name) => {
                const row = data.find(
                  (item) => (item.label || item.model) === name
                )
                return row?.model && row.model !== 'other' ? (
                  <ModelPill model={row.model} label={name} variant='inline' />
                ) : (
                  name
                )
              }}
            />
          }
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
