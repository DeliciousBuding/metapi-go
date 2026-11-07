import { z } from 'zod'

import { importedEndpointKeys } from './imported-upstreams'
import { request } from './transport'

const protocol = z.enum([
  'chat',
  'responses',
  'messages',
  'gemini',
  'responsesCompact',
  'responsesWebsocket',
])
const key = z.string().regex(/^[a-z][a-z0-9-]{0,63}$/)
const providerProtocol = z.enum(importedEndpointKeys)
const observationPointer = z
  .string()
  .max(1024)
  .refine(
    (path) =>
      path.startsWith('/') &&
      path.split('/').length <= 65 &&
      !/[\r\n\0]/.test(path) &&
      !/~(?![01])/.test(path)
  )
const observation = z
  .object({
    usagePaths: z
      .partialRecord(
        z.enum([
          'prompt_tokens',
          'completion_tokens',
          'total_tokens',
          'input_tokens',
          'output_tokens',
          'cache_read_input_tokens',
          'cache_creation_input_tokens',
          'prompt_tokens_details.cached_tokens',
          'input_tokens_details.cached_tokens',
          'completion_tokens_details.reasoning_tokens',
          'output_tokens_details.reasoning_tokens',
          'promptTokenCount',
          'candidatesTokenCount',
          'totalTokenCount',
          'thoughtsTokenCount',
        ]),
        observationPointer
      )
      .optional(),
    modelPath: observationPointer.optional(),
    errorPath: observationPointer.optional(),
    taskIdPath: observationPointer.optional(),
  })
  .strict()
  .refine(
    (value) =>
      Object.keys(value.usagePaths ?? {}).length > 0 ||
      !!value.modelPath ||
      !!value.errorPath ||
      !!value.taskIdPath
  )
const protocolObservations = z
  .object({ response: observation.optional(), event: observation.optional() })
  .strict()
  .refine((value) => !!value.response || !!value.event)
