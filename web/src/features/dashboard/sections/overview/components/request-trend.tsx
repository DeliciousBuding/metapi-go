import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Bar, CartesianGrid, ComposedChart, Line, XAxis, YAxis } from 'recharts'

import { QueryErrorBanner } from '@/components/common/query-error-banner'
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
} from '@/components/ui/chart'
import { Skeleton } from '@/components/ui/skeleton'
import type { OverviewPeriod } from '@/lib/api'
import { formatInt } from '@/lib/format'

import { overviewTrendRows } from '../overview-trend'
import { useOverviewReport } from '../use-overview-report'

export function RequestTrend({ period = '7d' }: { period?: OverviewPeriod }) {
  const { t } = useTranslation()
  const query = useOverviewReport(period)
  const points = query.data?.points ?? []
  const total = points.reduce((sum, point) => sum + point.requests, 0)
  const rows = overviewTrendRows(query.data)
  return (
    <Card className='h-full min-w-0'>
      <CardHeader>
        <CardTitle>{t('dashboard.operations.trend')}</CardTitle>
        <CardDescription>
          {t('dashboard.operations.trendWindow', {
            period: t(`dashboard.operations.period.${period}`),
          })}
        </CardDescription>
        <CardAction>
          <Link
            to='/dashboard/$section'
            params={{ section: 'models' }}
            className='text-primary text-xs hover:underline'
          >
            {t('dashboard.operations.performanceDetails')}
          </Link>
        </CardAction>
      </CardHeader>
      <CardContent className='space-y-3'>
        <QueryErrorBanner
          error={query.error}
          messageKey='dashboard.operations.loadError'
          onRetry={() => void query.refetch()}
        />
        {query.isLoading ? <Skeleton className='h-44 w-full' /> : null}
        {!query.isLoading && !query.isError && total === 0 ? (
          <p className='text-muted-foreground flex h-44 items-center justify-center text-sm'>
            {t('dashboard.operations.noRequests')}
          </p>
        ) : null}
        {!query.isLoading && !query.isError && total > 0 ? (
          <>
            <p className='text-muted-foreground text-xs'>
              {t('dashboard.operations.trendTotal', {
                count: total,
                value: formatInt(total),
              })}
            </p>
            <ChartContainer
              className='h-44 w-full'
              initialDimension={{ width: 640, height: 176 }}
              config={{
                requests: {
                  label: t('dashboard.operations.requests'),
                  color: 'var(--chart-1)',
                },
                successPercent: {
                  label: t('dashboard.operations.success'),
                  color: 'var(--chart-2)',
                },
              }}
            >
              <ComposedChart
                data={rows}
                margin={{ top: 8, right: 0, bottom: 0, left: 0 }}
              >
                <CartesianGrid vertical={false} />
                <XAxis
                  dataKey='date'
                  tickFormatter={(date: string) =>
                    period === '24h' ? date.slice(11, 16) : date.slice(5)
                  }
                  tickLine={false}
                  axisLine={false}
                  minTickGap={24}
                />
                <YAxis
                  yAxisId='requests'
                  tickLine={false}
                  axisLine={false}
                  width={40}
                  allowDecimals={false}
                />
                <YAxis
                  yAxisId='rate'
                  orientation='right'
                  tickLine={false}
                  axisLine={false}
                  width={42}
                  domain={[0, 100]}
                  tickFormatter={(value: number) => `${value}%`}
                />
                <ChartTooltip
                  content={
                    <ChartTooltipContent
                      formatter={(value, name) => (
                        <div className='flex w-full items-center justify-between gap-4'>
                          <span className='text-muted-foreground'>
                            {t(
                              `dashboard.operations.${name === 'successPercent' ? 'success' : 'requests'}`
                            )}
                          </span>
                          <span className='font-medium tabular-nums'>
                            {name === 'successPercent'
                              ? `${Number(value).toFixed(1)}%`
                              : formatInt(Number(value))}
                          </span>
                        </div>
                      )}
                    />
                  }
                />
                <Bar
                  yAxisId='requests'
                  dataKey='requests'
                  fill='var(--color-requests)'
                  radius={[3, 3, 0, 0]}
                  maxBarSize={36}
                  isAnimationActive={false}
                />
                <Line
                  yAxisId='rate'
                  dataKey='successPercent'
                  stroke='var(--color-successPercent)'
                  strokeWidth={2}
                  dot={{ r: 3 }}
                  connectNulls={false}
                  isAnimationActive={false}
                />
              </ComposedChart>
            </ChartContainer>
            <div className='text-muted-foreground flex flex-wrap gap-4 text-xs'>
              <span className='flex items-center gap-1.5'>
                <span className='bg-chart-1 size-2 rounded-sm' />
                {t('dashboard.operations.requests')}
              </span>
              <span className='flex items-center gap-1.5'>
                <span className='bg-chart-2 h-0.5 w-3' />
                {t('dashboard.operations.success')}
              </span>
            </div>
          </>
        ) : null}
      </CardContent>
    </Card>
  )
}
