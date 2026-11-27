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
  serverAddress?: string
  systemProxyUrl?: string
  payloadRules?: Record<string, unknown> | null
  modelAvailabilityProbeEnabled?: boolean
  codexUpstreamWebsocketEnabled?: boolean
  disableCrossProtocolFallback?: boolean
  proxySessionChannelConcurrencyLimit?: number
  proxySessionChannelQueueWaitMs?: number
  checkinEnabled?: boolean
  checkinCron?: string
  checkinScheduleMode?: 'cron' | 'interval' | 'window'
  checkinIntervalHours?: number
  checkinWindowStart?: string
  checkinWindowEnd?: string
  checkinSchedule?: ScheduleSpecV1
  balanceRefreshEnabled?: boolean
  balanceRefreshCron?: string
  balanceRefreshSchedule?: ScheduleSpecV1
  modelSyncCron?: string
  autoCreateModelRoutes?: boolean
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
  proxyRetryStatusRanges?: string
  proxyDisableStatusRanges?: string
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
