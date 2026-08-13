// metapi-go/features/dashboard/components — overview metric stat card.
//
// Demonstrates the recharts / shadcn chart dual-track: simple sparklines use
// the shadcn ChartContainer (DOM-rendered recharts, which CAN consume CSS
// var() via ChartStyle's --color-${key} injection), while complex charts use
// VChart (canvas, needs useChartColors). This card renders a metric value +
// an optional tiny area sparkline so the overview reads at a glance.
//
// Phase 2: values + sparkline data are stubbed by the overview section. Phase
// 3 will feed real snapshot metrics (activeAccounts / sites / checkin success
// / proxy 24h) from api.getDashboardSnapshot.

import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { Area, AreaChart } from 'recharts'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { ChartContainer, type ChartConfig } from '@/components/ui/chart'
import { CountUp } from '@/components/ui/count-up'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'

type StatCardProps = {
  title: string
  value: string
  /** Optional helper line under the value (e.g. trend delta). */
  hint?: string
  /** Sparkline samples (numeric). When omitted, no sparkline renders. */
  spark?: number[]
  /** Tailwind class for the sparkline stroke (e.g. 'text-chart-1'). */
  accentClassName?: string
  /** Render skeleton placeholders while the metric is still loading. */
  loading?: boolean
  /** Numeric value to animate (CountUp). When set, overrides the static alue string. */
  valueNumber?: number
  /** Formatter applied to the animated numeric value. */
  valueFormat?: (value: number) => string
  className?: string
}

const SPARK_CONFIG_BASE: ChartConfig = {
  value: {
    color: 'var(--chart-1)',
  },
}

export function StatCard({
  title,
  value,
  hint,
  spark,
  accentClassName,
  loading = false,
  valueNumber,
  valueFormat,
  className,
}: StatCardProps) {
  const { t } = useTranslation()
  const sparkConfig: ChartConfig = useMemo(
    () => ({
      ...SPARK_CONFIG_BASE,
      value: {
        ...SPARK_CONFIG_BASE.value,
        label: t('dashboard.statCard.trendLabel'),
      },
    }),
    [t]
  )
  const data = useMemo(
    () =>
      (spark ?? []).map((sample, index) => ({
        index,
        value: sample,
      })),
    [spark]
  )

  return (
    <Card className={cn('overflow-hidden', className)}>
      <CardHeader className='pb-2'>
        <CardTitle className='text-muted-foreground text-xs font-medium tracking-wide uppercase'>
          {title}
        </CardTitle>
      </CardHeader>
      <CardContent className='space-y-2'>
        {loading ? (
          <div className='space-y-2'>
            <Skeleton className='h-7 w-20' />
            <Skeleton className='h-4 w-32' />
          </div>
        ) : (
          <>
            <div className='flex items-end justify-between gap-2'>
              {valueNumber !== undefined && Number.isFinite(valueNumber) ? (
                <CountUp
                  value={valueNumber}
                  format={valueFormat}
                  className='text-2xl font-semibold tracking-tight'
                />
              ) : (
                <span className='text-2xl font-semibold tracking-tight tabular-nums'>
                  {value}
                </span>
              )}
              {hint ? (
                <span className='text-muted-foreground text-xs tabular-nums'>
                  {hint}
                </span>
              ) : null}
            </div>
            {data.length > 1 ? (
              <ChartContainer
                config={sparkConfig}
                className='h-10 w-full'
                initialDimension={{ width: 200, height: 40 }}
              >
                <AreaChart
                  data={data}
                  margin={{ top: 4, right: 0, bottom: 0, left: 0 }}
                >
                  <Area
                    dataKey='value'
                    stroke='var(--color-value)'
                    strokeWidth={1.5}
                    fill='var(--color-value)'
                    fillOpacity={0.16}
                    isAnimationActive={false}
                    className={accentClassName}
                  />
                </AreaChart>
              </ChartContainer>
            ) : null}
          </>
        )}
      </CardContent>
    </Card>
  )
}
