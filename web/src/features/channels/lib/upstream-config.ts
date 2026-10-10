import { t } from 'i18next'
import { z } from 'zod'

import type {
  ImportedEndpoint,
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

export const upstreamProtocolGroups = [
  'conversation',
  'retrieval',
  'image',
  'audio',
  'video',
] as const

// Persisted bits and keys follow store/direct_protocols.go in the same order.
// All endpoint, grant and member controls derive their capability list here.
type EndpointProfile = NonNullable<ImportedEndpoint['profile']>
type UpstreamProtocol = {
  key: keyof ImportedEndpointConfig
  bit: number
  group: (typeof upstreamProtocolGroups)[number]
  convertible: boolean
  requiredProfile?: EndpointProfile
}
export const upstreamProtocols: readonly UpstreamProtocol[] = [
  { key: 'chat', bit: 2, group: 'conversation', convertible: true },
  { key: 'responses', bit: 4, group: 'conversation', convertible: true },
  { key: 'messages', bit: 8, group: 'conversation', convertible: true },
  { key: 'gemini', bit: 16, group: 'conversation', convertible: true },
  { key: 'completions', bit: 32, group: 'conversation', convertible: false },
  { key: 'embeddings', bit: 64, group: 'retrieval', convertible: false },
  { key: 'rerank', bit: 128, group: 'retrieval', convertible: false },
  { key: 'imageGeneration', bit: 256, group: 'image', convertible: false },
  { key: 'imageEdit', bit: 512, group: 'image', convertible: false },
  { key: 'imageVariation', bit: 1024, group: 'image', convertible: false },
  { key: 'audioSpeech', bit: 2048, group: 'audio', convertible: false },
  { key: 'audioTranscription', bit: 4096, group: 'audio', convertible: false },
  { key: 'audioTranslation', bit: 8192, group: 'audio', convertible: false },
  { key: 'moderations', bit: 16384, group: 'conversation', convertible: false },
  { key: 'video', bit: 32768, group: 'video', convertible: false },
  {
    key: 'geminiEmbeddings',
    bit: 65536,
    group: 'retrieval',
    convertible: false,
  },
  {
    key: 'jinaEmbeddings',
    bit: 131072,
    group: 'retrieval',
    convertible: false,
    requiredProfile: 'jina-embeddings',
  },
  {
    key: 'modelscopeImageGeneration',
    bit: 262144,
    group: 'image',
    convertible: false,
    requiredProfile: 'modelscope-image',
  },
]

export function memberProtocols(member: ImportedMember) {
  const bits = member.protocolOrder?.length
    ? member.protocolOrder
    : upstreamProtocols
        .filter((p) => member.protocols & p.bit)
        .map((p) => p.bit)
  const protocols = bits.flatMap((bit) => {
    const protocol = upstreamProtocols.find((p) => p.bit === bit)
    return protocol ? [protocol] : []
  })
  const conversation = protocols
    .filter((p) => p.convertible)
    .map((p) => t(`channels.capabilities.names.${p.key}`))
    .join(' → ')
  const exact = protocols
    .filter((p) => !p.convertible)
    .map((p) => t(`channels.capabilities.names.${p.key}`))
    .join(' · ')
  return [conversation, exact].filter(Boolean).join(' · ')
}

const httpUrl = z
  .string()
  .url('channels.upstream.invalidUrl')
  .refine(
    (value) => /^https?:\/\//i.test(value),
    'channels.upstream.invalidUrl'
  )
// Profile-to-endpoint constraints match store/direct_endpoints.go. The provider
// picker below keeps wire formats independent of provider branding; only the
// Codex OAuth adapter is limited to its credential providers.
const profileEndpoints: Record<
  EndpointProfile,
  readonly (keyof ImportedEndpointConfig)[]
> = {
  codex: ['responses'],
  claudecode: ['messages'],
  deepseek: ['chat'],
  zai: ['chat'],
  'jina-embeddings': ['jinaEmbeddings'],
  'minimax-image': ['imageGeneration'],
  'modelscope-image': [
    'imageGeneration',
    'imageEdit',
    'modelscopeImageGeneration',
  ],
  'codex-image': ['imageGeneration', 'imageEdit'],
}
const endpoint = z.object({
  url: httpUrl,
  auth: z.enum(['bearer', 'x-api-key', 'x-goog-api-key']),
  profile: z
    .enum(
      Object.keys(profileEndpoints) as [EndpointProfile, ...EndpointProfile[]]
    )
    .optional(),
  modelPath: z.boolean().optional(),
  requestModel: z.string().max(255).optional(),
})

const endpointConfigSchema = z
  .object(
    Object.fromEntries(
      upstreamProtocols.map((protocol) => [
        protocol.key,
        endpoint
          .refine((value) => {
            if (
              protocol.requiredProfile &&
              value.profile !== protocol.requiredProfile
            ) {
              return false
            }
            if (
              value.profile &&
              (value.auth !== 'bearer' ||
                !profileEndpoints[value.profile].includes(protocol.key))
            ) {
              return false
            }
            if (
              value.modelPath &&
              protocol.key !== 'gemini' &&
              protocol.key !== 'geminiEmbeddings'
            ) {
              return false
            }
            if (value.profile === 'codex-image') {
              return !!value.requestModel?.trim()
            }
            return !value.requestModel
          }, 'channels.upstream.invalidEndpoints')
          .optional(),
      ])
    )
  )
  .strict()

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
  const required = upstreamProtocols.find(
    (p) => p.key === protocol
  )?.requiredProfile
  if (required) return [required]
  if (protocol === 'imageGeneration' || protocol === 'imageEdit') {
    const profiles =
      protocol === 'imageGeneration'
        ? ['minimax-image', 'modelscope-image']
        : ['modelscope-image']
    if (['codex', 'fenno'].includes(provider)) profiles.push('codex-image')
    return profiles
  }
  if (protocol === 'chat') return ['deepseek', 'zai']
  if (protocol === 'responses' && ['codex', 'fenno'].includes(provider)) {
    return ['codex']
  }
  if (protocol === 'messages' && provider === 'claudecode') {
    return ['claudecode']
  }
  return []
}
