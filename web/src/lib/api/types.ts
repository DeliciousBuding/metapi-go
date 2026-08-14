/**
 * Response types — preserved verbatim from the legacy single-file `api.ts`.
 */

export type BackupWebdavExportType = 'all' | 'accounts' | 'preferences'

type BackupWebdavConfig = {
  enabled: boolean
  fileUrl: string
  username: string
  password?: string
  hasPassword: boolean
  passwordMasked: string
  exportType: BackupWebdavExportType
  autoSyncEnabled: boolean
  autoSyncCron: string
}

type BackupWebdavState = {
  lastSyncAt?: string | null
  lastAttemptAt?: string | null
  lastError?: string | null
}

export type BackupWebdavResponse = BackupWebdavConfig & {
  success: boolean
  message?: string
  config: BackupWebdavConfig
  state: BackupWebdavState
  fileUrl?: string
  imported?: Record<string, number>
  appliedSettings?: unknown[]
}

// C1: unified recurring-scheduler run history.
export type SchedulerRunStatus = {
  job: string
  enabled: boolean
  lastRunAt?: string
  lastStatus?: string
  runs24h: number
  success24h: number
  note?: string
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

// G1: batch model verification + history.
type ModelVerifyItem = {
  model: string
  channelId?: number | null
  accountId?: number | null
  siteId?: number | null
  siteName?: string
  status: 'success' | 'failure' | 'inconclusive' | 'skipped'
  latencyMs?: number | null
  httpStatus?: number | null
  errorText?: string | null
  healthApplied?: boolean
  createdAt?: string
}

export type VerifyBatchResponse = {
  success: boolean
  batchId: string
  probed: number
  summary: {
    success: number
    failure: number
    inconclusive: number
    skipped: number
  }
  items: ModelVerifyItem[]
  note?: string
}

export type VerifyHistoryResponse = { items: ModelVerifyItem[] }

// I1: accounts/sites global tag system.
type TagIndexItem = {
  name: string
  accounts: number
  sites: number
  total: number
}

export type TagIndexResponse = { items: TagIndexItem[] }

// H1: product risk banners.
export type Announcement = {
  id: number
  title: string
  message: string
  severity: 'info' | 'warning' | 'critical'
  link?: string | null
  enabled: boolean
  dismissed?: boolean
  dismissedAt?: string | null
  createdAt: string
  updatedAt: string
}

export type AnnouncementsResponse = { items: Announcement[] }

// K1a: model name redirects.
type ModelRedirect = {
  id: number
  accountId: number
  username?: string
  siteName?: string
  canonical: string
  actual: string
  source: 'sync' | 'manual'
  lastSeenAt?: string | null
  createdAt: string
  updatedAt: string
}

export type ModelRedirectsResponse = { items: ModelRedirect[] }

export type RedirectFixCandidate = {
  siteId: number
  siteName: string
  accountId: number
  modelName: string
  canonical: string
  actual: string
}

export type RedirectApplyResponse = {
  success: boolean
  dryRun: boolean
  candidates?: RedirectFixCandidate[]
  count: number
  removed?: number
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

export type TestChatRequestPayload = {
  model: string
  messages: Array<{ role: string; content: string }>
  targetFormat?: 'openai' | 'claude' | 'responses' | 'gemini'
  stream?: boolean
  forcedChannelId?: number | null
  temperature?: number
  top_p?: number
  max_tokens?: number
  frequency_penalty?: number
  presence_penalty?: number
  seed?: number
}

type ProxyTestMethod = 'POST' | 'GET' | 'DELETE'
type ProxyTestRequestKind = 'json' | 'multipart' | 'empty'

type ProxyTestMultipartFile = {
  field: string
  name: string
  mimeType: string
  dataUrl: string
}

export type ProxyTestRequestEnvelope = {
  method: ProxyTestMethod
  path: string
  requestKind: ProxyTestRequestKind
  stream?: boolean
  jobMode?: boolean
  rawMode?: boolean
  forcedChannelId?: number | null
  jsonBody?: unknown
  rawJsonText?: string
  multipartFields?: Record<string, string>
  multipartFiles?: ProxyTestMultipartFile[]
}

export type SystemProxyTestRequest = {
  proxyUrl?: string
}

type RuntimeRoutingWeightsPayload = {
  baseWeightFactor?: number
  valueScoreFactor?: number
  costWeight?: number
  balanceWeight?: number
  usageWeight?: number
}

export type RuntimeSettingsPayload = {
  systemName?: string
  logo?: string
  footer?: string
  about?: string
  homePageContent?: string
  serverAddress?: string
  proxyToken?: string
  systemProxyUrl?: string
  payloadRules?: Record<string, unknown> | null
  modelAvailabilityProbeEnabled?: boolean
  codexUpstreamWebsocketEnabled?: boolean
  responsesCompactFallbackToResponsesEnabled?: boolean
  disableCrossProtocolFallback?: boolean
  proxySessionChannelConcurrencyLimit?: number
  proxySessionChannelQueueWaitMs?: number
  proxyDebugTraceEnabled?: boolean
  proxyDebugCaptureHeaders?: boolean
  proxyDebugCaptureBodies?: boolean
  proxyDebugCaptureStreamChunks?: boolean
  proxyDebugTargetSessionId?: string
  proxyDebugTargetClientKind?: string
  proxyDebugTargetModel?: string
  proxyDebugRetentionHours?: number
  proxyDebugMaxBodyBytes?: number
  checkinCron?: string
  checkinScheduleMode?: 'cron' | 'interval' | 'window'
  checkinIntervalHours?: number
  checkinWindowStart?: string
  checkinWindowEnd?: string
  checkinSchedule?: ScheduleSpecV1
  balanceRefreshCron?: string
  balanceRefreshSchedule?: ScheduleSpecV1
  logCleanupCron?: string
  logCleanupSchedule?: ScheduleSpecV1
  logCleanupUsageLogsEnabled?: boolean
  logCleanupProgramLogsEnabled?: boolean
  logCleanupRetentionDays?: number
  webhookUrl?: string
  barkUrl?: string
  webhookEnabled?: boolean
  barkEnabled?: boolean
  serverChanEnabled?: boolean
  serverChanKey?: string
  telegramEnabled?: boolean
  telegramApiBaseUrl?: string
  telegramBotToken?: string
  telegramChatId?: string
  telegramUseSystemProxy?: boolean
  telegramMessageThreadId?: string
  smtpEnabled?: boolean
  smtpHost?: string
  smtpPort?: number
  smtpSecure?: boolean
  smtpUser?: string
  smtpPass?: string
  smtpFrom?: string
  smtpTo?: string
  // extended notification channels + per-task toggles
  feishuEnabled?: boolean
  feishuWebhook?: string
  feishuSecret?: string
  feishuSecretMasked?: string
  dingtalkEnabled?: boolean
  dingtalkWebhook?: string
  dingtalkSecret?: string
  dingtalkSecretMasked?: string
  wecomEnabled?: boolean
  wecomWebhook?: string
  ntfyEnabled?: boolean
  ntfyUrl?: string
  ntfyTopic?: string
  ntfyToken?: string
  ntfyTokenMasked?: string
  notifyTaskToggles?: Record<string, boolean>
  notifyCooldownSec?: number
  adminIpAllowlist?: string[] | string
  routingFallbackUnitCost?: number
  proxyFirstByteTimeoutSec?: number
  tokenRouterFailureCooldownMaxSec?: number
  routingWeights?: RuntimeRoutingWeightsPayload
  proxyErrorKeywords?: string[] | string
  proxyEmptyContentFailEnabled?: boolean
  globalBlockedBrands?: string[]
  globalAllowedModels?: string[]
}

/**
 * ScheduleSpec v1 — semantic scheduling description. The backend keeps the
 * legacy `*_cron` field as the source of truth and derives/projects this
 * object; `custom` carries the original cron expression verbatim when a
 * semantic mapping is not possible.
 */
export type ScheduleSpecV1 =
  | { version: 1; kind: 'daily'; time: string }
  | { version: 1; kind: 'interval'; everyHours: number }
  | { version: 1; kind: 'window'; windowStart: string; windowEnd: string }
  | { version: 1; kind: 'custom'; cron: string }

type SettingsMigrationItem = {
  task: string
  legacyKey: string
  legacyValue: string
  v2Key: string
  schedule: ScheduleSpecV1
}

export type SettingsMigrationPreviewResponse = {
  success: boolean
  currentVersion: number
  targetVersion: number
  pending: number
  customCount: number
  legacyFieldsPreserved: boolean
  items: SettingsMigrationItem[]
}

export type SettingsMigrationApplyResponse =
  SettingsMigrationPreviewResponse & {
    applied: number
  }

export type ProxyLogStatusFilter = 'all' | 'success' | 'failed'
type ProxyLogClientConfidence = 'exact' | 'heuristic' | 'unknown' | null
type ProxyLogUsageSource = 'upstream' | 'self-log' | 'unknown' | null

export type ProxyLogBillingDetails = {
  quotaType: number
  usage: {
    promptTokens: number
    completionTokens: number
    totalTokens: number
    cacheReadTokens: number
    cacheCreationTokens: number
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
    totalCost: number
  }
} | null

export type ProxyLogListItem = {
  id: number
  createdAt: string
  modelRequested: string
  modelActual: string
  status: string
  httpStatus?: number | null
  latencyMs: number
  isStream?: boolean | null
  firstByteLatencyMs?: number | null
  totalTokens: number | null
  retryCount: number
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
  estimatedCost?: number | null
}

export type ProxyLogDetail = ProxyLogListItem & {
  routeId?: number | null
  channelId?: number | null
  httpStatus?: number | null
  billingDetails?: ProxyLogBillingDetails
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
  from?: string
  to?: string
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

type ProxyDebugTraceListItem = {
  id: number
  createdAt: string
  downstreamPath: string
  clientKind?: string | null
  sessionId?: string | null
  requestedModel?: string | null
  selectedChannelId?: number | null
  finalStatus?: string | null
  finalHttpStatus?: number | null
  finalUpstreamPath?: string | null
}

export type ProxyDebugTraceDetail = {
  trace: {
    id: number
    createdAt?: string | null
    updatedAt?: string | null
    downstreamPath?: string | null
    clientKind?: string | null
    sessionId?: string | null
    traceHint?: string | null
    requestedModel?: string | null
    stickySessionKey?: string | null
    stickyHitChannelId?: number | null
    selectedChannelId?: number | null
    selectedRouteId?: number | null
    selectedAccountId?: number | null
    selectedSiteId?: number | null
    selectedSitePlatform?: string | null
    endpointCandidatesJson?: string | null
    endpointRuntimeStateJson?: string | null
    decisionSummaryJson?: string | null
    requestHeadersJson?: string | null
    requestBodyJson?: string | null
    finalStatus?: string | null
    finalHttpStatus?: number | null
    finalUpstreamPath?: string | null
    finalResponseHeadersJson?: string | null
    finalResponseBodyJson?: string | null
  }
  attempts: Array<{
    id: number
    attemptIndex: number
    endpoint: string
    requestPath: string
    targetUrl: string
    runtimeExecutor?: string | null
    requestHeadersJson?: string | null
    requestBodyJson?: string | null
    responseStatus?: number | null
    responseHeadersJson?: string | null
    responseBodyJson?: string | null
    rawErrorText?: string | null
    recoverApplied?: boolean | null
    downgradeDecision?: boolean | null
    downgradeReason?: string | null
    memoryWriteJson?: string | null
    createdAt?: string | null
  }>
}

export type ProxyDebugTracesResponse = {
  items: ProxyDebugTraceListItem[]
}

export type OAuthProviderInfo = {
  provider: string
  label: string
  platform: string
  enabled: boolean
  loginType: 'oauth'
  requiresProjectId: boolean
  supportsDirectAccountRouting: boolean
  supportsCloudValidation: boolean
  supportsNativeProxy: boolean
}

export type OAuthProvidersResponse = {
  providers: OAuthProviderInfo[]
  defaults?: {
    systemProxyConfigured?: boolean
  }
}

export type OAuthRouteUnitStrategy = 'round_robin' | 'stick_until_unavailable'

type OAuthRouteUnitSummary = {
  id?: number
  routeUnitId?: number
  name: string
  strategy: OAuthRouteUnitStrategy
  memberCount: number
}

type OAuthRouteParticipation =
  | { kind: 'single' }
  | ({ kind: 'route_unit' } & OAuthRouteUnitSummary)

type OAuthStartInstructions = {
  redirectUri: string
  callbackPort: number
  callbackPath: string
  manualCallbackDelayMs: number
  sshTunnelCommand?: string
  sshTunnelKeyCommand?: string
}

export type OAuthStartResponse = {
  provider: string
  state: string
  authorizationUrl: string
  instructions: OAuthStartInstructions
}

export type OAuthSessionInfo = {
  provider: string
  state: string
  status: 'pending' | 'success' | 'error'
  accountId?: number
  siteId?: number
  error?: string
}

type OAuthQuotaWindowInfo = {
  supported: boolean
  limit?: number | null
  used?: number | null
  remaining?: number | null
  resetAt?: string | null
  message?: string | null
}

export type OAuthQuotaInfo = {
  status: 'supported' | 'unsupported' | 'error'
  source: 'official' | 'reverse_engineered'
  lastSyncAt?: string | null
  lastError?: string | null
  providerMessage?: string | null
  subscription?: {
    planType?: string | null
    activeStart?: string | null
    activeUntil?: string | null
  } | null
  windows: {
    fiveHour: OAuthQuotaWindowInfo
    sevenDay: OAuthQuotaWindowInfo
  }
  lastLimitResetAt?: string | null
}

export type OAuthConnectionInfo = {
  accountId: number
  siteId: number
  provider: string
  username?: string | null
  email?: string | null
  accountKey?: string | null
  planType?: string | null
  projectId?: string | null
  modelCount: number
  modelsPreview: string[]
  status: 'healthy' | 'abnormal'
  quota?: OAuthQuotaInfo | null
  routeChannelCount?: number
  lastModelSyncAt?: string | null
  lastModelSyncError?: string | null
  proxyUrl?: string | null
  useSystemProxy?: boolean
  routeUnit?: OAuthRouteUnitSummary | null
  routeParticipation?: OAuthRouteParticipation | null
  site?: { id: number; name: string; url: string; platform: string } | null
}

export type OAuthConnectionsResponse = {
  items: OAuthConnectionInfo[]
  total: number
  limit: number
  offset: number
}

export type OAuthQuotaBatchRefreshResponse = {
  success: boolean
  refreshed: number
  failed: number
  items: Array<{
    accountId: number
    success: boolean
    quota?: OAuthQuotaInfo
    error?: string
  }>
}

export type OAuthImportResponse = {
  success: boolean
  imported: number
  skipped: number
  failed: number
  items: Array<{
    name: string
    status: 'imported' | 'skipped' | 'failed'
    accountId?: number
    provider?: string
    message?: string
  }>
}

export type OAuthRouteUnitMutationResponse = {
  success: boolean
  routeUnit?: OAuthRouteUnitSummary
}

type DownstreamApiKeyTrendBucket = {
  startUtc: string | null
  totalRequests: number
  successRequests: number
  failedRequests: number
  successRate: number | null
  totalTokens: number
  totalCost: number
}

export type DownstreamApiKeyTrendResponse = {
  success: boolean
  range: '24h' | '7d' | 'all'
  item: {
    id: number
    name: string
  }
  bucketSeconds: number
  timeZone?: string | null
  buckets: DownstreamApiKeyTrendBucket[]
}

// ---------------------------------------------------------------------------
// Business API — 179 methods across 13 domain groups.
// Signatures preserved 1:1 from legacy api.ts; only the transport is swapped.
// ---------------------------------------------------------------------------