const cursorParameter = z.string().regex(/^$|^[A-Za-z][A-Za-z0-9_-]{0,63}$/)
const templatePath = z
  .string()
  .max(8192)
  .refine((path) => {
    if (
      !path.startsWith('/') ||
      path.startsWith('//') ||
      /[?#\\\r\n\0]/.test(path)
    )
      return false
    try {
      const decoded = decodeURIComponent(path)
      return (
        !/[\\\r\n\0]/.test(decoded) &&
        !decoded.split('/').some((part) => part === '.' || part === '..')
      )
    } catch {
      return false
    }
  })
const templateAuth = z.enum(['bearer', 'x-api-key', 'x-goog-api-key', 'none'])
const imageTaskProtocol = z.enum([
  'imageGeneration',
  'imageEdit',
  'imageVariation',
  'modelscopeImageGeneration',
])
const imageQueryPath = templatePath.refine((path) => {
  if (new TextEncoder().encode(path).length > 8192) return false
  try {
    const segments = decodeURIComponent(path).split('/')
    return (
      path.split('{taskId}').length === 2 &&
      segments.filter((segment) => segment === '{taskId}').length === 1 &&
      segments.every(
        (segment) => !/[{}]/.test(segment) || segment === '{taskId}'
      ) &&
      !/[?#]/.test(decodeURIComponent(path)) &&
      !/\p{Cc}/u.test(decodeURIComponent(path))
    )
  } catch {
    return false
  }
})
const imageQueryUrl = z
  .string()
  .max(9216)
  .refine((value) => {
    if (new TextEncoder().encode(value).length > 9216) return false
    if (!URL.canParse(value)) return false
    const url = new URL(value)
    const path = value.match(/^https?:\/\/[^/?#]+(\/[^?#]*)$/i)?.[1]
    try {
      return (
        ['http:', 'https:'].includes(url.protocol) &&
        !url.username &&
        !url.password &&
        !url.search &&
        !url.hash &&
        path !== undefined &&
        imageQueryPath.safeParse(decodeURIComponent(path)).success
      )
    } catch {
      return false
    }
  }, 'channels.upstream.invalidUrl')
const authenticationClaim = z.string().regex(/^[A-Za-z_][A-Za-z0-9_-]{0,63}$/)
const authentication = z
  .object({
    type: z.enum(['api_key', 'jwt_hs256']),
    header: z
      .string()
      .min(1)
      .max(128)
      .regex(/^[!#$%&'*+.^_`|~0-9A-Za-z-]+$/)
      .refine((header) => {
        const name = header.toLowerCase()
        return (
          ![
            'sec-',
            'proxy-',
            'x-forwarded-',
            'x-codex-',
            'x-claude-code-',
          ].some((prefix) => name.startsWith(prefix)) &&
          ![
            'host',
            'cookie',
            'set-cookie',
            'connection',
            'transfer-encoding',
            'content-type',
            'content-length',
            'content-encoding',
            'accept-encoding',
            'accept',
            'te',
            'trailer',
            'upgrade',
            'forwarded',
            'x-real-ip',
            'session-id',
            'conversation_id',
            'conversation-id',
            'thread-id',
            'chatgpt-account-id',
            'anthropic-version',
            'anthropic-beta',
            'openai-beta',
            'openai-organization',
            'openai-project',
            'x-request-id',
            'x-client-request-id',
            'request-id',
            'traceparent',
            'tracestate',
            'baggage',
          ].includes(name)
        )
      }),
    prefix: z
      .string()
      .refine((value) => new TextEncoder().encode(value).length <= 256)
      .refine((value) => !/\p{Cc}/u.test(value))
      .optional(),
    jwt: z
      .object({
        keySeparator: z.enum(['.', '|']),
        issuerClaim: authenticationClaim.refine(
          (claim) => !['exp', 'iat', 'nbf'].includes(claim)
        ),
        ttlSeconds: z.number().int().min(60).max(86400),
        timeUnit: z.enum(['seconds', 'milliseconds']),
        issuedAtClaim: authenticationClaim.optional(),
        notBeforeSkewSeconds: z.number().int().min(0).max(300).optional(),
        signType: z.enum(['', 'SIGN']).optional(),
      })
      .strict()
      .refine(
        (jwt) =>
          !jwt.issuedAtClaim ||
          ![jwt.issuerClaim, 'exp', 'nbf'].includes(jwt.issuedAtClaim)
      )
      .optional(),
  })
  .strict()
  .refine((value) => (value.type === 'jwt_hs256') === !!value.jwt)
const endpointTemplate = z
  .object({
    path: templatePath,
    auth: templateAuth,
    modelPath: z.boolean().optional(),
    profile: z.string().max(64).optional(),
  })
  .strict()
const catalogTemplate = z
  .object({
    path: templatePath,
    auth: templateAuth,
    cursorParameter: cursorParameter.optional(),
  })
  .strict()
const option = z
  .object({
    key,
    label: z.string().min(1),
    type: z.enum(['boolean', 'string', 'select', 'number', 'integer']),
    default: z.union([z.string(), z.boolean(), z.number().finite()]),
    choices: z.array(z.string()).optional(),
    min: z.number().finite().optional(),
    max: z.number().finite().optional(),
  })
  .refine((option) => {
    if (option.type === 'boolean') return typeof option.default === 'boolean'
    if (option.type === 'string') return typeof option.default === 'string'
    if (option.type === 'select')
      return (
        typeof option.default === 'string' &&
        !!option.choices?.includes(option.default)
      )
    return (
      typeof option.default === 'number' &&
      option.min !== undefined &&
      option.max !== undefined &&
      option.min <= option.default &&
      option.default <= option.max &&
      (option.type !== 'integer' || Number.isInteger(option.default))
    )
  })
const manifestFields = {
  key,
  name: z.string().min(1),
  version: z.string().min(1),
  options: z.array(option).max(32).optional(),
}
const wireManifest = z
  .object({
    ...manifestFields,
    apiVersion: z.literal(1),
    protocols: z.array(protocol).min(1).max(6),
    hooks: z
      .array(z.enum(['request', 'response', 'event']))
      .min(1)
      .max(3),
  })
  .strict()
const providerManifest = z
  .object({
    ...manifestFields,
    apiVersion: z.literal(2),
    kind: z.literal('provider'),
    taskCapture: z.literal('durable').optional(),
    protocols: z.array(providerProtocol).min(1).max(26),
    credentialKinds: z
      .array(z.enum(['api_key', 'oauth', 'gcp_service_account', 'none']))
      .min(1)
      .max(4),
    requestKinds: z
      .array(z.enum(['json', 'multipart']))
      .min(1)
      .max(2)
      .optional(),
    authentication: authentication.optional(),
    imageTasks: z
      .partialRecord(
        imageTaskProtocol,
        z
          .object({
            query: z
              .object({
                path: imageQueryPath,
                auth: templateAuth,
                modelPath: z.literal(false).optional(),
                profile: z.literal('').optional(),
              })
              .strict(),
          })
          .strict()
      )
      .refine((tasks) => Object.keys(tasks).length > 0)
      .optional(),
    hooks: z
      .array(
        z.enum([
          'request',
          'response',
          'event',
          'decodeResponse',
          'decodeEvent',
          'modelsRequest',
          'modelsResponse',
          'submit',
          'query',
          'content',
          'taskResult',
        ])
      )
      .min(1)
      .max(11),
    observations: z
      .partialRecord(providerProtocol, protocolObservations)
      .optional(),
    defaults: z
      .object({
        endpoints: z
          .partialRecord(providerProtocol, endpointTemplate)
          .optional(),
        catalog: catalogTemplate.optional(),
      })
      .strict()
      .optional(),
  })
  .strict()
  .refine((manifest) => {
    if (
      manifest.taskCapture &&
      !manifest.protocols.some((protocol) =>
        ['video', 'seedanceVideo', 'zenmuxVideo'].includes(protocol)
      )
    )
      return false
    const hooks: readonly string[] = manifest.hooks
    if (
      manifest.authentication &&
      (manifest.credentialKinds.length !== 1 ||
        manifest.credentialKinds[0] !== 'api_key')
    )
      return false
    if (
      new Set(manifest.protocols).size !== manifest.protocols.length ||
      new Set(hooks).size !== hooks.length ||
      new Set(manifest.credentialKinds).size !== manifest.credentialKinds.length
    )
      return false
    if (
      new Set(manifest.requestKinds ?? []).size !==
      (manifest.requestKinds ?? []).length
    )
      return false
    if (hooks.includes('modelsRequest') !== hooks.includes('modelsResponse'))
      return false
    if (manifest.defaults?.catalog && !hooks.includes('modelsRequest'))
      return false
    const imageTasks = Object.keys(manifest.imageTasks ?? {})
    if (
      imageTasks.some(
        (key) =>
          !manifest.protocols.includes(
            key as (typeof importedEndpointKeys)[number]
          ) ||
          !manifest.observations?.[key as (typeof importedEndpointKeys)[number]]
            ?.response?.taskIdPath
      )
    )
      return false
    if (
      Object.keys(manifest.observations ?? {}).some(
        (key) =>
          !manifest.protocols.includes(
            key as (typeof importedEndpointKeys)[number]
          )
      )
    )
      return false
    if (
      Object.keys(manifest.defaults?.endpoints ?? {}).some(
        (key) =>
          !manifest.protocols.includes(
            key as (typeof importedEndpointKeys)[number]
          )
      )
    )
      return false
    return manifest.protocols.every((protocol) => {
      const required = imageTasks.includes(protocol)
        ? ['submit', 'query', 'taskResult']
        : protocol === 'responsesWebsocket'
          ? ['request', 'event']
          : ['video', 'seedanceVideo', 'zenmuxVideo'].includes(protocol)
            ? ['submit', 'query', 'content', 'taskResult']
            : ['request', 'response']
      return required.every((hook) => hooks.includes(hook))
    })
  })
const itemSchema = z
  .object({
    key,
    revision: z.number().int().positive(),
    enabled: z.boolean(),
    manifest: z.union([wireManifest, providerManifest]).optional(),
    loadError: z.string().min(1).optional(),
  })
  .refine((item) =>
    item.manifest
      ? item.manifest.key === item.key && !item.loadError
      : !!item.loadError
  )

export type ChannelPlugin = z.infer<typeof itemSchema>
export type ChannelPluginOption = z.infer<typeof option>
export const pluginBindingSchema = z.object({
  key,
  protocols: z.array(protocol).min(1).max(6),
  options: z
    .record(z.string(), z.union([z.string(), z.boolean(), z.number().finite()]))
    .optional(),
})
export type PluginBinding = z.infer<typeof pluginBindingSchema>
export const providerBindingSchema = z
  .object({
    key,
    protocols: z.array(providerProtocol).min(1).max(26),
    options: pluginBindingSchema.shape.options,
    imageQueries: z
      .partialRecord(
        imageTaskProtocol,
        z.object({ url: imageQueryUrl, auth: templateAuth }).strict()
      )
      .optional(),
    catalog: z
      .object({
        url: z
          .string()
          .url('channels.upstream.invalidUrl')
          .refine((value) => {
            if (!URL.canParse(value)) return false
            const url = new URL(value)
            return (
              ['http:', 'https:'].includes(url.protocol) &&
              !url.username &&
              !url.password &&
              !url.search &&
              !url.hash
            )
          }, 'channels.upstream.invalidUrl'),
        auth: templateAuth,
        cursorParameter: cursorParameter.optional(),
      })
      .strict()
      .optional(),
  })
  .strict()
  .refine(
    (binding) =>
      new Set(binding.protocols).size === binding.protocols.length &&
      Object.keys(binding.imageQueries ?? {}).every((protocol) =>
        binding.protocols.includes(
          protocol as (typeof importedEndpointKeys)[number]
        )
      )
  )
export type ProviderBinding = z.infer<typeof providerBindingSchema>
export const channelPluginQueryKey = ['channel-plugins'] as const

async function commitPlugin(
  pluginKey: string,
  method: 'PUT' | 'PATCH',
  input: { enabled: boolean; expectedRevision: number; source?: string }
) {
  const result = z
    .object({
      key,
      revision: z.number().int().positive(),
      enabled: z.boolean(),
    })
    .parse(
      await request<unknown>(
        `/api/channel-plugins/${encodeURIComponent(pluginKey)}`,
        { method, body: JSON.stringify(input) }
      )
    )
  if (
    result.key !== pluginKey ||
    result.revision !== input.expectedRevision + 1 ||
    result.enabled !== input.enabled
  )
    throw new Error('Invalid channel plugin save response')
  return result
}

export const channelPluginsApi = {
  list: async () =>
    z
      .object({ items: z.array(itemSchema) })
      .parse(await request<unknown>('/api/channel-plugins')),
  detail: async (pluginKey: string, signal?: AbortSignal) => {
    const result = z
      .object({
        key,
        source: z.string(),
        revision: z.number().int().positive(),
        enabled: z.boolean(),
      })
      .parse(
        await request<unknown>(
          `/api/channel-plugins/${encodeURIComponent(pluginKey)}`,
          { signal }
        )
      )
    if (result.key !== pluginKey)
      throw new Error('Invalid channel plugin detail response')
    return result
  },
  save: (
    pluginKey: string,
    input: { source: string; enabled: boolean; expectedRevision: number }
  ) => commitPlugin(pluginKey, 'PUT', input),
  setEnabled: (item: ChannelPlugin, enabled: boolean) =>
    commitPlugin(item.key, 'PATCH', {
      enabled,
      expectedRevision: item.revision,
    }),
  remove: async (item: ChannelPlugin) =>
    z.object({ success: z.literal(true) }).parse(
      await request<unknown>(
        `/api/channel-plugins/${encodeURIComponent(item.key)}`,
        {
          method: 'DELETE',
          body: JSON.stringify({ expectedRevision: item.revision }),
        }
      )
    ),
}
