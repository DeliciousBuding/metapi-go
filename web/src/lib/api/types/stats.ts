export type OverviewPeriod = '24h' | '7d' | '30d' | 'all'

export type OverviewSiteSample = {
  bucketTs: string
  total: number
  success: number
  avgLatencyMs: number | null
}

export type OverviewReport = {
  period: OverviewPeriod
  window: { from?: string; to: string }
  summary: {
    totalCount: number
    successCount: number
    failedCount: number
    totalTokensAll: number
    totalCost: number
    averageLatencyMs: number | null
    averageFirstByteMs?: number | null
  }
  requestSummary: {
    totalCount: number
    successCount: number
    failedCount: number
    legacyAttemptCount: number
    partial: boolean
    partialReasons: string[]
  }
  gatewayRequestSummary?: {
    totalCount: number
    completedCount: number
    failedCount: number
    cancelledCount: number
    rejectedCount: number
    coverageStartedAt: string | null
  }
  gatewayRequestPoints?: Array<{
    date: string
    requests: number
    completedCount: number
    failedCount: number
    cancelledCount: number
    rejectedCount: number
  }>
  points: Array<{
    date: string
    requests: number
    successCount: number
    successRate: number
    tokens: number
    cost: number
  }>
  requestPoints: Array<{
    date: string
    requests: number
    successCount: number
    successRate: number
  }>
  siteAvailability: Array<{
    siteId: number
    siteName: string
    totalRequests: number
    successCount: number
    failedCount: number
    averageLatencyMs: number | null
    averageFirstByteMs?: number | null
    recentSamples?: OverviewSiteSample[]
  }>
  models: Array<{ model: string; calls: number; tokens: number; cost: number }>
}

export type TrafficFlowEdge = {
  downstreamKeyId: number | null
  downstreamKeyName: string | null
  model: string
  siteId: number | null
  siteName: string | null
  channelId: number | null
  requests: number
  failedCount: number
  totalTokens: number
  totalCost: number
  averageLatencyMs: number | null
}

export type TrafficFlowResponse = {
  period: OverviewPeriod
  window: { from?: string; to: string }
  totalRequests: number
  totalFailed: number
  totalTokens: number
  totalCost: number
  edges: TrafficFlowEdge[]
}

// A2: model cost distribution + latency chart gallery.
type ModelCostItem = {
  model: string
  label: string
  cost: number
  calls: number
  tokens: number
}

export type ModelCostDistributionResponse = {
  days: number
  since: string
  topN: number
  items: ModelCostItem[]
  totals: { cost: number; calls: number; tokens: number }
}

type LatencyBucket = {
  bucketStartMs: number
  bucketEndMs: number
  label: string
  count: number
  percent: number
}

export type LatencyHistogramResponse = {
  days: number
  since: string
  bucketMs: number
  total: number
  buckets: LatencyBucket[]
}

type LatencyTrendPoint = {
  date: string
  requests: number
  avgLatencyMs: number | null
  maxLatencyMs: number | null
  avgFirstByteMs: number | null
  p95LatencyMs: number | null
  successRate: number
}

export type LatencyTrendResponse = {
  days: number
  points: LatencyTrendPoint[]
  p95SampleCap: number
  truncatedDays: string[]
}

// read-only multiplier/rate overview.
export type RateOverviewResponse = {
  generatedAt: string
  summary: {
    accountsWithUnitCost: number
    accountsTotal: number
    channelsTotal: number
    channelsEnabled: number
  }
  accounts: Array<{
    accountId: number
    username: string
    siteId?: number | null
    siteName: string
    unitCost?: number | null
    channelCount: number
    totalWeight: number
  }>
  channels: Array<{
    channelId: number
    routeId?: number | null
    routePattern: string
    accountId?: number | null
    username: string
    modelName: string
    weight: number
    enabled: boolean
  }>
  sites: Array<{ siteId: number; siteName: string; globalWeight: number }>
  keys: Array<{ keyId: number; name: string; keyWeight?: number | null }>
  models: Array<{
    model: string
    calls: number
    spend: number
    tokens: number
  }>
}
