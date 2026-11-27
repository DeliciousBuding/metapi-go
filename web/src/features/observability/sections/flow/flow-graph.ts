import type { TrafficFlowEdge } from '@/lib/api'

export type FlowMetric = 'requests' | 'cost' | 'tokens'
type FlowColumn = 'key' | 'model' | 'upstream'

export type FlowNode = {
  id: string
  column: FlowColumn
  label: string
  sublabel?: string
  requests: number
  failedCount: number
  totalTokens: number
  totalCost: number
  metricValue: number
  share: number
  colorVar: string
  // Original identifiers for deep-link construction.
  downstreamKeyId?: number | null
  model?: string
  siteId?: number | null
  channelId?: number | null
}

type FlowLink = {
  id: string
  sourceId: string
  targetId: string
  stage: 'key-to-model' | 'model-to-upstream'
  requests: number
  failedCount: number
  totalTokens: number
  totalCost: number
  metricValue: number
  colorVar: string
  /** Underlying edges that traverse this pair (used for selection highlighting). */
  edgeKeys: Set<string>
}

type FlowGraph = {
  keys: FlowNode[]
  models: FlowNode[]
  upstreams: FlowNode[]
  links: FlowLink[]
  totalMetric: number
  edgeToNodeIds: Map<string, [string, string, string]>
  nodeToEdgeKeys: Map<string, Set<string>>
}

const CHART_VARS = [
  'var(--chart-1)',
  'var(--chart-2)',
  'var(--chart-3)',
  'var(--chart-4)',
  'var(--chart-5)',
] as const

function getEdgeMetricValue(edge: TrafficFlowEdge, metric: FlowMetric): number {
  if (metric === 'cost') return Math.max(0, edge.totalCost)
  if (metric === 'tokens') return Math.max(0, edge.totalTokens)
  return Math.max(0, edge.requests)
}

function keyNodeId(edge: TrafficFlowEdge): string {
  if (edge.downstreamKeyId != null && edge.downstreamKeyId > 0) {
    return `key:${edge.downstreamKeyId}`
  }
  const name = edge.downstreamKeyName?.trim()
  if (name) return `key:name:${name}`
  return 'key:unassigned'
}

function modelNodeId(edge: TrafficFlowEdge): string {
  const name = edge.model?.trim()
  return name ? `model:${name}` : 'model:unknown'
}

function upstreamNodeId(edge: TrafficFlowEdge): string {
  const sitePart =
    edge.siteId != null && edge.siteId > 0 ? `s${edge.siteId}` : 's0'
  const channelPart =
    edge.channelId != null && edge.channelId > 0 ? `c${edge.channelId}` : 'c0'
  if (sitePart === 's0' && channelPart === 'c0') {
    const siteName = edge.siteName?.trim()
    return siteName ? `upstream:name:${siteName}` : 'upstream:unassigned'
  }
  return `upstream:${sitePart}:${channelPart}`
}

