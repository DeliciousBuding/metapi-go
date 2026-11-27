export type ProxyLogStatusFilter = 'all' | 'success' | 'failed'
type ProxyLogClientConfidence = 'exact' | 'heuristic' | 'unknown' | null
type ProxyLogUsageSource = 'upstream' | 'self-log' | 'unknown' | null

type ProxyLogConversionDiagnostic = {
  phase: string
  code: string
  severity: 'warning' | 'error' | 'unknown' | string
  fieldClass: string
}

export type ProxyLogConversion = {
  sourceProtocol?: string | null
  upstreamProtocol?: string | null
  owner?: string | null
  quality?: string | null
  requestSteps?: string[] | null
  responseSteps?: string[] | null
  diagnostics?: ProxyLogConversionDiagnostic[] | null
}

export type ProxyLogStreamTiming = {
  headerMs?: number | null
  firstReasoningMs?: number | null
  firstTextMs?: number | null
  lastContentMs?: number | null
  maxContentGapMs?: number | null
  reasoningEvidence?:
    | 'none'
    | 'visible'
    | 'hidden'
    | 'summaryOnly'
    | string
    | null
  decodeTokens?: number | null
  decodeWindowMs?: number | null
  decodeTps?: number | null
  decodeStatus?:
    | 'measured'
    | 'bufferedBurst'
    | 'hiddenReasoning'
    | 'insufficientEvidence'
    | 'interrupted'
    | string
    | null
}

export type ProxyLogBillingToolSurcharge = {
  tool: string
  unit: string
  quantity: number
  source?: string | null
  unitPrice?: number | null
  cost: number
}

export type ProxyLogBillingDetails = {
  quotaType: number
  usage: {
    promptTokens: number
    completionTokens: number
    totalTokens: number
    cacheReadTokens: number
    cacheCreationTokens: number
    cacheCreation5mTokens?: number
    cacheCreation1hTokens?: number
    cacheCreation12hTokens?: number
    billablePromptTokens: number
    promptTokensIncludeCache: boolean | null
  }
  pricing: {
    modelRatio: number
    completionRatio: number
    cacheRatio: number
    cacheCreationRatio: number
    groupRatio: number
  }
  breakdown: {
    inputPerMillion: number
    outputPerMillion: number
    cacheReadPerMillion: number
    cacheCreationPerMillion: number
    inputCost: number
    outputCost: number
    cacheReadCost: number
    cacheCreationCost: number
    cacheCreation5mCost?: number
    cacheCreation1hCost?: number
    cacheCreation12hCost?: number
    toolSurcharges?: ProxyLogBillingToolSurcharge[]
    totalCost: number
  }
} | null

export type ProxyLogListItem = {
  /** Canonical ingress path; null for historical or unrecognized surfaces. */
  downstreamPath?: string | null
  requestId?: string | null
  gatewayRequestId?: string | null
  id: number
  createdAt: string
  modelRequested: string
  modelActual: string
  status: string
  httpStatus?: number | null
  latencyMs: number
  isStream?: boolean | null
  firstByteLatencyMs?: number | null
  firstOutputLatencyMs?: number | null
  upstreamReportedModel?: string | null
  totalTokens: number | null
  retryCount: number
  upstreamChannelId?: number | null
  upstreamGrantId?: number | null
  accountId?: number | null
  siteId?: number | null
  username?: string | null
  siteName?: string | null
  siteUrl?: string | null
  errorMessage?: string | null
  downstreamKeyId?: number | null
  downstreamKeyName?: string | null
  downstreamKeyGroupName?: string | null
  downstreamKeyTags?: string[]
  clientFamily?: string | null
  clientAppId?: string | null
  clientAppName?: string | null
  clientConfidence?: ProxyLogClientConfidence
  usageSource?: ProxyLogUsageSource
  promptTokens?: number | null
  completionTokens?: number | null
  cacheReadTokens?: number | null
  cacheCreationTokens?: number | null
  estimatedCost?: number | null
}

export type ProxyLogDetail = ProxyLogListItem & {
  routeId?: number | null
  channelId?: number | null
  httpStatus?: number | null
  billingDetails?: ProxyLogBillingDetails
  conversion?: ProxyLogConversion | null
  streamTiming?: ProxyLogStreamTiming | null
  requestAttempts?: ProxyLogRequestAttempts
}

/** One sibling dispatch attempt of the same client request (request-level diagnosis). */
export type ProxyLogAttempt = {
  id: number
  ordinal: number
  status: string
  httpStatus: number | null
  channelId: number | null
  siteId: number | null
  siteName: string
  accountUsername: string
  modelActual: string
  errorMessage: string
  latencyMs: number
  createdAt: string
  isCurrent: boolean
}

/**
 * Request-level attempt view on the log detail. Every dispatch attempt
 * persists its own row; requestId correlates one client request and ordinal
 * is the retry-loop position. partial marks a known-incomplete persisted set:
 * requestIdMissing (legacy row, siblings unverifiable — no list returned) or
 * ordinalGap (a loop iteration left no row, e.g. concurrency-skip). The
 * lastAttempt is the highest-ordinal recorded row, not the gateway response
 * or proof of client receipt. It is null when correlation/query is unavailable.
 */
export type ProxyLogRequestAttempts = {
  requestId: string | null
  currentOrdinal: number
  partial: boolean
  partialReasons: string[]
  attempts: ProxyLogAttempt[]
  lastAttempt?: {
    status: string
    httpStatus: number | null
    logId: number
  } | null
  /** @deprecated Legacy last-row projection, not a request outcome. */
  terminal: { status: string; httpStatus: number | null; logId: number } | null
}

type ProxyLogsSummary = {
  totalCount: number
  successCount: number
  failedCount: number
  totalCost: number
  totalTokensAll: number
}

export type ProxyLogsQuery = {
  limit?: number
  offset?: number
  status?: ProxyLogStatusFilter
  search?: string
  client?: string
  siteId?: number
  // Channel that served the request (`proxy_logs.channel_id`). Server-side
  // like every other list filter so total + summary stay consistent.
  channelId?: number
  // Downstream key that made the request (`proxy_logs.downstream_api_key_id`)
  // — the "this key's traffic" drill-down linked from the keys page and the
  // connect dialog's verify outcome. Server-side like every other filter.
  downstreamKeyId?: number
  from?: string
  to?: string
  // Latency bounds (ms) for the "Slow only" filter. Applied server-side so
  // total + summary agree with the returned items (no client-side re-filter).
  latencyMin?: number | null
  latencyMax?: number | null
}

type ProxyLogClientOption = {
  value: string
  label: string
}

export type ProxyLogsResponse = {
  items: ProxyLogListItem[]
  total: number
  page: number
  pageSize: number
  clientOptions: ProxyLogClientOption[]
  summary: ProxyLogsSummary
}
