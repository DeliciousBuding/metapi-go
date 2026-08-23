// metapi-go/features/settings/lib — TanStack Query hooks wrapping the
// runtime-settings endpoint (GET/PUT /api/settings/runtime) plus the settings
// migration endpoints (GET preview / POST apply). All sections share the
// single runtime-settings cache entry; writes invalidate it so every mounted
// section refetches the merged server state.

import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from '@tanstack/react-query'

import {
  api,
  type RuntimeSettingsPayload,
  type ScheduleSpecV1,
  type SettingsMigrationApplyResponse,
} from '@/lib/api'

/**
 * Strongly-typed read view of GET /api/settings/runtime. The backend returns
 * branding/site keys and masked secret fields alongside the payload keys; all
 * of them are enumerated here so sections never fall back to an untyped bag.
 */
export type RuntimeSettings = {
  // site & branding
  systemName?: string
  logo?: string
  footer?: string
  about?: string
  serverAddress?: string
  // authentication
  currentAdminIp?: string
  adminIpAllowlist?: string[] | string
  // scheduling
  checkinCron?: string
  checkinScheduleMode?: string
  checkinIntervalHours?: number
  checkinWindowStart?: string
  checkinWindowEnd?: string
  checkinSchedule?: ScheduleSpecV1
  balanceRefreshCron?: string
  balanceRefreshSchedule?: ScheduleSpecV1
  logCleanupCron?: string
  logCleanupSchedule?: ScheduleSpecV1
  logCleanupRetentionDays?: number
  logCleanupUsageLogsEnabled?: boolean
  logCleanupProgramLogsEnabled?: boolean
  // proxy transport
  systemProxyUrl?: string
  proxyErrorKeywords?: string[] | string
  proxyEmptyContentFailEnabled?: boolean
  payloadRules?: Record<string, unknown>
  codexUpstreamWebsocketEnabled?: boolean
  responsesCompactFallbackToResponsesEnabled?: boolean
  proxySessionChannelConcurrencyLimit?: number
  proxySessionChannelQueueWaitMs?: number
  modelAvailabilityProbeEnabled?: boolean
  // routing
  routingFallbackUnitCost?: number
  tokenRouterFailureCooldownMaxSec?: number
  proxyFirstByteTimeoutSec?: number
  disableCrossProtocolFallback?: boolean
  routingWeights?: {
    baseWeightFactor?: number
    valueScoreFactor?: number
    costWeight?: number
    balanceWeight?: number
    usageWeight?: number
  }
  // notifications
  notifyCooldownSec?: number
  webhookUrl?: string
  webhookEnabled?: boolean
  barkUrl?: string
  barkEnabled?: boolean
  serverChanEnabled?: boolean
  serverChanKey?: string
  serverChanKeyMasked?: string
  telegramEnabled?: boolean
  telegramApiBaseUrl?: string
  telegramBotToken?: string
  telegramBotTokenMasked?: string
  telegramChatId?: string
  telegramUseSystemProxy?: boolean
  telegramMessageThreadId?: string
  smtpEnabled?: boolean
  smtpHost?: string
  smtpPort?: number
  smtpSecure?: boolean
  smtpUser?: string
  smtpPass?: string
  smtpPassMasked?: string
  smtpFrom?: string
  smtpTo?: string
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
  // models / allowlist
  globalBlockedBrands?: string[] | string
  globalAllowedModels?: string[] | string
  // downstream
  proxyTokenMasked?: string
}

export const runtimeSettingsQueryKeys = {
  all: ['runtime-settings'] as const,
  detail: () => [...runtimeSettingsQueryKeys.all, 'detail'] as const,
}

const settingsMigrationQueryKeys = {
  all: ['settings-migration'] as const,
  preview: () => [...settingsMigrationQueryKeys.all, 'preview'] as const,
}

/**
 * Read the merged runtime-settings document. All mounted sections share this
 * cache entry; `useUpdateRuntimeSettings` invalidates it after writes.
 */
export function useRuntimeSettings(
  options?: Omit<UseQueryOptions<RuntimeSettings>, 'queryKey' | 'queryFn'>
) {
  return useQuery({
    queryKey: runtimeSettingsQueryKeys.detail(),
    queryFn: async () => {
      const data = await api.getRuntimeSettings()
      return (data ?? {}) as RuntimeSettings
    },
    staleTime: 15 * 1000,
    ...options,
  })
}

/**
 * Partial update against PUT /api/settings/runtime. The backend accepts a
 * partial payload, so callers pass only the keys they own. On success the
 * shared runtime-settings cache is invalidated so every mounted section
 * refetches the new server state.
 */
export function useUpdateRuntimeSettings() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (payload: Partial<RuntimeSettingsPayload>) =>
      api.updateRuntimeSettings(payload),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: runtimeSettingsQueryKeys.all,
      })
    },
  })
}

/** Read the one-click schedule migration preview. */
export function useSettingsMigrationPreview() {
  return useQuery({
    queryKey: settingsMigrationQueryKeys.preview(),
    queryFn: async () => api.getSettingsMigrationPreview(),
    staleTime: 30 * 1000,
  })
}

/** Apply the one-click schedule migration, then refresh the preview + runtime. */
export function useApplySettingsMigration() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async () =>
      api.applySettingsMigration() as Promise<SettingsMigrationApplyResponse>,
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: settingsMigrationQueryKeys.all,
      })
      void queryClient.invalidateQueries({
        queryKey: runtimeSettingsQueryKeys.all,
      })
    },
  })
}

/**
 * Split a runtime list field that the backend serializes as either a
 * newline/comma string or a string[]. Returns a clean string[] with empty
 * entries removed. Used by adminIpAllowlist, proxyErrorKeywords,
 * globalBlockedBrands, globalAllowedModels, etc.
 */
export function splitListField(value: unknown): string[] {
  if (Array.isArray(value)) {
    return value.map((item) => String(item).trim()).filter(Boolean)
  }
  if (typeof value === 'string') {
    return value
      .split(/\r?\n|,/)
      .map((item) => item.trim())
      .filter(Boolean)
  }
  return []
}

/**
 * Join a string[] back into the newline-separated form expected by the
 * textarea inputs (adminIpAllowlist / proxyErrorKeywords).
 */
export function joinListField(items: readonly string[]): string {
  return items.join('\n')
}

/** Coerce an unknown runtime value to a string (empty string when missing). */
export function asString(value: unknown): string {
  if (value === null || value === undefined) {
    return ''
  }
  return String(value)
}

/** Coerce an unknown runtime value to a number (NaN→undefined for Zod). */
export function asNumber(value: unknown): number | undefined {
  if (value === null || value === undefined || value === '') {
    return undefined
  }
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : undefined
}

/** Coerce an unknown runtime value to a boolean. */
export function asBoolean(value: unknown): boolean {
  return Boolean(value)
}
