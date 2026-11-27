// metapi-go/features/observability/sections/flow — three-column traffic flow
// explorer (Downstream Key → Requested Model → Upstream Site/Channel) powered
// by GET /api/stats/flow. Uses a crisp CSS Grid + responsive SVG Bézier curves
// with solid OKLCH chart tokens (no gradients), full keyboard selection,
// deep-link drill-downs into /proxy-logs and /channels, and an sr-only data
// table for assistive technologies.

import { Link } from '@tanstack/react-router'
import {
  AlertTriangle,
  ArrowUpRight,
  GitFork,
  KeyRound,
  Layers,
  Server,
  X,
} from 'lucide-react'
import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from 'react'
import { useTranslation } from 'react-i18next'

import { ModelPill } from '@/components/common/model-pill'
import { Badge } from '@/components/ui/badge'
import { Button, buttonVariants } from '@/components/ui/button'
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
import { cn } from '@/lib/utils'

import { useTrafficFlow } from '../../api'
import { ObservabilityErrorBanner } from '../../components/observability-error-banner'
import {
  buildFlowGraph,
  resolveSelectionHighlight,
  type FlowMetric,
  type FlowNode,
} from './flow-graph'

const PERIODS: readonly OverviewPeriod[] = ['24h', '7d', '30d', 'all']
const METRICS: readonly FlowMetric[] = ['requests', 'cost', 'tokens']

type LineGeometry = {
  id: string
  path: string
  strokeWidth: number
  colorVar: string
  hasFailure: boolean
}

function formatMetricValue(
  node: Pick<FlowNode, 'requests' | 'totalCost' | 'totalTokens'>,
  metric: FlowMetric,
  locale: string
): string {
  if (metric === 'cost') {
    return formatCurrency(node.totalCost, { fractionDigits: 4, locale })
  }
  if (metric === 'tokens') {
    return formatInt(node.totalTokens, locale)
  }
  return formatInt(node.requests, locale)
}