export function buildFlowGraph(
  edges: TrafficFlowEdge[],
  metric: FlowMetric,
  labels: {
    unassignedKey: string
    unknownModel: string
    unassignedUpstream: string
    channelPrefix: string
    sitePrefix: string
  }
): FlowGraph {
  const keyMap = new Map<string, FlowNode>()
  const modelMap = new Map<string, FlowNode>()
  const upstreamMap = new Map<string, FlowNode>()
  const linkMap = new Map<string, FlowLink>()
  const edgeToNodeIds = new Map<string, [string, string, string]>()
  const nodeToEdgeKeys = new Map<string, Set<string>>()

  const registerEdgeOnNode = (nodeId: string, edgeKey: string) => {
    let set = nodeToEdgeKeys.get(nodeId)
    if (!set) {
      set = new Set<string>()
      nodeToEdgeKeys.set(nodeId, set)
    }
    set.add(edgeKey)
  }

  let totalMetric = 0

  edges.forEach((edge, index) => {
    const value = getEdgeMetricValue(edge, metric)
    totalMetric += value

    const kId = keyNodeId(edge)
    const mId = modelNodeId(edge)
    const uId = upstreamNodeId(edge)
    const edgeKey = `${index}:${kId}|${mId}|${uId}`

    edgeToNodeIds.set(edgeKey, [kId, mId, uId])
    registerEdgeOnNode(kId, edgeKey)
    registerEdgeOnNode(mId, edgeKey)
    registerEdgeOnNode(uId, edgeKey)

    // Key node
    const existingKey = keyMap.get(kId)
    if (existingKey) {
      existingKey.requests += edge.requests
      existingKey.failedCount += edge.failedCount
      existingKey.totalTokens += edge.totalTokens
      existingKey.totalCost += edge.totalCost
      existingKey.metricValue += value
    } else {
      const name = edge.downstreamKeyName?.trim()
      const label =
        name ||
        (edge.downstreamKeyId != null && edge.downstreamKeyId > 0
          ? `#${edge.downstreamKeyId}`
          : labels.unassignedKey)
      const sublabel =
        name && edge.downstreamKeyId != null && edge.downstreamKeyId > 0
          ? `#${edge.downstreamKeyId}`
          : undefined
      keyMap.set(kId, {
        id: kId,
        column: 'key',
        label,
        sublabel,
        requests: edge.requests,
        failedCount: edge.failedCount,
        totalTokens: edge.totalTokens,
        totalCost: edge.totalCost,
        metricValue: value,
        share: 0,
        colorVar: CHART_VARS[0],
        downstreamKeyId: edge.downstreamKeyId,
      })
    }

    // Model node
    const existingModel = modelMap.get(mId)
    if (existingModel) {
      existingModel.requests += edge.requests
      existingModel.failedCount += edge.failedCount
      existingModel.totalTokens += edge.totalTokens
      existingModel.totalCost += edge.totalCost
      existingModel.metricValue += value
    } else {
      const modelName = edge.model?.trim()
      modelMap.set(mId, {
        id: mId,
        column: 'model',
        label: modelName || labels.unknownModel,
        requests: edge.requests,
        failedCount: edge.failedCount,
        totalTokens: edge.totalTokens,
        totalCost: edge.totalCost,
        metricValue: value,
        share: 0,
        colorVar: CHART_VARS[1],
        model: modelName || undefined,
      })
    }

    // Upstream node (site + optional channel)
    const existingUpstream = upstreamMap.get(uId)
    if (existingUpstream) {
      existingUpstream.requests += edge.requests
      existingUpstream.failedCount += edge.failedCount
      existingUpstream.totalTokens += edge.totalTokens
      existingUpstream.totalCost += edge.totalCost
      existingUpstream.metricValue += value
    } else {
      const siteName = edge.siteName?.trim()
      let label = labels.unassignedUpstream
      if (siteName) {
        label = siteName
      } else if (edge.siteId != null && edge.siteId > 0) {
        label = `${labels.sitePrefix}${edge.siteId}`
      } else if (edge.channelId != null && edge.channelId > 0) {
        label = `${labels.channelPrefix}${edge.channelId}`
      }
      const sublabel =
        edge.channelId != null &&
        edge.channelId > 0 &&
        label !== `${labels.channelPrefix}${edge.channelId}`
          ? `${labels.channelPrefix}${edge.channelId}`
          : undefined
      upstreamMap.set(uId, {
        id: uId,
        column: 'upstream',
        label,
        sublabel,
        requests: edge.requests,
        failedCount: edge.failedCount,
        totalTokens: edge.totalTokens,
        totalCost: edge.totalCost,
        metricValue: value,
        share: 0,
        colorVar: CHART_VARS[2],
        siteId: edge.siteId,
        channelId: edge.channelId,
      })
    }

    // Links: key -> model, model -> upstream
    const addLink = (
      sourceId: string,
      targetId: string,
      stage: FlowLink['stage']
    ) => {
      const id = `${sourceId}=>${targetId}`
      const existing = linkMap.get(id)
      if (existing) {
        existing.requests += edge.requests
        existing.failedCount += edge.failedCount
        existing.totalTokens += edge.totalTokens
        existing.totalCost += edge.totalCost
        existing.metricValue += value
        existing.edgeKeys.add(edgeKey)
      } else {
        linkMap.set(id, {
          id,
          sourceId,
          targetId,
          stage,
          requests: edge.requests,
          failedCount: edge.failedCount,
          totalTokens: edge.totalTokens,
          totalCost: edge.totalCost,
          metricValue: value,
          colorVar: CHART_VARS[0],
          edgeKeys: new Set([edgeKey]),
        })
      }
    }

    addLink(kId, mId, 'key-to-model')
    addLink(mId, uId, 'model-to-upstream')
  })

  const sortAndColor = (nodes: FlowNode[]) => {
    nodes.sort(
      (a, b) =>
        b.metricValue - a.metricValue ||
        b.requests - a.requests ||
        a.label.localeCompare(b.label)
    )
    const denom =
      totalMetric > 0
        ? totalMetric
        : nodes.reduce((sum, item) => sum + item.requests, 0)
    nodes.forEach((node, idx) => {
      node.colorVar = CHART_VARS[idx % CHART_VARS.length]
      node.share =
        denom > 0
          ? (totalMetric > 0 ? node.metricValue : node.requests) / denom
          : 0
    })
    return nodes
  }

  const keys = sortAndColor([...keyMap.values()])
  const models = sortAndColor([...modelMap.values()])
  const upstreams = sortAndColor([...upstreamMap.values()])

  const colorByNodeId = new Map<string, string>()
  for (const node of [...keys, ...models, ...upstreams]) {
    colorByNodeId.set(node.id, node.colorVar)
  }

  const links = [...linkMap.values()].map((link) => ({
    ...link,
    colorVar:
      link.stage === 'key-to-model'
        ? (colorByNodeId.get(link.sourceId) ?? CHART_VARS[0])
        : (colorByNodeId.get(link.targetId) ?? CHART_VARS[1]),
  }))
  links.sort((a, b) => b.metricValue - a.metricValue || b.requests - a.requests)

  return {
    keys,
    models,
    upstreams,
    links,
    totalMetric,
    edgeToNodeIds,
    nodeToEdgeKeys,
  }
}

