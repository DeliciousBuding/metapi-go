/* eslint-disable no-nested-ternary -- timeout fallback uses chained ternary */
/**
 * metapi-go business API surface.
 *
 * This is a faithful, signature-preserving rewrite of the legacy
 * `web/api.ts` (179 methods across 13 domain groups). The transport layer is
 * swapped: every JSON method now flows through the shared axios instance
 * `apiClient` from `@/lib/http-client` (which owns auth injection, GET dedup,
 * business-error toasts, and 401 → refresh → replay), while the handful of
 * streaming / raw-Response / SSE endpoints keep using `fetchAuthenticatedResponse`
 * because they cannot pass through the axios JSON interceptors.
 *
 * Method names, parameters, URLs, HTTP verbs, bodies, and custom timeouts are
 * preserved 1:1 so downstream TanStack Query hooks and feature code keep
 * working without signature churn.
 *
 * TODO(types): ~29 parameter sites still use `any` (untyped backend payloads).
 * Replace with Zod schemas + `z.infer` types in a follow-up; left as `any`
 * for now to keep the rewrite signature-stable. Search for `: any` to find them.
 */

import { clearAuthSession, getAuthToken } from '@/lib/auth-session'
import {
  apiClient,
  extractResponseErrorMessage,
  fetchAuthenticatedResponse,
  type ApiRequestConfig,
} from '@/lib/http-client'

// ---------------------------------------------------------------------------
// Transport helpers
// ---------------------------------------------------------------------------

type RequestOptions = {
  method?: 'GET' | 'POST' | 'PUT' | 'DELETE' | 'PATCH'
  body?: string
  timeoutMs?: number
  signal?: AbortSignal
  headers?: Record<string, string>
  skipErrorHandler?: boolean
}

/**
 * Core JSON request helper. Routes GET through `apiClient.get` so the
 * http-client GET-dedup applies; non-GET goes through `apiClient.request`.
 * Returns the parsed response body, matching the legacy `res.json()` contract.
 */
async function request<T = any>(
  url: string,
  options: RequestOptions = {}
): Promise<T> {
  const {
    method = 'GET',
    body,
    timeoutMs = 30_000,
    signal,
    headers,
    skipErrorHandler = false,
  } = options

  const requestHeaders: Record<string, string> | undefined = body
    ? { 'Content-Type': 'application/json', ...headers }
    : headers

  const baseConfig: ApiRequestConfig = {
    timeout: timeoutMs,
    signal,
    headers: requestHeaders,
    skipErrorHandler,
  }

  const response =
    method === 'GET'
      ? await apiClient.get(url, baseConfig)
      : await apiClient.request({
          url,
          method,
          data: body,
          ...baseConfig,
        })

  return response.data as T
}

function buildQueryString(
  params?: Record<string, string | number | boolean | null | undefined>
) {
  if (!params) return ''
  const searchParams = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value === undefined || value === null || value === '') continue
    searchParams.set(key, String(value))
  }
  const serialized = searchParams.toString()
  return serialized ? `?${serialized}` : ''
}

function parseContentDispositionFilename(
  headerValue: string | null
): string | null {
  if (!headerValue) return null
  const utf8Match = /filename\*=UTF-8''([^;]+)/i.exec(headerValue)
  if (utf8Match?.[1]) {
    try {
      return decodeURIComponent(utf8Match[1])
    } catch {
      return utf8Match[1]
    }
  }
  const quotedMatch = /filename="([^"]+)"/i.exec(headerValue)
  if (quotedMatch?.[1]) return quotedMatch[1]
  const bareMatch = /filename=([^;]+)/i.exec(headerValue)
  return bareMatch?.[1]?.trim() || null
}

type BufferLike = {
  from(data: ArrayBuffer): { toString(encoding: 'base64'): string }
}

const nodeBuffer = (globalThis as typeof globalThis & { Buffer?: BufferLike })
  .Buffer

function arrayBufferToBase64(buffer: ArrayBuffer): string {
  if (nodeBuffer) {
    return nodeBuffer.from(buffer).toString('base64')
  }

  let binary = ''
  const bytes = new Uint8Array(buffer)
  const chunkSize = 0x8000
  for (let index = 0; index < bytes.length; index += chunkSize) {
    binary += String.fromCharCode(...bytes.subarray(index, index + chunkSize))
  }
  return btoa(binary)
}

async function streamSse(
  url: string,
  handlers: {
    onLog?: (entry: any) => void
    onDone?: (payload: any) => void
    signal?: AbortSignal
  }
) {
  const response = await fetchAuthenticatedResponse(url, {
    method: 'GET',
    signal: handlers.signal,
    headers: {
      Accept: 'text/event-stream',
    },
    timeoutMs: 120_000,
  })

  if (!response.ok) {
    throw new Error(await extractResponseErrorMessage(response))
  }
  if (!response.body) {
    throw new Error('响应未返回流式内容')
  }

  const decoder = new TextDecoder()
  const reader = response.body.getReader()
  let buffer = ''

  const flushBuffer = (final = false) => {
    const chunks = final ? [...buffer.split('\n\n'), ''] : buffer.split('\n\n')
    if (!final) buffer = chunks.pop() || ''
    else buffer = ''

    for (const chunk of chunks) {
      const lines = chunk.split('\n')
      let eventName = 'message'
      const dataLines: string[] = []

      for (const line of lines) {
        if (line.startsWith('event:')) {
          eventName = line.slice('event:'.length).trim() || 'message'
        } else if (line.startsWith('data:')) {
          dataLines.push(line.slice('data:'.length).trim())
        }
      }

      if (dataLines.length <= 0) continue
      let payload: any = dataLines.join('\n')
      try {
        payload = JSON.parse(payload)
      } catch {
        // keep string payload
      }

      if (eventName === 'log') {
        handlers.onLog?.(payload)
      } else if (eventName === 'done') {
        handlers.onDone?.(payload)
      }
    }
  }

  while (true) {
    const { done, value } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true })
    flushBuffer(false)
  }

  if (buffer.trim()) {
    flushBuffer(true)
  }
}

