import { z } from 'zod'

import {
  asBoolean,
  asNumber,
  asString,
  type RuntimeSettings,
} from '../../../lib/runtime-settings'

export const notifySchema = z.object({
  notifyCooldownSec: z.coerce.number().int().min(0),
  webhookEnabled: z.boolean(),
  webhookUrl: z.string().optional(),
  barkEnabled: z.boolean(),
  barkUrl: z.string().optional(),
  serverChanEnabled: z.boolean(),
  serverChanKey: z.string().optional(),
  telegramEnabled: z.boolean(),
  telegramUseSystemProxy: z.boolean(),
  telegramApiBaseUrl: z.string().optional(),
  telegramBotToken: z.string().optional(),
  telegramChatId: z.string().optional(),
  telegramMessageThreadId: z.string().optional(),
  smtpEnabled: z.boolean(),
  smtpHost: z.string().optional(),
  smtpPort: z.coerce.number().int(),
  smtpSecure: z.boolean(),
  smtpUser: z.string().optional(),
  smtpPass: z.string().optional(),
  smtpFrom: z.string().optional(),
  smtpTo: z.string().optional(),
  feishuEnabled: z.boolean(),
  feishuWebhook: z.string().optional(),
  feishuSecret: z.string().optional(),
  dingtalkEnabled: z.boolean(),
  dingtalkWebhook: z.string().optional(),
  dingtalkSecret: z.string().optional(),
  wecomEnabled: z.boolean(),
  wecomWebhook: z.string().optional(),
  ntfyEnabled: z.boolean(),
  ntfyUrl: z.string().optional(),
  ntfyTopic: z.string().optional(),
  ntfyToken: z.string().optional(),
  muteTokenExpired: z.boolean(),
  muteLowBalance: z.boolean(),
  muteProxyAllFailed: z.boolean(),
})

export type NotifyFormValues = z.infer<typeof notifySchema>

const SECRET_FIELDS = [
  'serverChanKey',
  'telegramBotToken',
  'smtpPass',
  'feishuSecret',
  'dingtalkSecret',
  'ntfyToken',
] as const

export type SecretFieldName = (typeof SECRET_FIELDS)[number]

export const DEFAULT_VALUES: NotifyFormValues = {
  notifyCooldownSec: 300,
  webhookEnabled: false,
  webhookUrl: '',
  barkEnabled: false,
  barkUrl: '',
  serverChanEnabled: false,
  serverChanKey: '',
  telegramEnabled: false,
  telegramUseSystemProxy: false,
  telegramApiBaseUrl: 'https://api.telegram.org',
  telegramBotToken: '',
  telegramChatId: '',
  telegramMessageThreadId: '',
  smtpEnabled: false,
  smtpHost: '',
  smtpPort: 587,
  smtpSecure: false,
  smtpUser: '',
  smtpPass: '',
  smtpFrom: '',
  smtpTo: '',
  feishuEnabled: false,
  feishuWebhook: '',
  feishuSecret: '',
  dingtalkEnabled: false,
  dingtalkWebhook: '',
  dingtalkSecret: '',
  wecomEnabled: false,
  wecomWebhook: '',
  ntfyEnabled: false,
  ntfyUrl: 'https://ntfy.sh',
  ntfyTopic: '',
  ntfyToken: '',
  muteTokenExpired: false,
  muteLowBalance: false,
  muteProxyAllFailed: false,
}

export function deriveServerValues(
  data: RuntimeSettings | undefined
): NotifyFormValues | null {
  if (!data) {
    return null
  }
  const toggles = (data.notifyTaskToggles ?? {}) as Record<string, unknown>
  return {
    notifyCooldownSec: asNumber(data.notifyCooldownSec) ?? 300,
    webhookEnabled: asBoolean(data.webhookEnabled),
    webhookUrl: asString(data.webhookUrl),
    barkEnabled: asBoolean(data.barkEnabled),
    barkUrl: asString(data.barkUrl),
    serverChanEnabled: asBoolean(data.serverChanEnabled),
    serverChanKey: '',
    telegramEnabled: asBoolean(data.telegramEnabled),
    telegramUseSystemProxy: asBoolean(data.telegramUseSystemProxy),
    telegramApiBaseUrl:
      asString(data.telegramApiBaseUrl) || 'https://api.telegram.org',
    telegramBotToken: '',
    telegramChatId: asString(data.telegramChatId),
    telegramMessageThreadId: asString(data.telegramMessageThreadId),
    smtpEnabled: asBoolean(data.smtpEnabled),
    smtpHost: asString(data.smtpHost),
    smtpPort: asNumber(data.smtpPort) ?? 587,
    smtpSecure: asBoolean(data.smtpSecure),
    smtpUser: asString(data.smtpUser),
    smtpPass: '',
    smtpFrom: asString(data.smtpFrom),
    smtpTo: asString(data.smtpTo),
    feishuEnabled: asBoolean(data.feishuEnabled),
    feishuWebhook: asString(data.feishuWebhook),
    feishuSecret: '',
    dingtalkEnabled: asBoolean(data.dingtalkEnabled),
    dingtalkWebhook: asString(data.dingtalkWebhook),
    dingtalkSecret: '',
    wecomEnabled: asBoolean(data.wecomEnabled),
    wecomWebhook: asString(data.wecomWebhook),
    ntfyEnabled: asBoolean(data.ntfyEnabled),
    ntfyUrl: asString(data.ntfyUrl) || 'https://ntfy.sh',
    ntfyTopic: asString(data.ntfyTopic),
    ntfyToken: '',
    muteTokenExpired: asBoolean(toggles.token_expired),
    muteLowBalance: asBoolean(toggles.low_balance),
    muteProxyAllFailed: asBoolean(toggles.proxy_all_failed),
  }
}

export function notificationsToPayload(
  changed: Partial<NotifyFormValues>,
  baseline: NotifyFormValues | null
): Record<string, unknown> {
  const payload: Record<string, unknown> = {}
  for (const [key, value] of Object.entries(changed)) {
    if (SECRET_FIELDS.includes(key as SecretFieldName)) {
      // Blank secret = keep stored; never submit a masked value.
      if (value) {
        payload[key] = value
      }
      continue
    }
    payload[key] = value
  }
  // Mute toggles are stored as notifyTaskToggles; submit the whole map when
  // any mute field changed.
  const muteChanged =
    changed.muteTokenExpired !== undefined ||
    changed.muteLowBalance !== undefined ||
    changed.muteProxyAllFailed !== undefined
  if (muteChanged) {
    const current = { ...(baseline ?? DEFAULT_VALUES), ...changed }
    payload.notifyTaskToggles = {
      token_expired: current.muteTokenExpired,
      low_balance: current.muteLowBalance,
      proxy_all_failed: current.muteProxyAllFailed,
    }
  }
  delete payload.muteTokenExpired
  delete payload.muteLowBalance
  delete payload.muteProxyAllFailed
  return payload
}
