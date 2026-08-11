// metapi-go/features/dashboard/sections/models — models section.
//
// Plan §5.5.1 models: ModelAnalysisPanel（模型可用性/延迟）+ 砍重复的
// Cost/Latency 卡片并入. Phase 3 wires three VChart charts:
//   - ModelCost donut        ← api.getModelCostDistribution(days, topN)
//   - Latency histogram bars ← api.getLatencyHistogram(days, bucketMs)
//   - Latency trend lines    ← api.getLatencyTrend(days)
// The legacy standalone Cost and Latency cards are folded in (not rendered
// as separate sections).

import { VChart } from '@visactor/react-vchart'
import { useQuery } from '@tanstack/react-query'
import { Inbox, TriangleAlert } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { useTheme } from '@/context/theme-provider'
import { api } from '@/lib/api'

import { ChartShell } from '../../components/chart-shell'
import { useChartColors, useThemeLabelColor } from '../../hooks/use-chart-colors'
import {
  VCHART_OPTION,
  buildLatencyHistogramSpec,
  buildLatencyTrendSpec,
  buildModelCostSpec,
} from '../../lib/chart-specs'
import type { ModelCostRow } from '../../types'

function ChartError({ message }: { message: string }) {
  return (
    <div className='flex h-full w-full flex-col items-center justify-center gap-1.5'>
      <TriangleAlert className='size-5 text-destructive/80' />
      <p className='text-destructive text-xs'>{message}</p>
    </div>
  )
}

function ChartEmpty({ message }: { message: string }) {
  return (
    <div className='flex h-full w-full flex-col items-center justify-center gap-1.5'>
      <Inbox className='size-5 text-muted-foreground/60' />
      <p className='text-muted-foreground text-xs'>{message}</p>
    </div>
  )
}

export function ModelsSection() {
  const { t } = useTranslation()
  const colors = useChartColors()
  const labelColor = useThemeLabelColor()
  const { resolvedTheme } = useTheme()

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
    [costQuery.data],
  )

  const histogramData = useMemo<Array<{ label: string; count: number }>>(
    () =>
      (histogramQuery.data?.buckets ?? []).map((bucket) => ({
        label: bucket.label,
        count: bucket.count,
      })),
    [histogramQuery.data],
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
        rows.push({ date: point.date, metric: metricAvg, latency: point.avgLatencyMs })
      }
      if (point.p95LatencyMs !== null && point.p95LatencyMs !== undefined) {
        rows.push({ date: point.date, metric: metricP95, latency: point.p95LatencyMs })
      }
    }
    return rows
  }, [trendQuery.data, metricAvg, metricP95])

  const costSpec = useMemo(
    () => buildModelCostSpec(colors, labelColor, costData),
    [colors, labelColor, costData],
  )
  const histogramSpec = useMemo(
    () => buildLatencyHistogramSpec(colors, histogramData),
    [colors, histogramData],
  )
  const trendSpec = useMemo(
    () => buildLatencyTrendSpec(colors, trendData),
    [colors, trendData],
  )

  const renderChart = (
    spec: Record<string, unknown>,
    suffix: string,
    isLoading: boolean,
    isError: boolean,
    dataLength: number,
    emptyKey: string,
  ) => {
    if (isLoading) return null
    if (isError) {
      return <ChartError message={t('dashboard.models.loadError')} />
    }
    if (dataLength === 0) {
      return <ChartEmpty message={t(emptyKey)} />
    }
    return (
      <VChart
        key={`models-${suffix}-${resolvedTheme}`}
        spec={spec as never}
        option={VCHART_OPTION}
        style={{ width: '100%', height: '100%' }}
      />
    )
  }

  return (
    <div className='grid grid-cols-1 gap-4 lg:grid-cols-2'>
      <ChartShell
        title={t('dashboard.models.costDistribution.title')}
        description={t('dashboard.models.costDistribution.description')}
        height={320}
        loading={costQuery.isLoading}
      >
        {renderChart(
          costSpec,
          'cost',
          costQuery.isLoading,
          costQuery.isError,
          costData.length,
          'dashboard.models.costDistribution.empty',
        )}
      </ChartShell>

      <ChartShell
        title={t('dashboard.models.latencyHistogram.title')}
        description={t('dashboard.models.latencyHistogram.description')}
        height={320}
        loading={histogramQuery.isLoading}
      >
        {renderChart(
          histogramSpec,
          'histogram',
          histogramQuery.isLoading,
          histogramQuery.isError,
          histogramData.length,
          'dashboard.models.latencyHistogram.empty',
        )}
      </ChartShell>

      <ChartShell
        title={t('dashboard.models.latencyTrend.title')}
        description={t('dashboard.models.latencyTrend.description')}
        height={300}
        className='lg:col-span-2'
        loading={trendQuery.isLoading}
      >
        {renderChart(
          trendSpec,
          'trend',
          trendQuery.isLoading,
          trendQuery.isError,
          trendData.length,
          'dashboard.models.latencyTrend.empty',
        )}
      </ChartShell>
    </div>
  )
}