function computeLinkThickness(value: number, maxValue: number): number {
  if (maxValue <= 0 || value <= 0) return 1.5
  const ratio = Math.min(1, value / maxValue)
  return Number((1.5 + ratio * 6.5).toFixed(2))
}

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

  const canvasRef = useRef<HTMLDivElement | null>(null)
  const nodeRefs = useRef(new Map<string, HTMLButtonElement>())
  const [svgState, setSvgState] = useState<{
    width: number
    height: number
    lines: LineGeometry[]
  }>({ width: 0, height: 0, lines: [] })

  const registerNodeRef = useCallback(
    (nodeId: string, element: HTMLButtonElement | null) => {
      if (element) {
        nodeRefs.current.set(nodeId, element)
      } else {
        nodeRefs.current.delete(nodeId)
      }
    },
    []
  )

  const recomputeLines = useCallback(() => {
    const container = canvasRef.current
    if (!container) return
    const containerRect = container.getBoundingClientRect()
    if (containerRect.width <= 0 || containerRect.height <= 0) {
      setSvgState((prev) =>
        prev.lines.length === 0 ? prev : { width: 0, height: 0, lines: [] }
      )
      return
    }

    let maxLinkMetric = 0
    for (const link of graph.links) {
      const value = link.metricValue > 0 ? link.metricValue : link.requests
      if (value > maxLinkMetric) maxLinkMetric = value
    }

    const lines: LineGeometry[] = []
    for (const link of graph.links) {
      const sourceEl = nodeRefs.current.get(link.sourceId)
      const targetEl = nodeRefs.current.get(link.targetId)
      if (!sourceEl || !targetEl) continue
      const sourceRect = sourceEl.getBoundingClientRect()
      const targetRect = targetEl.getBoundingClientRect()
      // Only draw Bézier links when the columns sit side-by-side (desktop 3-col).
      if (targetRect.left <= sourceRect.right + 4) continue

      const x1 = sourceRect.right - containerRect.left
      const y1 = sourceRect.top + sourceRect.height / 2 - containerRect.top
      const x2 = targetRect.left - containerRect.left
      const y2 = targetRect.top + targetRect.height / 2 - containerRect.top
      const dx = (x2 - x1) * 0.48
      const path = `M ${x1.toFixed(1)} ${y1.toFixed(1)} C ${(x1 + dx).toFixed(1)} ${y1.toFixed(1)}, ${(x2 - dx).toFixed(1)} ${y2.toFixed(1)}, ${x2.toFixed(1)} ${y2.toFixed(1)}`
      const value = link.metricValue > 0 ? link.metricValue : link.requests
      lines.push({
        id: link.id,
        path,
        strokeWidth: computeLinkThickness(value, maxLinkMetric),
        colorVar: link.colorVar,
        hasFailure: link.failedCount > 0,
      })
    }

    setSvgState({
      width: Math.round(containerRect.width),
      height: Math.round(containerRect.height),
      lines,
    })
  }, [graph.links])

  useLayoutEffect(() => {
    recomputeLines()
    const container = canvasRef.current
    if (!container) return
    if (typeof ResizeObserver !== 'undefined') {
      const observer = new ResizeObserver(() => recomputeLines())
      observer.observe(container)
      return () => observer.disconnect()
    }
    return undefined
  }, [recomputeLines, graph])

  const toggleNode = useCallback((nodeId: string) => {
    setSelectedNodeId((prev) => (prev === nodeId ? null : nodeId))
  }, [])

  // Build deep-link query params for the selected node.
  const selectedLogsSearch = useMemo(() => {
    if (!selectedNode) return null
    const base: Record<string, string | number> = {
      view: 'attempts',
      ...(data?.window?.from ? { from: data.window.from } : {}),
      ...(data?.window?.to ? { to: data.window.to } : {}),
      ...(failedOnly ? { status: 'failed' } : {}),
    }
    if (selectedNode.column === 'key' && selectedNode.downstreamKeyId) {
      return { ...base, downstreamKeyId: selectedNode.downstreamKeyId }
    }
    if (selectedNode.column === 'model' && selectedNode.model) {
      return { ...base, q: selectedNode.model }
    }
    if (selectedNode.column === 'upstream') {
      return {
        ...base,
        ...(selectedNode.siteId ? { siteId: selectedNode.siteId } : {}),
        ...(selectedNode.channelId
          ? { channelId: selectedNode.channelId }
          : {}),
      }
    }
    return base
  }, [selectedNode, data?.window, failedOnly])

  const selectedChannelSearch = useMemo(() => {
    if (!selectedNode || selectedNode.column !== 'upstream') return null
    if (selectedNode.channelId && selectedNode.channelId > 0) {
      return { channelId: selectedNode.channelId }
    }
    return null
  }, [selectedNode])

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
                  <div
                    ref={canvasRef}
                    className='relative grid grid-cols-1 gap-4 lg:grid-cols-3 lg:gap-16'
                  >
                    {/* SVG Bézier connector layer (solid OKLCH stroke, no gradients) */}
                    {svgState.width > 0 && svgState.lines.length > 0 && (
                      <svg
                        aria-hidden='true'
                        width={svgState.width}
                        height={svgState.height}
                        className='pointer-events-none absolute inset-0 hidden lg:block'
                      >
                        {svgState.lines.map((line) => {
                          const isHighlighted =
                            highlight.highlightedLinks === null ||
                            highlight.highlightedLinks.has(line.id)
                          let strokeOpacity = 0.28
                          if (highlight.highlightedLinks !== null) {
                            strokeOpacity = isHighlighted ? 0.8 : 0.06
                          }
                          return (
                            <path
                              key={line.id}
                              d={line.path}
                              fill='none'
                              stroke={
                                line.hasFailure && failedOnly
                                  ? 'var(--destructive)'
                                  : line.colorVar
                              }
                              strokeWidth={line.strokeWidth}
                              strokeOpacity={strokeOpacity}
                              strokeLinecap='round'
                              className='transition-opacity duration-150'
                            />
                          )
                        })}
                      </svg>
                    )}

                    <FlowColumnPanel
                      title={t('observability.flow.columns.key')}
                      icon={KeyRound}
                      nodes={graph.keys}
                      metric={metric}
                      locale={locale}
                      selectedNodeId={selectedNode?.id ?? null}
                      highlightedNodes={highlight.highlightedNodes}
                      onSelect={toggleNode}
                      registerRef={registerNodeRef}
                    />
                    <FlowColumnPanel
                      title={t('observability.flow.columns.model')}
                      icon={Layers}
                      nodes={graph.models}
                      metric={metric}
                      locale={locale}
                      selectedNodeId={selectedNode?.id ?? null}
                      highlightedNodes={highlight.highlightedNodes}
                      onSelect={toggleNode}
                      registerRef={registerNodeRef}
                    />
                    <FlowColumnPanel
                      title={t('observability.flow.columns.upstream')}
                      icon={Server}
                      nodes={graph.upstreams}
                      metric={metric}
                      locale={locale}
                      selectedNodeId={selectedNode?.id ?? null}
                      highlightedNodes={highlight.highlightedNodes}
                      onSelect={toggleNode}
                      registerRef={registerNodeRef}
                    />
                  </div>

                  {/* Action bar anchored below the diagram */}
                  <div
                    role='region'
                    aria-label={t('observability.flow.actionBar.label')}
                    aria-live='polite'
                    className='bg-muted/40 flex flex-wrap items-center justify-between gap-3 rounded-lg border px-3.5 py-2.5'
                  >
                    {selectedNode ? (
                      <>
                        <div className='flex min-w-0 flex-wrap items-center gap-2 text-xs'>
                          <Badge variant='secondary'>
                            {t(
                              `observability.flow.columns.${selectedNode.column}`
                            )}
                          </Badge>
                          <span className='text-foreground truncate font-medium'>
                            {selectedNode.label}
                          </span>
                          {selectedNode.sublabel && (
                            <span className='text-muted-foreground font-mono'>
                              {selectedNode.sublabel}
                            </span>
                          )}
                          <span className='text-muted-foreground tabular-nums'>
                            {t('observability.flow.actionBar.summary', {
                              requests: formatInt(
                                selectedNode.requests,
                                locale
                              ),
                              failed: formatInt(
                                selectedNode.failedCount,
                                locale
                              ),
                              paths: formatInt(
                                highlight.matchingEdgeCount,
                                locale
                              ),
                            })}
                          </span>
                        </div>
                        <div className='flex flex-wrap items-center gap-2'>
                          {selectedLogsSearch && (
                            <Link
                              to='/proxy-logs'
                              search={selectedLogsSearch}
                              className={buttonVariants({
                                size: 'xs',
                                variant: 'outline',
                              })}
                            >
                              {t('observability.flow.actionBar.goToLogs')}
                              <ArrowUpRight
                                className='size-3.5'
                                aria-hidden='true'
                              />
                            </Link>
                          )}
                          {selectedChannelSearch && (
                            <Link
                              to='/channels'
                              search={selectedChannelSearch}
                              className={buttonVariants({
                                size: 'xs',
                                variant: 'outline',
                              })}
                            >
                              {t('observability.flow.actionBar.goToChannel')}
                              <ArrowUpRight
                                className='size-3.5'
                                aria-hidden='true'
                              />
                            </Link>
                          )}
                        </div>
                      </>
                    ) : (
                      <p className='text-muted-foreground text-xs'>
                        {t('observability.flow.actionBar.idleHint')}
                      </p>
                    )}
                  </div>

                  {/* Screen-reader accessible semantic table */}
                  <table className='sr-only'>
                    <caption>{t('observability.flow.cardTitle')}</caption>
                    <thead>
                      <tr>
                        <th scope='col'>
                          {t('observability.flow.columns.key')}
                        </th>
                        <th scope='col'>
                          {t('observability.flow.columns.model')}
                        </th>
                        <th scope='col'>
                          {t('observability.flow.columns.upstream')}
                        </th>
                        <th scope='col'>
                          {t('observability.flow.summary.requests')}
                        </th>
                        <th scope='col'>
                          {t('observability.flow.summary.failed')}
                        </th>
                        <th scope='col'>
                          {t('observability.flow.summary.tokens')}
                        </th>
                        <th scope='col'>
                          {t('observability.flow.summary.cost')}
                        </th>
                      </tr>
                    </thead>
                    <tbody>
                      {filteredEdges.map((edge) => (
                        <tr
                          key={`${edge.downstreamKeyId ?? 'none'}:${edge.downstreamKeyName ?? ''}:${edge.model}:${edge.siteId ?? 'none'}:${edge.siteName ?? ''}:${edge.channelId ?? 'none'}`}
                        >
                          <td>
                            {edge.downstreamKeyName ||
                              (edge.downstreamKeyId
                                ? `#${edge.downstreamKeyId}`
                                : t('observability.flow.unassignedKey'))}
                          </td>
                          <td>
                            {edge.model || t('observability.flow.unknownModel')}
                          </td>
                          <td>
                            {edge.siteName ||
                              (edge.siteId
                                ? `#${edge.siteId}`
                                : t('observability.flow.unassignedUpstream'))}
                            {edge.channelId ? ` (#${edge.channelId})` : ''}
                          </td>
                          <td>{formatInt(edge.requests, locale)}</td>
                          <td>{formatInt(edge.failedCount, locale)}</td>
                          <td>{formatInt(edge.totalTokens, locale)}</td>
                          <td>
                            {formatCurrency(edge.totalCost, {
                              fractionDigits: 4,
                              locale,
                            })}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </>
              )}
            </CardContent>
          </Card>
        </>
      )}
    </div>
  )
}

