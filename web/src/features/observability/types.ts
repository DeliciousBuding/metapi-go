// metapi-go/features/observability — shared types for the Observability
// workspace (Overview / Health / Proxy Logs). The sections own their data
// fetching; these types describe the backend wire contracts they consume.

import type { ReactNode } from 'react'

export type ObservabilitySectionId = 'overview' | 'health'

export type ObservabilitySection = {
  id: ObservabilitySectionId
  title: string
  description?: string
  build: () => ReactNode
}

// ---- /api/stats/usage-heatmap ------------------------------------------

export type UsageHeatmapCell = {
  bucket: string
  key: string
  label: string
  calls: number
  tokens: number
  spend: number
}

export type UsageHeatmapResponse = {
  dimension: 'site' | 'model'
  days: number
  since: string
  source: string
  cellLimit: number
  count: number
  cells: UsageHeatmapCell[]
}

// ---- /api/stats/slow-requests ------------------------------------------

export type SlowRequestItem = {
  id: number
  model: string
  status: string
  latencyMs: number
  firstByteLatencyMs: number
  httpStatus: number
  requestId: string
  accountId: number | null
  siteId: number | null
  siteName: string
  createdAt: string
}

export type SlowRequestsResponse = {
  hours: number
  minLatencyMs: number
  limit: number
  since: string
  count: number
  items: SlowRequestItem[]
}

// ---- /api/monitor/health -------------------------------------------------

export type RuntimeHealthBreaker = {
  siteId: number
  model: string
  breakerLevel: number
  breakerUntilMs?: number | null
  penaltyScore: number
}

type StatusCounts = {
  total: number
  active: number
  disabled: number
  other: number
}

export type CooldownChannel = {
  channelId: number
  accountId: number
  siteId: number
  siteName: string
  failCount: number
  cooldownUntil: string
}

export type MonitorHealthResponse = {
  generatedAt: string
  runtimeHealth: {
    sitesTracked: number
    sitesBreakerOpen: number
    modelsTracked: number
    modelsBreakerOpen: number
    openBreakers: RuntimeHealthBreaker[]
  }
  cooldown: {
    channelsCooling: number
    channelsWithFailures: number
    channelsRecentlyFailed: number
    cooling: CooldownChannel[]
  }
  sites: StatusCounts
  accounts: StatusCounts
}

export const observabilityKeys = {
  all: ['observability'] as const,
  usageHeatmap: (params: { days?: number; dimension?: 'site' | 'model' }) =>
    [...observabilityKeys.all, 'usage-heatmap', params] as const,
  slowRequests: (params: {
    limit?: number
    minLatencyMs?: number
    hours?: number
  }) => [...observabilityKeys.all, 'slow-requests', params] as const,
  monitorHealth: () => [...observabilityKeys.all, 'monitor-health'] as const,
}
