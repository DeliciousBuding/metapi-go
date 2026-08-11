// metapi-go/features/dashboard/sections/models — models section.
//
// Plan §5.5.1 models: ModelAnalysisPanel（模型可用性/延迟）+ 砍重复的
// Cost/Latency 卡片并入. The legacy ModelAnalysisPanel rendered 4 pill tabs
// (spend/trend/calls/rank) with a horizontal bar / gradient area / donut pie /
// rank table. Here we wire the cost-distribution donut (VChart) and stub the
// availability / latency table — the duplicate standalone Cost and Latency
// cards are folded in (not rendered as separate sections).
//
// Phase 2: cost donut fed an empty array; availability table is a stub.
// Phase 3: api.getModelCostDistribution(days, topN) → ModelCostRow[];
//   api.getLatencyTrend(days) for the latency table; api.getDashboardInsights()
//   for the model-analysis summary.

import { useMemo } from 'react'
import { Cpu } from 'lucide-react'
import { VChart } from '@visactor/react-vchart'

import { useTranslation } from 'react-i18next'

import { useTheme } from '@/context/theme-provider'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'

import { ChartShell } from '../../components/chart-shell'
import { useChartColors, useThemeLabelColor } from '../../hooks/use-chart-colors'
import { VCHART_OPTION, buildModelCostSpec } from '../../lib/chart-specs'
import type { ModelCostRow } from '../../types'

// TODO phase 3: api.getModelCostDistribution(days, topN) → ModelCostRow[]
const MODEL_COST_DATA: ModelCostRow[] = []

export function ModelsSection() {
  const { t } = useTranslation()
  const colors = useChartColors()
  const labelColor = useThemeLabelColor()
  const { resolvedTheme } = useTheme()

  const modelCostSpec = useMemo(
    () => buildModelCostSpec(colors, labelColor, MODEL_COST_DATA),
    [colors, labelColor],
  )

  return (
    <div className='grid grid-cols-1 gap-4 lg:grid-cols-2'>
      <ChartShell
        title={t('dashboard.models.costDistribution.title')}
        description={t('dashboard.models.costDistribution.description')}
        height={320}
      >
        <VChart
          key={`models-cost-${resolvedTheme}`}
          spec={modelCostSpec as never}
          option={VCHART_OPTION}
          style={{ width: '100%', height: '100%' }}
        />
      </ChartShell>

      <Card>
        <CardHeader>
          <CardTitle className='flex items-center gap-2 text-sm font-medium'>
            <Cpu className='size-4' />
            {t('dashboard.models.availability.title')}
          </CardTitle>
          <CardDescription className='text-xs'>
            {t('dashboard.models.availability.description')}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className='flex min-h-64 flex-col items-center justify-center gap-2 rounded-lg border border-dashed py-8 text-center'>
            <p className='text-muted-foreground text-sm'>
              {t('dashboard.models.availability.placeholder')}
            </p>
            <code className='text-muted-foreground/70 text-xs'>
              api.getDashboardInsights() · api.getLatencyTrend()
            </code>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
