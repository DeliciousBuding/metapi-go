import { t } from 'i18next'

import type {
  ImportedEndpoint,
  ImportedEndpointConfig,
  ImportedMember,
} from '@/lib/api/imported-upstreams'

export const upstreamProtocolGroups = [
  'conversation',
  'retrieval',
  'image',
  'audio',
  'video',
  'native',
] as const

// Persisted bits and keys follow store/direct_protocols.go in the same order.
// All endpoint, grant and member controls derive their capability list here.
export type EndpointProfile = NonNullable<ImportedEndpoint['profile']>
type UpstreamProtocol = {
  key: keyof ImportedEndpointConfig
  bit: number
  group: (typeof upstreamProtocolGroups)[number]
  convertible: boolean
  authModes?: readonly ImportedEndpoint['auth'][]
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
  {
    key: 'seedanceVideo',
    bit: 524288,
    group: 'video',
    convertible: false,
    requiredProfile: 'seedance-video',
  },
  {
    key: 'zenmuxVideo',
    bit: 1048576,
    group: 'video',
    convertible: false,
    requiredProfile: 'zenmux-video',
  },
  {
    key: 'ollama',
    bit: 2097152,
    group: 'conversation',
    convertible: true,
    requiredProfile: 'ollama',
  },
  { key: 'systemOne', bit: 4194304, group: 'native', convertible: false },
  { key: 'alphaSearch', bit: 8388608, group: 'retrieval', convertible: false },
  {
    key: 'responsesCompact',
    bit: 16777216,
    group: 'native',
    convertible: false,
    authModes: ['bearer'],
  },
  {
    key: 'responsesWebsocket',
    bit: 33554432,
    group: 'native',
    convertible: false,
    authModes: ['bearer'],
  },
  {
    key: 'decisions',
    bit: 67108864,
    group: 'native',
    convertible: false,
    authModes: ['bearer'],
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
  const video = protocols
    .filter((p) => p.group === 'video')
    .map((p) => t(`channels.capabilities.names.${p.key}`))
    .join(' → ')
  const exact = protocols
    .filter((p) => !p.convertible && p.group !== 'video')
    .map((p) => t(`channels.capabilities.names.${p.key}`))
    .join(' · ')
  return [conversation, video, exact].filter(Boolean).join(' · ')
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
        ? ['openrouter-image', 'minimax-image', 'modelscope-image']
        : ['openrouter-image', 'modelscope-image']
    if (['codex', 'fenno'].includes(provider)) profiles.push('codex-image')
    return profiles
  }
  if (protocol === 'chat') {
    const profiles = [
      'bailian',
      'deepseek',
      'moonshot',
      'zai',
      'longcat',
      'openrouter',
      'cerebras',
      'nanogpt',
      'cline',
      'github-copilot',
    ]
    if (provider === 'opencode_go') profiles.push('opencode-go')
    return profiles
  }
  if (protocol === 'responses') {
    return [
      ...(['codex', 'fenno'].includes(provider) ? ['codex'] : []),
      'xai-subscription',
    ]
  }
  if (protocol === 'responsesWebsocket') {
    return ['codex', 'fenno'].includes(provider) ? ['codex'] : []
  }
  if (protocol === 'messages') {
    return [
      ...(provider === 'claudecode' ? ['claudecode'] : []),
      'bedrock',
      'ollama-messages',
      'vertex-anthropic',
    ]
  }
  if (protocol === 'gemini' || protocol === 'geminiEmbeddings') {
    return protocol === 'gemini' ? ['vertex', 'antigravity'] : ['vertex']
  }
  if (protocol === 'alphaSearch' && ['codex', 'fenno'].includes(provider)) {
    return ['codex-alpha-search']
  }
  return []
}
