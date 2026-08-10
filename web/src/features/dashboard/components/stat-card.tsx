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
import { Area, AreaChart } from 'recharts'

import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  ChartContainer,
  type ChartConfig,
} from '@/components/ui/chart'
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
  className?: string
}

const SPARK_CONFIG: ChartConfig = {
  value: {
    label: 'Trend',
    color: 'var(--chart-1)',
  },
}

export function StatCard({
  title,
  value,
  hint,
  spark,
  accentClassName,
  className,
}: StatCardProps) {
  const data = useMemo(
    () =>
      (spark ?? []).map((sample, index) => ({
        index,
        value: sample,
      })),
    [spark],
  )

  return (
    <Card className={cn('overflow-hidden', className)}>
      <CardHeader className='pb-2'>
        <CardTitle className='text-muted-foreground text-xs font-medium tracking-wide uppercase'>
          {title}
        </CardTitle>
      </CardHeader>
      <CardContent className='space-y-2'>
        <div className='flex items-end justify-between gap-2'>
          <span className='text-2xl font-semibold tabular-nums'>
            {value}
          </span>
          {hint ? (
            <span className='text-muted-foreground text-xs'>{hint}</span>
          ) : null}
        </div>
        {data.length > 1 ? (
          <ChartContainer
            config={SPARK_CONFIG}
            className='h-10 w-full'
            initialDimension={{ width: 200, height: 40 }}
          >
            <AreaChart data={data} margin={{ top: 4, right: 0, bottom: 0, left: 0 }}>
              <defs>
                <linearGradient id='stat-spark' x1='0' y1='0' x2='0' y2='1'>
                  <stop
                    offset='0%'
                    stopColor='var(--color-value)'
                    stopOpacity={0.4}
                  />
                  <stop
                    offset='100%'
                    stopColor='var(--color-value)'
                    stopOpacity={0}
                  />
                </linearGradient>
              </defs>
              <Area
                dataKey='value'
                stroke='var(--color-value)'
                strokeWidth={1.5}
                fill='url(#stat-spark)'
                isAnimationActive={false}
                className={accentClassName}
              />
            </AreaChart>
          </ChartContainer>
        ) : null}
      </CardContent>
    </Card>
  )
}
