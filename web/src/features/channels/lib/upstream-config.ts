import { z } from 'zod'

import type {
  ImportedEndpointConfig,
  ImportedMember,
  ImportedUpstreamDetail,
  ImportedUpstreamUpdate,
} from '@/lib/api/imported-upstreams'
import {
  isValidMapObject,
  parseStringMap,
  serializeStringMap,
} from '@/lib/helpers/string-map'

export const upstreamKeys = {
  all: ['imported-upstreams'] as const,
  detail: (id: number) => ['imported-upstreams', id] as const,
  credentials: (id: number) =>
    ['imported-upstreams', id, 'credentials'] as const,
  models: (id: number) => ['imported-upstreams', id, 'models'] as const,
  groups: ['imported-upstreams', 'groups'] as const,
}

export const upstreamProtocols = [
  { key: 'chat', bit: 2, name: 'Chat' },
  { key: 'responses', bit: 4, name: 'Responses' },
  { key: 'messages', bit: 8, name: 'Messages' },
  { key: 'gemini', bit: 16, name: 'Gemini' },
] as const

export function memberProtocols(member: ImportedMember) {
  const order = member.protocolOrder?.length
    ? member.protocolOrder
    : upstreamProtocols
        .filter((p) => member.protocols & p.bit)
        .map((p) => p.bit)
  return order
    .map((bit) => upstreamProtocols.find((p) => p.bit === bit)?.name)
    .filter(Boolean)
    .join(' → ')
}

const httpUrl = z
  .string()
  .url('channels.upstream.invalidUrl')
  .refine(
    (value) => /^https?:\/\//i.test(value),
    'channels.upstream.invalidUrl'
  )
const endpoint = z.object({
  url: httpUrl,
  auth: z.enum(['bearer', 'x-api-key', 'x-goog-api-key']),
  profile: z.enum(['codex', 'claudecode', 'deepseek', 'zai']).optional(),
  modelPath: z.boolean().optional(),
})

const endpointConfigSchema = z.object({
  chat: endpoint.optional(),
  responses: endpoint.optional(),
  messages: endpoint.optional(),
  gemini: endpoint.optional(),
})

export const connectionSchema = z.object({
  name: z.string().trim().min(1, 'channels.upstream.required'),
  baseUrl: httpUrl,
  endpointConfig: z.custom<ImportedEndpointConfig>(
    (value) => endpointConfigSchema.safeParse(value).success,
    'channels.upstream.invalidEndpoints'
  ),
  useSystemProxy: z.boolean(),
  openaiChatCompletionPath: z.string(),
  openaiResponsePath: z.string(),
  anthropicMessagePath: z.string(),
  channelProxy: z.string(),
  customHeaders: z
    .string()
    .refine(
      (value) => parseStringMap(value) !== null,
      'channels.upstream.invalidHeaders'
    ),
  paramOverride: z
    .string()
    .refine(isValidMapObject, 'channels.upstream.invalidObject'),
})
export type ConnectionValues = z.infer<typeof connectionSchema>

export function connectionValues(
  detail: ImportedUpstreamDetail
): ConnectionValues {
  return {
    name: detail.name,
    baseUrl: detail.baseUrl,
    endpointConfig: detail.endpointConfig ?? {},
    useSystemProxy: detail.useSystemProxy,
    openaiChatCompletionPath: detail.openaiChatCompletionPath ?? '',
    openaiResponsePath: detail.openaiResponsePath ?? '',
    anthropicMessagePath: detail.anthropicMessagePath ?? '',
    channelProxy: '',
    customHeaders: '',
    paramOverride: '',
  }
}

// Ordered header rows use the shared string-map editor without collapsing
// duplicate keys or changing the server's array contract.
export function headersForEditor(raw: string): string {
  if (!raw.trim()) return ''
  const rows: Array<{ header_key: string; header_value: string }> =
    JSON.parse(raw)
  return serializeStringMap(
    rows.map((row) => ({ key: row.header_key, value: row.header_value }))
  )
}

function headersForRequest(value: string): string {
  const entries = parseStringMap(value)
  if (!entries) throw new Error('Invalid header rows')
  return entries.length
    ? JSON.stringify(
        entries.map((row) => ({ header_key: row.key, header_value: row.value }))
      )
    : ''
}

export function connectionPatch(
  values: ConnectionValues,
  dirty: Partial<Record<keyof ConnectionValues, unknown>>
): ImportedUpstreamUpdate {
  const patch: ImportedUpstreamUpdate = {}
  for (const key of Object.keys(dirty) as Array<keyof ConnectionValues>) {
    if (!dirty[key]) continue
    Object.assign(patch, {
      [key]:
        key === 'customHeaders'
          ? headersForRequest(values.customHeaders)
          : values[key],
    })
  }
  return patch
}

export function availableProfiles(
  protocol: keyof ImportedEndpointConfig,
  provider: string
): string[] {
  if (protocol === 'chat') return ['deepseek', 'zai']
  if (protocol === 'responses' && ['codex', 'fenno'].includes(provider)) {
    return ['codex']
  }
  if (protocol === 'messages' && provider === 'claudecode') {
    return ['claudecode']
  }
  return []
}
