// metapi-go/features/dashboard/sections/models — models section.
//
// Plan §5.5.1 models: ModelAnalysisPanel（模型可用性/延迟）+ 砍重复的
// Cost/Latency 卡片并入. Phase 3 wires three recharts-based charts:
//   - ModelCost donut        ← api.getModelCostDistribution(days, topN)
//   - Latency histogram bars ← api.getLatencyHistogram(days, bucketMs)
//   - Latency trend lines    ← api.getLatencyTrend(days)
// The legacy standalone Cost and Latency cards are folded in (not rendered
// as separate sections).

import { useQuery } from '@tanstack/react-query'
import { Inbox, TriangleAlert } from 'lucide-react'
import { useMemo, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { api } from '@/lib/api'

import { ChartShell } from '../../components/chart-shell'
import {
  LatencyHistogramChart,
  LatencyTrendChart,
  ModelCostChart,
} from '../../components/charts'
import type { ModelCostRow } from '../../types'

function ChartError({ message }: { message: string }) {
  return (
    <div className='flex h-full w-full flex-col items-center justify-center gap-1.5'>
      <TriangleAlert className='text-destructive/80 size-5' />
      <p className='text-destructive text-xs'>{message}</p>
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

export function ModelsSection() {
  const { t } = useTranslation()

  const costQuery = useQuery({
    queryKey: ['dashboard-model-cost-distribution', 30, 8],
    queryFn: () => api.getModelCostDistribution(30, 8),
  })

  const histogramQuery = useQuery({
    queryKey: ['dashboard-latency-histogram', 7, 500],
    queryFn: () => api.getLatencyHistogram(7, 500),
  })

  const trendQuery = useQuery({
    queryKey: ['dashboard-latency-trend', 7],
    queryFn: () => api.getLatencyTrend(7),
  })

  const costData = useMemo<ModelCostRow[]>(
    () => costQuery.data?.items ?? [],
    [costQuery.data]
  )

  // All-zero cost (calls logged but no spend attributed in the 30d window)
  // leaves the donut with nothing to draw — surface a text summary instead.
  const costTotal = useMemo(
    () => costData.reduce((sum, row) => sum + row.cost, 0),
    [costData]
  )
  const costCalls = useMemo(
    () => costData.reduce((sum, row) => sum + row.calls, 0),
    [costData]
  )

  const histogramData = useMemo<Array<{ label: string; count: number }>>(
    () =>
      (histogramQuery.data?.buckets ?? []).map((bucket) => ({
        label: bucket.label,
        count: bucket.count,
      })),
    [histogramQuery.data]
  )

  const metricAvg = t('dashboard.models.latencyTrend.metricAvg')
  const metricP95 = t('dashboard.models.latencyTrend.metricP95')
  const trendData = useMemo<
    Array<{ date: string; metric: string; latency: number }>
  >(() => {
    const points = trendQuery.data?.points
    if (!points) return []
    const rows: Array<{ date: string; metric: string; latency: number }> = []
    for (const point of points) {
      if (point.avgLatencyMs !== null && point.avgLatencyMs !== undefined) {
        rows.push({
          date: point.date,
          metric: metricAvg,
          latency: point.avgLatencyMs,
        })
      }
      if (point.p95LatencyMs !== null && point.p95LatencyMs !== undefined) {
        rows.push({
          date: point.date,
          metric: metricP95,
          latency: point.p95LatencyMs,
        })
      }
    }
    return rows
  }, [trendQuery.data, metricAvg, metricP95])

  const costTooltipLabels = useMemo(
    () => ({
      cost: t('dashboard.models.costDistribution.tooltip.cost'),
      calls: t('dashboard.models.costDistribution.tooltip.calls'),
      tokens: t('dashboard.models.costDistribution.tooltip.tokens'),
      share: t('dashboard.models.costDistribution.tooltip.share'),
    }),
    [t]
  )

  const renderChartBody = (
    isLoading: boolean,
    isError: boolean,
    isEmpty: boolean,
    emptyKey: string,
    chart: ReactNode
  ): ReactNode => {
    if (isLoading) return null
    if (isError) {
      return <ChartError message={t('dashboard.models.loadError')} />
    }
    if (isEmpty) {
      return <ChartEmpty message={t(emptyKey)} />
    }
    return chart
  }

  return (
    <div className='grid grid-cols-1 gap-4 lg:grid-cols-2'>
      <ChartShell
        title={t('dashboard.models.costDistribution.title')}
        description={t('dashboard.models.costDistribution.description')}
        height={300}
        loading={costQuery.isLoading}
      >
        {renderChartBody(
          costQuery.isLoading,
          costQuery.isError,
          costData.length === 0,
          'dashboard.models.costDistribution.empty',
          costTotal === 0 ? (
            <ChartEmpty
              message={t('dashboard.models.costDistribution.zeroCost', {
                calls: costCalls,
              })}
            />
          ) : (
            <ModelCostChart data={costData} labels={costTooltipLabels} />
          )
        )}
      </ChartShell>

      <ChartShell
        title={t('dashboard.models.latencyHistogram.title')}
        description={t('dashboard.models.latencyHistogram.description')}
        height={300}
        loading={histogramQuery.isLoading}
      >
        {renderChartBody(
          histogramQuery.isLoading,
          histogramQuery.isError,
          histogramData.length === 0,
          'dashboard.models.latencyHistogram.empty',
          <LatencyHistogramChart data={histogramData} />
        )}
      </ChartShell>

      <ChartShell
        title={t('dashboard.models.latencyTrend.title')}
        description={t('dashboard.models.latencyTrend.description')}
        height={300}
        className='lg:col-span-2'
        loading={trendQuery.isLoading}
      >
        {renderChartBody(
          trendQuery.isLoading,
          trendQuery.isError,
          trendData.length === 0,
          'dashboard.models.latencyTrend.empty',
          <LatencyTrendChart
            data={trendData}
            avgLabel={metricAvg}
            p95Label={metricP95}
          />
        )}
      </ChartShell>
    </div>
  )
}
