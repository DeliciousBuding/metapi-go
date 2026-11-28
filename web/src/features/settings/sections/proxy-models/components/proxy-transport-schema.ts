import { z } from 'zod'

import {
  asBoolean,
  asString,
  joinListField,
  splitListField,
  type RuntimeSettings,
} from '../../../lib/runtime-settings'

// payloadRules is a Record<string, unknown> on the wire; the textarea holds a
// JSON string. Zod refines it to a parseable object so we can show a friendly
// error before PUT.
const payloadRulesSchema = z
  .string()
  .optional()
  .refine(
    (value) => {
      if (!value) {
        return true
      }
      try {
        const parsed = JSON.parse(value)
        return typeof parsed === 'object' && parsed !== null
      } catch {
        return false
      }
    },
    {
      message: 'settings.proxyModels.proxyTransport.schema.payloadRulesInvalid',
    }
  )

export const proxyTransportSchema = z.object({
  systemProxyUrl: z.string().optional(),
  proxyErrorKeywords: z.string().optional(),
  proxyEmptyContentFailEnabled: z.boolean(),
  payloadRules: payloadRulesSchema,
  codexUpstreamWebsocketEnabled: z.boolean(),
  modelAvailabilityProbeEnabled: z.boolean(),
})

export type ProxyTransportFormValues = z.infer<typeof proxyTransportSchema>

export const DEFAULT_VALUES: ProxyTransportFormValues = {
  systemProxyUrl: '',
  proxyErrorKeywords: '',
  proxyEmptyContentFailEnabled: false,
  payloadRules: '',
  codexUpstreamWebsocketEnabled: false,
  modelAvailabilityProbeEnabled: false,
}

export function deriveServerValues(
  data: RuntimeSettings | undefined
): ProxyTransportFormValues | null {
  if (!data) {
    return null
  }
  const payloadRulesRaw = data.payloadRules
  let payloadRulesJson = ''
  if (payloadRulesRaw && typeof payloadRulesRaw === 'object') {
    try {
      payloadRulesJson = JSON.stringify(payloadRulesRaw, null, 2)
    } catch {
      payloadRulesJson = ''
    }
  }
  return {
    systemProxyUrl: asString(data.systemProxyUrl),
    proxyErrorKeywords: joinListField(splitListField(data.proxyErrorKeywords)),
    proxyEmptyContentFailEnabled: asBoolean(data.proxyEmptyContentFailEnabled),
    payloadRules: payloadRulesJson,
    codexUpstreamWebsocketEnabled: asBoolean(
      data.codexUpstreamWebsocketEnabled
    ),
    modelAvailabilityProbeEnabled: asBoolean(
      data.modelAvailabilityProbeEnabled
    ),
  }
}

export function proxyTransportToPayload(
  changed: Partial<ProxyTransportFormValues>
) {
  const payload: Record<string, unknown> = {}
  if (changed.systemProxyUrl !== undefined) {
    payload.systemProxyUrl = changed.systemProxyUrl
  }
  if (changed.proxyErrorKeywords !== undefined) {
    payload.proxyErrorKeywords = splitListField(changed.proxyErrorKeywords)
  }
  if (changed.proxyEmptyContentFailEnabled !== undefined) {
    payload.proxyEmptyContentFailEnabled = changed.proxyEmptyContentFailEnabled
  }
  if (changed.payloadRules !== undefined) {
    payload.payloadRules = changed.payloadRules
      ? (JSON.parse(changed.payloadRules) as Record<string, unknown>)
      : null
  }
  if (changed.codexUpstreamWebsocketEnabled !== undefined) {
    payload.codexUpstreamWebsocketEnabled =
      changed.codexUpstreamWebsocketEnabled
  }
  if (changed.modelAvailabilityProbeEnabled !== undefined) {
    payload.modelAvailabilityProbeEnabled =
      changed.modelAvailabilityProbeEnabled
  }
  return payload
}
