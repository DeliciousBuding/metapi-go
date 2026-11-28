import { AlertTriangle, type LucideIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { ModelPill } from '@/components/common/model-pill'
import { formatCurrency, formatInt } from '@/lib/format'
import { cn } from '@/lib/utils'

import type { FlowMetric, FlowNode } from './flow-graph'

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

export function FlowColumnPanel({
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
  icon: LucideIcon
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