// ---------------------------------------------------------------------------
// Types — preserved verbatim from legacy api.ts
// ---------------------------------------------------------------------------

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

type RedirectFixCandidate = {
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

type TestChatRequestPayload = {
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

const DEFAULT_PROXY_TEST_TIMEOUT_MS = 30_000
const LONG_RUNNING_PROXY_TEST_TIMEOUT_MS = 150_000

function resolveProxyTestTimeoutMs(data: ProxyTestRequestEnvelope) {
  if (data.jobMode) return LONG_RUNNING_PROXY_TEST_TIMEOUT_MS
  if (data.path === '/v1/images/generations') {
    return LONG_RUNNING_PROXY_TEST_TIMEOUT_MS
  }
  if (data.path === '/v1/images/edits') {
    return LONG_RUNNING_PROXY_TEST_TIMEOUT_MS
  }
  if (data.path === '/v1/videos' && data.method === 'POST') {
    return LONG_RUNNING_PROXY_TEST_TIMEOUT_MS
  }
  return DEFAULT_PROXY_TEST_TIMEOUT_MS
}

function proxyTestRequest(data: ProxyTestRequestEnvelope) {
  return request('/api/test/proxy', {
    method: 'POST',
    body: JSON.stringify(data),
    timeoutMs: resolveProxyTestTimeoutMs(data),
  })
}

async function proxyTestStreamRequest(
  data: ProxyTestRequestEnvelope,
  signal?: AbortSignal
) {
  return fetchAuthenticatedResponse('/api/test/proxy/stream', {
    method: 'POST',
    signal,
    body: JSON.stringify(data),
    timeoutMs: resolveProxyTestTimeoutMs(data),
  })
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

export const api = {
  // Sites
  getSites: () => request('/api/sites'),
  addSite: (data: any) =>
    request('/api/sites', { method: 'POST', body: JSON.stringify(data) }),
  updateSite: (id: number, data: any) =>
    request(`/api/sites/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  deleteSite: (id: number) => request(`/api/sites/${id}`, { method: 'DELETE' }),
  batchUpdateSites: (data: any) =>
    request('/api/sites/batch', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  detectSite: (url: string) =>
    request('/api/sites/detect', {
      method: 'POST',
      body: JSON.stringify({ url }),
    }),
  getSiteDisabledModels: (siteId: number) =>
    request(`/api/sites/${siteId}/disabled-models`),
  updateSiteDisabledModels: (siteId: number, models: string[]) =>
    request(`/api/sites/${siteId}/disabled-models`, {
      method: 'PUT',
      body: JSON.stringify({ models }),
    }),
  getSiteAvailableModels: (siteId: number) =>
    request(`/api/sites/${siteId}/available-models`),
  probeSiteNow: (
    siteId: number,
    options?: {
      scope?: 'single' | 'all'
      modelName?: string
      latencyThresholdMs?: number
    }
  ) =>
    request(`/api/sites/${siteId}/probe-now`, {
      method: 'POST',
      body: JSON.stringify(options || {}),
      timeoutMs: options?.scope === 'all' ? 120_000 : 30_000,
    }),

  // Accounts
  getAccounts: async (params?: { includeOauth?: boolean }) => {
    const result = await request<any>(
      `/api/accounts${buildQueryString(params)}`
    )
    return Array.isArray(result?.accounts) ? result.accounts : result
  },
  getAccountsSnapshot: (options?: { refresh?: boolean }) =>
    request(
      `/api/accounts${buildQueryString(options?.refresh ? { refresh: 1 } : undefined)}`
    ) as Promise<{
      generatedAt: string
      accounts: any[]
      sites: any[]
    }>,
  addAccount: (data: any) =>
    request('/api/accounts', { method: 'POST', body: JSON.stringify(data) }),
  loginAccount: (data: {
    siteId: number
    username: string
    password: string
  }) =>
    request('/api/accounts/login', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  verifyToken: (data: {
    siteId: number
    accessToken: string
    platformUserId?: number
    credentialMode?: 'auto' | 'session' | 'apikey'
  }) =>
    request('/api/accounts/verify-token', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  rebindAccountSession: (
    id: number,
    data: {
      accessToken: string
      platformUserId?: number
      refreshToken?: string
      tokenExpiresAt?: number
    }
  ) =>
    request(`/api/accounts/${id}/rebind-session`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  updateAccount: (id: number, data: any) =>
    request(`/api/accounts/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  deleteAccount: (id: number) =>
    request(`/api/accounts/${id}`, { method: 'DELETE' }),
  batchUpdateAccounts: (data: any) =>
    request('/api/accounts/batch', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  refreshBalance: (id: number) =>
    request(`/api/accounts/${id}/balance`, { method: 'POST' }),
  getAccountModels: (id: number) => request(`/api/accounts/${id}/models`),
  addAccountAvailableModels: (accountId: number, models: string[]) =>
    request(`/api/accounts/${accountId}/models/manual`, {
      method: 'POST',
      body: JSON.stringify({ models }),
    }),
  refreshAccountHealth: (data?: { accountId?: number; wait?: boolean }) =>
    request('/api/accounts/health/refresh', {
      method: 'POST',
      body: JSON.stringify(data || {}),
      timeoutMs: data?.wait ? 150_000 : 30_000,
    }),

  // Account tokens
  getAccountTokens: (accountId?: number) =>
    request(`/api/account-tokens${accountId ? `?accountId=${accountId}` : ''}`),
  addAccountToken: (data: any) =>
    request('/api/account-tokens', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  updateAccountToken: (id: number, data: any) =>
    request(`/api/account-tokens/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  deleteAccountToken: (id: number) =>
    request(`/api/account-tokens/${id}`, { method: 'DELETE' }),
  batchUpdateAccountTokens: (data: any) =>
    request('/api/account-tokens/batch', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  getAccountTokenGroups: (accountId: number) =>
    request(`/api/account-tokens/groups/${accountId}`),
  setDefaultAccountToken: (id: number) =>
    request(`/api/account-tokens/${id}/default`, { method: 'POST' }),
  getAccountTokenValue: (id: number) =>
    request(`/api/account-tokens/${id}/value`),
  syncAccountTokens: (accountId: number) =>
    request(`/api/account-tokens/sync/${accountId}`, {
      method: 'POST',
      timeoutMs: 45_000,
    }),
  syncAllAccountTokens: (wait = false) =>
    request('/api/account-tokens/sync-all', {
      method: 'POST',
      body: JSON.stringify(wait ? { wait: true } : {}),
      timeoutMs: wait ? 150_000 : 30_000,
    }),

  // Check-in
  triggerCheckinAll: () => request('/api/checkin/trigger', { method: 'POST' }),
  triggerCheckin: (id: number) =>
    request(`/api/checkin/trigger/${id}`, { method: 'POST' }),
  getCheckinLogs: (params?: string) =>
    request(`/api/checkin/logs${params ? `?${params}` : ''}`),

  // Routes
  getRoutes: () => request('/api/routes'),
  getRoutesLite: () => request('/api/routes/lite'),
  getRoutesSummary: () => request('/api/routes/summary'),
  getRouteChannels: (routeId: number) =>
    request(`/api/routes/${routeId}/channels`),
  batchAddChannels: (
    routeId: number,
    channels: Array<{
      accountId: number
      tokenId?: number
      sourceModel?: string
    }>
  ) =>
    request(`/api/routes/${routeId}/channels/batch`, {
      method: 'POST',
      body: JSON.stringify({ channels }),
    }),
  addRoute: (data: any) =>
    request('/api/routes', { method: 'POST', body: JSON.stringify(data) }),
  updateRoute: (id: number, data: any) =>
    request(`/api/routes/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  deleteRoute: (id: number) =>
    request(`/api/routes/${id}`, { method: 'DELETE' }),
  clearRouteCooldown: (id: number) =>
    request(`/api/routes/${id}/cooldown/clear`, { method: 'POST' }),
  batchUpdateRoutes: (data: { ids: number[]; action: 'enable' | 'disable' }) =>
    request('/api/routes/batch', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  addChannel: (routeId: number, data: any) =>
    request(`/api/routes/${routeId}/channels`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  updateChannel: (id: number, data: any) =>
    request(`/api/channels/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  batchUpdateChannels: (updates: Array<{ id: number; priority: number }>) =>
    request('/api/channels/batch', {
      method: 'PUT',
      body: JSON.stringify({ updates }),
    }),
  deleteChannel: (id: number) =>
    request(`/api/channels/${id}`, { method: 'DELETE' }),
  rebuildRoutes: (refreshModels = true, wait = false) =>
    request('/api/routes/rebuild', {
      method: 'POST',
      body: JSON.stringify({
        refreshModels,
        ...(wait ? { wait: true } : {}),
      }),
      timeoutMs: wait ? 150_000 : 30_000,
    }),
  refreshRouteDecisionSnapshots: () =>
    request('/api/routes/decision/refresh', {
      method: 'POST',
      body: JSON.stringify({}),
    }),
  getRouteDecision: (model: string) =>
    request(`/api/routes/decision?model=${encodeURIComponent(model)}`),
  getRouteDecisionsBatch: (
    models: string[],
    options?: { refreshPricingCatalog?: boolean; persistSnapshots?: boolean }
  ) =>
    request('/api/routes/decision/batch', {
      method: 'POST',
      body: JSON.stringify({
        models,
        ...(options?.refreshPricingCatalog
          ? { refreshPricingCatalog: true }
          : {}),
        ...(options?.persistSnapshots ? { persistSnapshots: true } : {}),
      }),
    }),
  getRouteDecisionsByRouteBatch: (
    items: Array<{ routeId: number; model: string }>,
    options?: { refreshPricingCatalog?: boolean; persistSnapshots?: boolean }
  ) =>
    request('/api/routes/decision/by-route/batch', {
      method: 'POST',
      body: JSON.stringify({
        items,
        ...(options?.refreshPricingCatalog
          ? { refreshPricingCatalog: true }
          : {}),
        ...(options?.persistSnapshots ? { persistSnapshots: true } : {}),
      }),
    }),
  getRouteWideDecisionsBatch: (
    routeIds: number[],
    options?: { refreshPricingCatalog?: boolean; persistSnapshots?: boolean }
  ) =>
    request('/api/routes/decision/route-wide/batch', {
      method: 'POST',
      body: JSON.stringify({
        routeIds,
        ...(options?.refreshPricingCatalog
          ? { refreshPricingCatalog: true }
          : {}),
        ...(options?.persistSnapshots ? { persistSnapshots: true } : {}),
      }),
    }),

  // Stats
  getDashboard: () => request('/api/stats/dashboard'),
  getDashboardSnapshot: (options?: { refresh?: boolean }) =>
    request(
      `/api/stats/dashboard${buildQueryString({
        view: 'summary',
        ...(options?.refresh ? { refresh: 1 } : {}),
      })}`
    ),
  getDashboardInsights: (options?: { refresh?: boolean }) =>
    request(
      `/api/stats/dashboard${buildQueryString({
        view: 'insights',
        ...(options?.refresh ? { refresh: 1 } : {}),
      })}`
    ),
  getProxyLogs: (params?: ProxyLogsQuery) =>
    request(
      `/api/stats/proxy-logs${buildQueryString(params)}`
    ) as Promise<ProxyLogsResponse>,
  getProxyLogsQuery: (params?: ProxyLogsQuery) =>
    request(
      `/api/stats/proxy-logs${buildQueryString({ ...params, view: 'query' })}`
    ) as Promise<{
      items: ProxyLogsResponse['items']
      total: number
      page: number
      pageSize: number
    }>,
  getProxyLogsMeta: (
    params?: Omit<ProxyLogsQuery, 'limit' | 'offset'> & {
      refresh?: number | boolean
    }
  ) => {
    const refresh =
      params?.refresh === true
        ? 1
        : typeof params?.refresh === 'number'
          ? params.refresh
          : undefined
    const queryParams = {
      ...params,
      view: 'meta',
      ...(refresh !== undefined ? { refresh } : {}),
    } as Record<string, string | number | boolean | null | undefined>
    if (refresh === undefined) delete queryParams.refresh
    return request(
      `/api/stats/proxy-logs${buildQueryString(queryParams)}`
    ) as Promise<{
      clientOptions: ProxyLogsResponse['clientOptions']
      summary: ProxyLogsResponse['summary']
      sites: Array<{ id: number; name: string; status?: string | null }>
    }>
  },
  getProxyLogDetail: (id: number) =>
    request(`/api/stats/proxy-logs/${id}`) as Promise<ProxyLogDetail>,
  getProxyDebugTraces: (params?: { limit?: number }) =>
    request(
      `/api/stats/proxy-debug/traces${buildQueryString(params)}`
    ) as Promise<ProxyDebugTracesResponse>,
  getProxyDebugTraceDetail: (id: number) =>
    request(
      `/api/stats/proxy-debug/traces/${id}`
    ) as Promise<ProxyDebugTraceDetail>,
  checkModels: (accountId: number) =>
    request(`/api/models/check/${accountId}`, { method: 'POST' }),
  getSiteDistribution: () => request('/api/stats/site-distribution'),
  getSiteTrend: (days = 7) => request(`/api/stats/site-trend?days=${days}`),
  getBalanceHistory: (accountId: number, days = 30) =>
    request(`/api/stats/balance-history?accountId=${accountId}&days=${days}`),
  // A3: income vs outcome balance analysis.
  getBalanceIncomeOutcome: (days = 30) =>
    request(`/api/stats/balance-income-outcome?days=${days}`),
  // B1: admin write-operation audit log.
  getAdminAuditLogs: (params?: URLSearchParams) =>
    request(`/api/admin/audit-logs${params ? `?${params.toString()}` : ''}`),
  getAttention: (limit = 20) => request(`/api/stats/attention?limit=${limit}`),
  // A2: model cost distribution + latency chart gallery.
  getModelCostDistribution: (days = 30, topN = 8) =>
    request(
      `/api/stats/model-cost-distribution?days=${days}&topN=${topN}`
    ) as Promise<ModelCostDistributionResponse>,
  getLatencyHistogram: (days = 7, bucketMs = 500) =>
    request(
      `/api/stats/latency-histogram?days=${days}&bucketMs=${bucketMs}`
    ) as Promise<LatencyHistogramResponse>,
  getLatencyTrend: (days = 7) =>
    request(
      `/api/stats/latency-trend?days=${days}`
    ) as Promise<LatencyTrendResponse>,
  // G1: batch model verification + history.
  verifyModelsBatch: (models: string[], accountId = 0, limit = 50) =>
    request('/api/models/verify-batch', {
      method: 'POST',
      body: JSON.stringify({ models, accountId, limit }),
    }) as Promise<VerifyBatchResponse>,
  getModelVerifyHistory: (limit = 50, model = '') =>
    request(
      `/api/models/verify-history?limit=${limit}${model ? `&model=${encodeURIComponent(model)}` : ''}`
    ) as Promise<VerifyHistoryResponse>,
  // I1: accounts/sites global tag system.
  getTags: () => request('/api/tags') as Promise<TagIndexResponse>,
  updateAccountTags: (accountId: number, tags: string[]) =>
    request(`/api/accounts/${accountId}/tags`, {
      method: 'PUT',
      body: JSON.stringify({ tags }),
    }) as Promise<{ success: boolean; tags: string[] }>,
  updateSiteTags: (siteId: number, tags: string[]) =>
    request(`/api/sites/${siteId}/tags`, {
      method: 'PUT',
      body: JSON.stringify({ tags }),
    }) as Promise<{ success: boolean; tags: string[] }>,
  // H1: product risk banners.
  getActiveAnnouncements: () =>
    request('/api/announcements/active') as Promise<AnnouncementsResponse>,
  getAnnouncements: () =>
    request('/api/announcements') as Promise<AnnouncementsResponse>,
  createAnnouncement: (payload: {
    title: string
    message: string
    severity: Announcement['severity']
    link?: string | null
    enabled?: boolean
  }) =>
    request('/api/announcements', {
      method: 'POST',
      body: JSON.stringify(payload),
    }) as Promise<AnnouncementsResponse>,
  updateAnnouncement: (
    id: number,
    payload: {
      title: string
      message: string
      severity: Announcement['severity']
      link?: string | null
      enabled?: boolean
    }
  ) =>
    request(`/api/announcements/${id}`, {
      method: 'PUT',
      body: JSON.stringify(payload),
    }) as Promise<{ success: boolean; revision: boolean }>,
  deleteAnnouncement: (id: number) =>
    request(`/api/announcements/${id}`, {
      method: 'DELETE',
    }) as Promise<{ success: boolean }>,
  dismissAnnouncement: (id: number) =>
    request(`/api/announcements/${id}/dismiss`, {
      method: 'POST',
      body: '{}',
    }) as Promise<{ success: boolean }>,
  // K1a: model name redirects.
  getModelRedirects: (params?: { accountId?: number; source?: string }) =>
    request(
      `/api/model-redirects${buildQueryString(params)}`
    ) as Promise<ModelRedirectsResponse>,
  updateModelRedirect: (
    id: number,
    payload: { actual?: string; source?: string }
  ) =>
    request(`/api/model-redirects/${id}`, {
      method: 'PUT',
      body: JSON.stringify(payload),
    }) as Promise<{ success: boolean }>,
  deleteModelRedirect: (id: number) =>
    request(`/api/model-redirects/${id}`, {
      method: 'DELETE',
    }) as Promise<{ success: boolean }>,
  generateModelRedirects: (accountId = 0) =>
    request('/api/model-redirects/generate', {
      method: 'POST',
      body: JSON.stringify({ accountId }),
    }) as Promise<{
      success: boolean
      created: number
      accounts?: number
    }>,
  applyModelRedirects: (dryRun: boolean) =>
    request('/api/model-redirects/apply', {
      method: 'POST',
      body: JSON.stringify({ dryRun }),
    }) as Promise<RedirectApplyResponse>,
  // multiplier/rate overview (GET) + batch edit (PUT).
  getRateOverview: () =>
    request('/api/models/rates') as Promise<RateOverviewResponse>,
  // batch rate editing — unit_cost + weight only
  updateRates: (body: {
    accounts?: Array<{ id: number; unitCost: number }>
    channels?: Array<{ id: number; weight: number }>
  }) =>
    request<{
      success: boolean
      updatedAccounts: number
      updatedChannels: number
    }>('/api/models/rates', { method: 'PUT', body: JSON.stringify(body) }),
  // C1: unified recurring-scheduler run history.
  getSchedulerStatus: () =>
    request<{ items: SchedulerRunStatus[]; generatedAt: string }>(
      '/api/scheduler/status'
    ),
  getSiteSnapshot: async (days = 7, options?: { refresh?: boolean }) => {
    const query = buildQueryString({
      days,
      ...(options?.refresh ? { refresh: 1 } : {}),
    })
    const [distribution, trend, sites] = await Promise.all([
      request<{ distribution: any[] }>(`/api/stats/site-distribution${query}`),
      request<{ trend: any[] }>(`/api/stats/site-trend${query}`),
      request<any[]>('/api/sites'),
    ])
    return {
      generatedAt: new Date().toISOString(),
      distribution: Array.isArray(distribution?.distribution)
        ? distribution.distribution
        : [],
      trend: Array.isArray(trend?.trend) ? trend.trend : [],
      sites: Array.isArray(sites) ? sites : [],
    }
  },
  getModelBySite: (siteId?: number, days = 7) =>
    request(
      `/api/stats/model-by-site?${siteId ? `siteId=${siteId}&` : ''}days=${days}`
    ),

  // Search
  search: (query: string) =>
    request('/api/search', {
      method: 'POST',
      body: JSON.stringify({ query, limit: 20 }),
    }),

  // OAuth
  getOAuthProviders: () =>
    request('/api/oauth/providers') as Promise<OAuthProvidersResponse>,
  startOAuthProvider: (
    provider: string,
    data?: {
      accountId?: number
      projectId?: string
      proxyUrl?: string | null
      useSystemProxy?: boolean
    }
  ) =>
    request(`/api/oauth/providers/${encodeURIComponent(provider)}/start`, {
      method: 'POST',
      body: JSON.stringify(data || {}),
    }) as Promise<OAuthStartResponse>,
  getOAuthSession: (state: string) =>
    request(
      `/api/oauth/sessions/${encodeURIComponent(state)}`
    ) as Promise<OAuthSessionInfo>,
  submitOAuthManualCallback: (state: string, callbackUrl: string) =>
    request(
      `/api/oauth/sessions/${encodeURIComponent(state)}/manual-callback`,
      {
        method: 'POST',
        body: JSON.stringify({ callbackUrl }),
      }
    ) as Promise<{ success: true }>,
  getOAuthConnections: (params?: { limit?: number; offset?: number }) =>
    request(
      `/api/oauth/connections${buildQueryString(params)}`
    ) as Promise<OAuthConnectionsResponse>,
  refreshOAuthConnectionQuota: (accountId: number) =>
    request(`/api/oauth/connections/${accountId}/quota/refresh`, {
      method: 'POST',
      body: JSON.stringify({}),
    }) as Promise<{ success: true; quota: OAuthQuotaInfo }>,
  refreshOAuthConnectionQuotaBatch: (accountIds: number[]) =>
    request('/api/oauth/connections/quota/refresh-batch', {
      method: 'POST',
      body: JSON.stringify({ accountIds }),
    }) as Promise<OAuthQuotaBatchRefreshResponse>,
  updateOAuthConnectionProxy: (
    accountId: number,
    data: { proxyUrl?: string | null; useSystemProxy?: boolean }
  ) =>
    request(`/api/oauth/connections/${accountId}/proxy`, {
      method: 'PATCH',
      body: JSON.stringify(data || {}),
    }) as Promise<{ success: true }>,
  rebindOAuthConnection: (
    accountId: number,
    data?: { proxyUrl?: string | null; useSystemProxy?: boolean }
  ) =>
    request(`/api/oauth/connections/${accountId}/rebind`, {
      method: 'POST',
      body: JSON.stringify(data || {}),
    }) as Promise<OAuthStartResponse>,
  deleteOAuthConnection: (accountId: number) =>
    request(`/api/oauth/connections/${accountId}`, {
      method: 'DELETE',
    }) as Promise<{ success: true }>,
  importOAuthConnections: (data: Record<string, unknown>) =>
    request('/api/oauth/import', {
      method: 'POST',
      body: JSON.stringify(Array.isArray(data.items) ? data : { data }),
    }) as Promise<OAuthImportResponse>,
  createOAuthRouteUnit: (data: {
    accountIds: number[]
    name: string
    strategy: OAuthRouteUnitStrategy
  }) =>
    request('/api/oauth/route-units', {
      method: 'POST',
      body: JSON.stringify(data),
    }) as Promise<OAuthRouteUnitMutationResponse>,
  deleteOAuthRouteUnit: (routeUnitId: number) =>
    request(`/api/oauth/route-units/${routeUnitId}`, {
      method: 'DELETE',
    }) as Promise<{ success: true }>,

  // Events
  getEvents: (params?: string) =>
    request(`/api/events${params ? `?${params}` : ''}`),
  getEventCount: () => request('/api/events/count'),
  markEventRead: (id: number) =>
    request(`/api/events/${id}/read`, { method: 'POST' }),
  markAllEventsRead: () => request('/api/events/read-all', { method: 'POST' }),
  clearEvents: () => request('/api/events', { method: 'DELETE' }),
  getTasks: (limit = 50) =>
    request(
      `/api/tasks?limit=${Math.max(1, Math.min(200, Math.trunc(limit)))}`
    ),
  getTask: (id: string) => request(`/api/tasks/${encodeURIComponent(id)}`),

  // Auth management
  getAuthInfo: () => request('/api/settings/auth/info'),
  changeAuthToken: (oldToken: string, newToken: string) =>
    request('/api/settings/auth/change', {
      method: 'POST',
      body: JSON.stringify({ oldToken, newToken }),
      skipErrorHandler: true,
    }),
  getRuntimeSettings: () => request('/api/settings/runtime'),
  getBrandList: () => request('/api/settings/brand-list'),
  updateRuntimeSettings: (data: RuntimeSettingsPayload) =>
    request('/api/settings/runtime', {
      method: 'PUT',
      body: JSON.stringify(data),
      skipErrorHandler: true,
    }),
  getUpdateCenterStatus: () => request('/api/update-center/status'),
  saveUpdateCenterConfig: (data: any) =>
    request('/api/update-center/config', {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  checkUpdateCenter: () =>
    request('/api/update-center/check', {
      method: 'POST',
      body: JSON.stringify({}),
    }),
  deployUpdateCenter: (data: {
    source: 'github-release' | 'docker-hub-tag'
    targetTag: string
    targetDigest?: string | null
  }) =>
    request('/api/update-center/deploy', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  rollbackUpdateCenter: (data: { targetRevision: string }) =>
    request('/api/update-center/rollback', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  streamUpdateCenterTaskLogs: (
    taskId: string,
    handlers: {
      onLog?: (entry: any) => void
      onDone?: (payload: any) => void
      signal?: AbortSignal
    }
  ) =>
    streamSse(
      `/api/update-center/tasks/${encodeURIComponent(taskId)}/stream`,
      handlers
    ),
  testSystemProxy: (data: SystemProxyTestRequest) =>
    request('/api/settings/system-proxy/test', {
      method: 'POST',
      body: JSON.stringify(data),
      timeoutMs: 20_000,
      skipErrorHandler: true,
    }),
  getRuntimeDatabaseConfig: () => request('/api/settings/database/runtime'),
  updateRuntimeDatabaseConfig: (
    data: Partial<{
      dialect: 'sqlite' | 'postgres'
      connectionString: string
      ssl: boolean
    }>
  ) =>
    request('/api/settings/database/runtime', {
      method: 'PUT',
      body: JSON.stringify(data),
      skipErrorHandler: true,
    }),
  testExternalDatabaseConnection: (data: {
    dialect: 'sqlite' | 'postgres'
    connectionString: string
    ssl?: boolean
  }) =>
    request('/api/settings/database/test-connection', {
      method: 'POST',
      body: JSON.stringify(data),
      skipErrorHandler: true,
    }),
  migrateExternalDatabase: (data: {
    dialect: 'sqlite' | 'postgres'
    connectionString: string
    overwrite?: boolean
    ssl?: boolean
  }) =>
    request('/api/settings/database/migrate', {
      method: 'POST',
      body: JSON.stringify(data),
      timeoutMs: 120_000,
      skipErrorHandler: true,
    }),
  getDownstreamApiKeys: () => request('/api/downstream-keys'),
  createDownstreamApiKey: (data: any) =>
    request('/api/downstream-keys', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  updateDownstreamApiKey: (id: number, data: any) =>
    request(`/api/downstream-keys/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  deleteDownstreamApiKey: (id: number) =>
    request(`/api/downstream-keys/${id}`, { method: 'DELETE' }),
  batchDownstreamApiKeys: (data: {
    ids: number[]
    action: 'enable' | 'disable' | 'delete' | 'resetUsage' | 'updateMetadata'
    groupOperation?: 'keep' | 'set' | 'clear'
    groupName?: string
    tagOperation?: 'keep' | 'append'
    tags?: string[]
  }) =>
    request('/api/downstream-keys/batch', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  resetDownstreamApiKeyUsage: (id: number) =>
    request(`/api/downstream-keys/${id}/reset-usage`, { method: 'POST' }),
  getDownstreamApiKeysSummary: (params?: {
    range?: '24h' | '7d' | 'all'
    status?: 'all' | 'enabled' | 'disabled'
    search?: string
  }) => request(`/api/downstream-keys/summary${buildQueryString(params)}`),
  getDownstreamApiKeyOverview: (id: number) =>
    request(`/api/downstream-keys/${id}/overview`),
  getDownstreamApiKeyTrend: (
    id: number,
    params?: { range?: '24h' | '7d' | 'all'; timeZone?: string }
  ) =>
    request<DownstreamApiKeyTrendResponse>(
      `/api/downstream-keys/${id}/trend${buildQueryString(params)}`
    ),
  exportBackup: (type: 'all' | 'accounts' | 'preferences' = 'all') =>
    request(`/api/settings/backup/export?type=${encodeURIComponent(type)}`),
  /** Raw text export for file download; throws on non-OK responses. */
  exportBackupRaw: async (type: 'all' | 'accounts' | 'preferences' = 'all') => {
    const response = await fetchAuthenticatedResponse(
      `/api/settings/backup/export?type=${encodeURIComponent(type)}`
    )
    if (!response.ok) {
      throw new Error(await extractResponseErrorMessage(response))
    }
    return response.text()
  },
  /** Semantic schedule migration preview (one-click upgrade). */
  getSettingsMigrationPreview: () =>
    request<SettingsMigrationPreviewResponse>(
      '/api/settings/migration/preview'
    ),
  /** Apply the semantic schedule migration (append-only v2 keys). */
  applySettingsMigration: () =>
    request<SettingsMigrationApplyResponse>('/api/settings/migration/apply', {
      method: 'POST',
      body: JSON.stringify({}),
      skipErrorHandler: true,
    }),
  importBackup: (data: any) =>
    request('/api/settings/backup/import', {
      method: 'POST',
      body: JSON.stringify({ data }),
      skipErrorHandler: true,
    }),
  // F1: import plan preview before commit.
  previewBackupImport: (data: any) =>
    request('/api/settings/backup/import/preview', {
      method: 'POST',
      body: JSON.stringify({ data }),
      skipErrorHandler: true,
    }),
  getBackupWebdavConfig: () =>
    request<BackupWebdavResponse>('/api/settings/backup/webdav'),
  saveBackupWebdavConfig: (
    data: Partial<{
      enabled: boolean
      fileUrl: string
      username: string
      password?: string
      clearPassword?: boolean
      exportType: BackupWebdavExportType
      autoSyncEnabled: boolean
      autoSyncCron: string
    }>
  ) =>
    request<BackupWebdavResponse>('/api/settings/backup/webdav', {
      method: 'PUT',
      body: JSON.stringify(data),
      skipErrorHandler: true,
    }),
  exportBackupToWebdav: (type?: BackupWebdavExportType) =>
    request<BackupWebdavResponse>('/api/settings/backup/webdav/export', {
      method: 'POST',
      body: JSON.stringify(type ? { type } : {}),
      timeoutMs: 60_000,
      skipErrorHandler: true,
    }),
  importBackupFromWebdav: () =>
    request<BackupWebdavResponse>('/api/settings/backup/webdav/import', {
      method: 'POST',
      body: JSON.stringify({}),
      timeoutMs: 60_000,
      skipErrorHandler: true,
    }),
  clearRuntimeCache: () =>
    request('/api/settings/maintenance/clear-cache', {
      method: 'POST',
      skipErrorHandler: true,
    }),
  clearUsageData: () =>
    request('/api/settings/maintenance/clear-usage', {
      method: 'POST',
      skipErrorHandler: true,
    }),
  factoryReset: () =>
    request('/api/settings/maintenance/factory-reset', {
      method: 'POST',
      skipErrorHandler: true,
    }),
  testNotification: () =>
    request('/api/settings/notify/test', {
      method: 'POST',
      skipErrorHandler: true,
    }),

  // Monitor embed
  getMonitorConfig: () => request('/api/monitor/config'),
  updateMonitorConfig: (data: { ldohCookie?: string | null }) =>
    request('/api/monitor/config', {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  initMonitorSession: () => request('/api/monitor/session', { method: 'POST' }),
  // Clears the HttpOnly `meta_monitor_auth` cookie (Path=/monitor-proxy/);
  // must be called while Bearer auth is still valid.
  clearMonitorSession: () =>
    request('/api/monitor/session', { method: 'DELETE' }),

  // Models marketplace
  getModelsMarketplace: (options?: {
    refresh?: boolean
    includePricing?: boolean
  }) => {
    const params = new URLSearchParams()
    if (options?.refresh) params.set('refresh', '1')
    if (options?.includePricing) params.set('includePricing', '1')
    const query = params.toString()
    return request(`/api/models/marketplace${query ? `?${query}` : ''}`, {
      timeoutMs: options?.refresh ? 45_000 : 15_000,
    })
  },
  /** Cross-site effective model price comparison. */
  getModelPriceCompare: (options?: {
    model?: string
    days?: number
    limit?: number
    topModels?: number
  }) => {
    const params = new URLSearchParams()
    if (options?.model) params.set('model', options.model)
    if (options?.days != null) params.set('days', String(options.days))
    if (options?.limit != null) params.set('limit', String(options.limit))
    if (options?.topModels != null) {
      params.set('topModels', String(options.topModels))
    }
    const query = params.toString()
    return request(`/api/models/price-compare${query ? `?${query}` : ''}`, {
      timeoutMs: 20_000,
    })
  },
  getModelTokenCandidates: () => request('/api/models/token-candidates'),

  // Simple chat test from admin panel
  startTestChatJob: (data: TestChatRequestPayload) =>
    request('/api/test/chat/jobs', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  getTestChatJob: (jobId: string) =>
    request(`/api/test/chat/jobs/${encodeURIComponent(jobId)}`),
  deleteTestChatJob: (jobId: string) =>
    request(`/api/test/chat/jobs/${encodeURIComponent(jobId)}`, {
      method: 'DELETE',
    }),
  startProxyTestJob: (data: ProxyTestRequestEnvelope) =>
    request('/api/test/proxy/jobs', {
      method: 'POST',
      body: JSON.stringify(data),
      timeoutMs: resolveProxyTestTimeoutMs(data),
    }),
  getProxyTestJob: (jobId: string) =>
    request(`/api/test/proxy/jobs/${encodeURIComponent(jobId)}`),
  deleteProxyTestJob: (jobId: string) =>
    request(`/api/test/proxy/jobs/${encodeURIComponent(jobId)}`, {
      method: 'DELETE',
    }),
  getProxyFileContentDataUrl: async (
    fileId: string,
    options: Pick<RequestOptions, 'signal' | 'timeoutMs'> = {}
  ) => {
    const response = await fetchAuthenticatedResponse(
      `/v1/files/${encodeURIComponent(fileId)}/content`,
      {
        method: 'GET',
        ...options,
      }
    )
    if (!response.ok) {
      throw new Error(await extractResponseErrorMessage(response))
    }

    const mimeType =
      (response.headers.get('content-type') || 'application/octet-stream')
        .split(';')[0]
        .trim() || 'application/octet-stream'
    const filename = parseContentDispositionFilename(
      response.headers.get('content-disposition')
    )
    const base64 = arrayBufferToBase64(await response.arrayBuffer())
    return {
      filename,
      mimeType,
      data: `data:${mimeType};base64,${base64}`,
    }
  },
  testProxy: proxyTestRequest,
  proxyTest: proxyTestRequest,
  testChat: (data: TestChatRequestPayload) =>
    request('/api/test/chat', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  testProxyStream: proxyTestStreamRequest,
  proxyTestStream: proxyTestStreamRequest,
  testChatStream: async (
    data: TestChatRequestPayload,
    signal?: AbortSignal
  ) => {
    const token = getAuthToken(localStorage)
    if (!token) {
      clearAuthSession(localStorage)
      throw new Error('Session expired')
    }
    return fetch('/api/test/chat/stream', {
      method: 'POST',
      signal,
      headers: {
        Authorization: `Bearer ${token}`,
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(data),
    })
  },
}