function FlowColumnPanel({
  title,
  icon: Icon,
  nodes,
  metric,
  locale,
  selectedNodeId,
  highlightedNodes,
  onSelect,
  registerRef,
}: {
  title: string
  icon: typeof KeyRound
  nodes: FlowNode[]
  metric: FlowMetric
  locale: string
  selectedNodeId: string | null
  highlightedNodes: Set<string> | null
  onSelect: (nodeId: string) => void
  registerRef: (nodeId: string, element: HTMLButtonElement | null) => void
}) {
  const { t } = useTranslation()

  return (
    <section className='relative z-10 flex min-w-0 flex-col gap-2'>
      <header className='text-muted-foreground flex items-center justify-between px-1 text-xs font-medium'>
        <span className='flex items-center gap-1.5'>
          <Icon className='size-3.5' aria-hidden='true' />
          {title}
        </span>
        <span className='tabular-nums'>{nodes.length}</span>
      </header>
      <div className='flex flex-col gap-1.5'>
        {nodes.map((node) => {
          const isSelected = selectedNodeId === node.id
          const isDimmed =
            highlightedNodes !== null && !highlightedNodes.has(node.id)
          const barWidth = `${Math.max(4, Math.round(node.share * 100))}%`
          return (
            <button
              key={node.id}
              ref={(el) => registerRef(node.id, el)}
              type='button'
              aria-pressed={isSelected}
              onClick={() => onSelect(node.id)}
              className={cn(
                'bg-card hover:border-foreground/25 focus-visible:ring-ring relative flex w-full flex-col gap-1.5 rounded-lg border px-3 py-2 text-left transition-opacity outline-none focus-visible:ring-2',
                isSelected &&
                  'border-primary bg-accent/40 ring-primary/30 ring-1',
                isDimmed && 'opacity-35'
              )}
            >
              <div className='flex min-w-0 items-center justify-between gap-2'>
                <div className='flex min-w-0 items-center gap-1.5'>
                  <span
                    aria-hidden='true'
                    className='size-2 shrink-0 rounded-full'
                    style={{ backgroundColor: node.colorVar }}
                  />
                  {node.column === 'model' && node.model ? (
                    <ModelPill model={node.model} variant='inline' />
                  ) : (
                    <span
                      className='truncate text-sm font-medium'
                      title={node.label}
                    >
                      {node.label}
                    </span>
                  )}
                  {node.sublabel && (
                    <span className='text-muted-foreground text-2xs shrink-0 font-mono'>
                      {node.sublabel}
                    </span>
                  )}
                </div>
                <span className='text-foreground shrink-0 text-xs font-medium tabular-nums'>
                  {formatMetricValue(node, metric, locale)}
                </span>
              </div>

              <div className='text-2xs flex items-center justify-between gap-2 tabular-nums'>
                <div className='bg-muted h-1 flex-1 overflow-hidden rounded-full'>
                  <div
                    className='h-full rounded-full'
                    style={{
                      width: barWidth,
                      backgroundColor: node.colorVar,
                    }}
                  />
                </div>
                <span className='text-muted-foreground shrink-0'>
                  {`${(node.share * 100).toFixed(1)}%`}
                </span>
                {node.failedCount > 0 && (
                  <span
                    className='text-destructive-soft-fg inline-flex shrink-0 items-center gap-0.5 font-medium'
                    title={t('observability.flow.failedCountTitle', {
                      count: node.failedCount,
                    })}
                  >
                    <AlertTriangle className='size-3' aria-hidden='true' />
                    {formatInt(node.failedCount, locale)}
                  </span>
                )}
              </div>
            </button>
          )
        })}
      </div>
    </section>
  )
}