export function resolveSelectionHighlight(
  graph: FlowGraph,
  selectedNodeId: string | null
): {
  highlightedNodes: Set<string> | null
  highlightedLinks: Set<string> | null
  matchingEdgeCount: number
} {
  if (!selectedNodeId) {
    return {
      highlightedNodes: null,
      highlightedLinks: null,
      matchingEdgeCount: graph.edgeToNodeIds.size,
    }
  }
  const activeEdges = graph.nodeToEdgeKeys.get(selectedNodeId)
  if (!activeEdges || activeEdges.size === 0) {
    return {
      highlightedNodes: new Set([selectedNodeId]),
      highlightedLinks: new Set(),
      matchingEdgeCount: 0,
    }
  }

  const highlightedNodes = new Set<string>([selectedNodeId])
  for (const edgeKey of activeEdges) {
    const triple = graph.edgeToNodeIds.get(edgeKey)
    if (!triple) continue
    highlightedNodes.add(triple[0])
    highlightedNodes.add(triple[1])
    highlightedNodes.add(triple[2])
  }

  const highlightedLinks = new Set<string>()
  for (const link of graph.links) {
    for (const edgeKey of link.edgeKeys) {
      if (activeEdges.has(edgeKey)) {
        highlightedLinks.add(link.id)
        break
      }
    }
  }

  return {
    highlightedNodes,
    highlightedLinks,
    matchingEdgeCount: activeEdges.size,
  }
}
