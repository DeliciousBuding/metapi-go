// metapi-go/features/observability/sections/flow — three-column traffic flow
// explorer (Downstream Key → Requested Model → Upstream Site/Channel) powered
// by GET /api/stats/flow. Uses a crisp CSS Grid + responsive SVG Bézier curves
// with solid OKLCH chart tokens (no gradients), full keyboard selection,
// deep-link drill-downs into /proxy-logs and /channels, and an sr-only data
// table for assistive technologies.

import { GitFork, X } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
} from '@/components/ui/empty'
import { KpiValue } from '@/components/ui/kpi-value'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { toBcp47 } from '@/i18n/languages'
import type { OverviewPeriod } from '@/lib/api'
import { formatCurrency, formatInt, formatRatio } from '@/lib/format'

import { useTrafficFlow } from '../../api'
import { ObservabilityErrorBanner } from '../../components/observability-error-banner'
import { FlowDiagram } from './flow-diagram'
import { FlowEdgeTable } from './flow-edge-table'
import {
  buildFlowGraph,
  resolveSelectionHighlight,
  type FlowMetric,
} from './flow-graph'
import { FlowSelectionDetails } from './flow-selection-details'

const PERIODS: readonly OverviewPeriod[] = ['24h', '7d', '30d', 'all']
const METRICS: readonly FlowMetric[] = ['requests', 'cost', 'tokens']

