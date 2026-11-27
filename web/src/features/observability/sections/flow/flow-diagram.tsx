import { KeyRound, Layers, Server } from 'lucide-react'
import { useCallback, useLayoutEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { FlowColumnPanel } from './flow-column-panel'
import type {
  buildFlowGraph,
  resolveSelectionHighlight,
  FlowMetric,
} from './flow-graph'

type LineGeometry = {
  id: string
  path: string
  strokeWidth: number
  colorVar: string
  hasFailure: boolean
}

function computeLinkThickness(value: number, maxValue: number): number {
  if (maxValue <= 0 || value <= 0) return 1.5
  const ratio = Math.min(1, value / maxValue)
  return Number((1.5 + ratio * 6.5).toFixed(2))
}

export function FlowDiagram({
  graph,
  highlight,
  selectedNodeId,
  metric,
  locale,
  onSelect,
  failedOnly,
}: {
  graph: ReturnType<typeof buildFlowGraph>
  highlight: ReturnType<typeof resolveSelectionHighlight>
  selectedNodeId: string | null
  metric: FlowMetric
  locale: string
  onSelect: (nodeId: string) => void
  failedOnly: boolean
}) {
  const { t } = useTranslation()
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
  const panelProps = {
    metric,
    locale,
    selectedNodeId,
    onSelect,
    highlightedNodes: highlight.highlightedNodes,
    registerRef: registerNodeRef,
  }
  return (
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
        {...panelProps}
      />
      <FlowColumnPanel
        title={t('observability.flow.columns.model')}
        icon={Layers}
        nodes={graph.models}
        {...panelProps}
      />
      <FlowColumnPanel
        title={t('observability.flow.columns.upstream')}
        icon={Server}
        nodes={graph.upstreams}
        {...panelProps}
      />
    </div>
  )
}
