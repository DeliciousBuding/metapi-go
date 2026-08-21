// metapi-go/features/dashboard/components — lazy-loaded sparkline renderer.
//
// Round 3 audit (H-domain performance): the dashboard shipped ~332KB of
// recharts on first paint even when no chart was rendered, because
// stat-card.tsx imported it statically. This module is the only dashboard
// file that recharts can reach through the eager overview section; it is
// loaded via React.lazy + dynamic import() from StatCard and only mounts
// when a stat card actually has sparkline data, so the recharts chunk is
// fetched on demand instead of up front.

import { Area, AreaChart } from 'recharts'

import { ChartContainer, type ChartConfig } from '@/components/ui/chart'

export type SparklinePoint = {
  index: number
  value: number
}

type StatCardSparklineProps = {
  data: SparklinePoint[]
  config: ChartConfig
  accentClassName?: string
}

export default function StatCardSparkline({
  data,
  config,
  accentClassName,
}: StatCardSparklineProps) {
  return (
    <ChartContainer
      config={config}
      className='h-10 w-full'
      initialDimension={{ width: 200, height: 40 }}
    >
      <AreaChart data={data} margin={{ top: 4, right: 0, bottom: 0, left: 0 }}>
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
  )
}