export function FlowSection() {
  const { t, i18n } = useTranslation()
  const locale = toBcp47(i18n.language || 'en')

  const [period, setPeriod] = useState<OverviewPeriod>('24h')
  const [metric, setMetric] = useState<FlowMetric>('requests')
  const [failedOnly, setFailedOnly] = useState(false)
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null)

  const flowQuery = useTrafficFlow({ period, topN: 20 })
  const data = flowQuery.data

  const filteredEdges = useMemo(() => {
    const edges = data?.edges ?? []
    return failedOnly ? edges.filter((edge) => edge.failedCount > 0) : edges
  }, [data?.edges, failedOnly])

  const graph = useMemo(
    () =>
      buildFlowGraph(filteredEdges, metric, {
        unassignedKey: t('observability.flow.unassignedKey'),
        unknownModel: t('observability.flow.unknownModel'),
        unassignedUpstream: t('observability.flow.unassignedUpstream'),
        channelPrefix: t('observability.flow.channelPrefix'),
        sitePrefix: '#',
      }),
    [filteredEdges, metric, t]
  )

  const allNodes = useMemo(
    () => [...graph.keys, ...graph.models, ...graph.upstreams],
    [graph.keys, graph.models, graph.upstreams]
  )

  const selectedNode = useMemo(
    () =>
      selectedNodeId
        ? (allNodes.find((node) => node.id === selectedNodeId) ?? null)
        : null,
    [allNodes, selectedNodeId]
  )

  // Clear stale selection if filters/period removed the selected node.
  useEffect(() => {
    if (selectedNodeId && !selectedNode) {
      setSelectedNodeId(null)
    }
  }, [selectedNode, selectedNodeId])

  const highlight = useMemo(
    () => resolveSelectionHighlight(graph, selectedNode?.id ?? null),
    [graph, selectedNode?.id]
  )
  const toggleNode = useCallback((nodeId: string) => {
    setSelectedNodeId((prev) => (prev === nodeId ? null : nodeId))
  }, [])

  const totalRequests = data?.totalRequests ?? 0
  const totalFailed = data?.totalFailed ?? 0
  const totalTokens = data?.totalTokens ?? 0
  const totalCost = data?.totalCost ?? 0
  const successCount = Math.max(0, totalRequests - totalFailed)

  const summaryStrip = (
    <div className='bg-card grid grid-cols-2 overflow-hidden rounded-xl border lg:grid-cols-4'>
      <div className='flex flex-col gap-1 border-r border-b p-3.5 lg:border-b-0'>
        <span className='text-muted-foreground text-xs'>
          {t('observability.flow.summary.requests')}
        </span>
        <KpiValue size='lg'>{formatInt(totalRequests, locale)}</KpiValue>
      </div>
      <div className='flex flex-col gap-1 border-b p-3.5 lg:border-r lg:border-b-0'>
        <span className='text-muted-foreground text-xs'>
          {t('observability.flow.summary.failed')}
        </span>
        <div className='flex items-baseline gap-2'>
          <KpiValue
            size='lg'
            className={
              totalFailed > 0 ? 'text-destructive-soft-fg' : 'text-foreground'
            }
          >
            {formatInt(totalFailed, locale)}
          </KpiValue>
          <span className='text-muted-foreground text-xs tabular-nums'>
            {totalRequests > 0
              ? t('observability.flow.summary.successRate', {
                  rate: formatRatio(successCount, totalRequests),
                })
              : '—'}
          </span>
        </div>
      </div>
      <div className='flex flex-col gap-1 border-r p-3.5'>
        <span className='text-muted-foreground text-xs'>
          {t('observability.flow.summary.tokens')}
        </span>
        <KpiValue size='lg'>{formatInt(totalTokens, locale)}</KpiValue>
      </div>
      <div className='flex flex-col gap-1 p-3.5'>
        <span className='text-muted-foreground text-xs'>
          {t('observability.flow.summary.cost')}
        </span>
        <KpiValue size='lg'>
          {formatCurrency(totalCost, { fractionDigits: 4, locale })}
        </KpiValue>
      </div>
    </div>
  )

  return (
    <div className='flex flex-col gap-4'>
      {/* Controls bar: period + metric + failed-only switch */}
      <div className='flex flex-wrap items-center justify-between gap-3'>
        <div className='flex flex-wrap items-center gap-2'>
          <div
            role='group'
            aria-label={t('observability.flow.periodLabel')}
            className='bg-muted inline-flex items-center gap-1 rounded-lg p-1'
          >
            {PERIODS.map((value) => (
              <Button
                key={value}
                type='button'
                size='sm'
                variant={period === value ? 'secondary' : 'ghost'}
                aria-pressed={period === value}
                onClick={() => setPeriod(value)}
                className='h-7 px-2.5 text-xs'
              >
                {t(`dashboard.operations.period.${value}`)}
              </Button>
            ))}
          </div>

          <div
            role='group'
            aria-label={t('observability.flow.metricLabel')}
            className='bg-muted inline-flex items-center gap-1 rounded-lg p-1'
          >
            {METRICS.map((value) => (
              <Button
                key={value}
                type='button'
                size='sm'
                variant={metric === value ? 'secondary' : 'ghost'}
                aria-pressed={metric === value}
                onClick={() => setMetric(value)}
                className='h-7 px-2.5 text-xs'
              >
                {t(`observability.flow.metric.${value}`)}
              </Button>
            ))}
          </div>
        </div>

        <label className='flex cursor-pointer items-center gap-2 text-xs'>
          <Switch checked={failedOnly} onCheckedChange={setFailedOnly} />
          <span className='text-muted-foreground font-medium'>
            {t('observability.flow.failedOnly')}
          </span>
        </label>
      </div>

      {flowQuery.isError && !flowQuery.isLoading ? (
        <ObservabilityErrorBanner
          messageKey='observability.flow.loadFailed'
          isRetrying={flowQuery.isFetching && !flowQuery.isLoading}
          onRetry={() => void flowQuery.refetch()}
        />
      ) : (
        <>
          {flowQuery.isLoading ? (
            <div className='grid grid-cols-2 gap-3 lg:grid-cols-4'>
              {[0, 1, 2, 3].map((i) => (
                <Skeleton key={i} className='h-20 rounded-xl' />
              ))}
            </div>
          ) : (
            summaryStrip
          )}

          <Card>
            <CardHeader className='flex flex-row flex-wrap items-start justify-between gap-2'>
              <div>
                <CardTitle className='flex items-center gap-2'>
                  <GitFork className='size-4' aria-hidden='true' />
                  {t('observability.flow.cardTitle')}
                </CardTitle>
                <CardDescription className='text-xs'>
                  {t('observability.flow.cardDescription')}
                </CardDescription>
              </div>
              {selectedNode && (
                <Button
                  type='button'
                  size='xs'
                  variant='ghost'
                  onClick={() => setSelectedNodeId(null)}
                >
                  <X className='size-3.5' aria-hidden='true' />
                  {t('observability.flow.clearSelection')}
                </Button>
              )}
            </CardHeader>

            <CardContent className='space-y-4'>
              {flowQuery.isLoading && (
                <Skeleton className='h-72 w-full rounded-lg' />
              )}
              {!flowQuery.isLoading && filteredEdges.length === 0 && (
                <Empty className='min-h-40 border'>
                  <EmptyHeader>
                    <EmptyMedia variant='icon'>
                      <GitFork />
                    </EmptyMedia>
                    <EmptyDescription>
                      {failedOnly && (data?.edges?.length ?? 0) > 0
                        ? t('observability.flow.emptyFailedFilter')
                        : t('observability.flow.empty')}
                    </EmptyDescription>
                  </EmptyHeader>
                </Empty>
              )}
              {!flowQuery.isLoading && filteredEdges.length > 0 && (
                <>
                  <FlowDiagram
                    graph={graph}
                    highlight={highlight}
                    selectedNodeId={selectedNode?.id ?? null}
                    metric={metric}
                    locale={locale}
                    onSelect={toggleNode}
                    failedOnly={failedOnly}
                  />
                  <FlowSelectionDetails
                    selectedNode={selectedNode}
                    matchingEdgeCount={highlight.matchingEdgeCount}
                    timeWindow={data?.window}
                    failedOnly={failedOnly}
                    locale={locale}
                  />
                  <FlowEdgeTable edges={filteredEdges} locale={locale} />
                </>
              )}
            </CardContent>
          </Card>
        </>
      )}
    </div>
  )
}
