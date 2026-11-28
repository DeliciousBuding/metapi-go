import { z } from 'zod'

import type { ImportedEndpointConfig } from '@/lib/api/imported-upstreams'

import { upstreamProtocols, type EndpointProfile } from './upstream-protocols'

export const httpUrl = z
  .string()
  .url('channels.upstream.invalidUrl')
  .refine(
    (value) => /^https?:\/\//i.test(value),
    'channels.upstream.invalidUrl'
  )
export const modelWireUrlSchema = httpUrl.refine((value) => {
  if (!URL.canParse(value)) return false
  const url = new URL(value)
  return !url.username && !url.password && !url.search && !url.hash
}, 'channels.upstream.invalidUrl')
// Profile-to-endpoint constraints match store/direct_endpoints.go. The picker
// also respects provider restrictions for adapters such as Codex and OpenCode Go.
const profileEndpoints: Record<
  EndpointProfile,
  readonly (keyof ImportedEndpointConfig)[]
> = {
  codex: ['responses', 'responsesWebsocket'],
  claudecode: ['messages'],
  'xai-subscription': ['responses'],
  'github-copilot': ['chat'],
  deepseek: ['chat'],
  zai: ['chat'],
  bailian: ['chat'],
  cline: ['chat'],
  moonshot: ['chat'],
  longcat: ['chat'],
  openrouter: ['chat'],
  cerebras: ['chat'],
  nanogpt: ['chat'],
  'openrouter-image': ['imageGeneration', 'imageEdit'],
  'opencode-go': ['chat'],
  'jina-embeddings': ['jinaEmbeddings'],
  'minimax-image': ['imageGeneration'],
  'modelscope-image': [
    'imageGeneration',
    'imageEdit',
    'modelscopeImageGeneration',
  ],
  'codex-image': ['imageGeneration', 'imageEdit'],
  ollama: ['ollama'],
  'ollama-messages': ['messages'],
  bedrock: ['messages'],
  vertex: ['gemini', 'geminiEmbeddings'],
  antigravity: ['gemini'],
  'vertex-anthropic': ['messages'],
  'seedance-video': ['seedanceVideo'],
  'zenmux-video': ['zenmuxVideo'],
  'codex-alpha-search': ['alphaSearch'],
}
const endpoint = z.object({
  url: z.string().url('channels.upstream.invalidUrl'),
  auth: z.enum(['bearer', 'x-api-key', 'x-goog-api-key', 'none']),
  profile: z
    .enum(
      Object.keys(profileEndpoints) as [EndpointProfile, ...EndpointProfile[]]
    )
    .optional(),
  modelPath: z.boolean().optional(),
  requestModel: z.string().max(255).optional(),
  modelWireUrls: z
    .object({
      responses: modelWireUrlSchema,
      messages: modelWireUrlSchema,
    })
    .strict()
    .optional(),
})

export const endpointConfigSchema = z
  .object(
    Object.fromEntries(
      upstreamProtocols.map((protocol) => [
        protocol.key,
        endpoint
          .refine((value) => {
            const websocket = protocol.key === 'responsesWebsocket'
            if (
              !(websocket ? /^wss?:\/\//i : /^https?:\/\//i).test(value.url)
            ) {
              return false
            }
            if (
              protocol.authModes &&
              !protocol.authModes.includes(value.auth)
            ) {
              return false
            }
            if (
              protocol.requiredProfile &&
              value.profile !== protocol.requiredProfile
            ) {
              return false
            }
            const optionalAuth =
              value.profile === 'ollama' || value.profile === 'ollama-messages'
            if (value.auth === 'none' && !optionalAuth) return false
            if (
              value.profile &&
              ((value.auth !== 'bearer' &&
                !(optionalAuth && value.auth === 'none')) ||
                !profileEndpoints[value.profile].includes(protocol.key))
            ) {
              return false
            }
            if (value.profile === 'antigravity' && value.modelPath) {
              return false
            }
            if (
              value.modelPath &&
              protocol.key !== 'gemini' &&
              protocol.key !== 'geminiEmbeddings' &&
              !(
                protocol.key === 'messages' &&
                (value.profile === 'bedrock' ||
                  value.profile === 'vertex-anthropic')
              )
            ) {
              return false
            }
            if (
              ['bedrock', 'vertex', 'vertex-anthropic'].includes(
                value.profile ?? ''
              ) &&
              !value.modelPath
            ) {
              return false
            }
            if (value.profile === 'opencode-go' && !value.modelWireUrls) {
              return false
            }
            if (value.modelWireUrls && value.profile !== 'opencode-go') {
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
