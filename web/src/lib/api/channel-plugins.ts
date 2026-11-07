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
const templateAuth = z.enum(['bearer', 'x-api-key', 'x-goog-api-key'])
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
    protocols: z.array(providerProtocol).min(1).max(26),
    credentialKinds: z
      .array(z.enum(['api_key', 'oauth', 'gcp_service_account']))
      .min(1)
      .max(3),
    hooks: z
      .array(
        z.enum([
          'request',
          'response',
          'event',
          'modelsRequest',
          'modelsResponse',
          'submit',
          'query',
          'content',
          'taskResult',
        ])
      )
      .min(1)
      .max(9),
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
    const hooks: readonly string[] = manifest.hooks
    if (
      new Set(manifest.protocols).size !== manifest.protocols.length ||
      new Set(hooks).size !== hooks.length ||
      new Set(manifest.credentialKinds).size !== manifest.credentialKinds.length
    )
      return false
    if (hooks.includes('modelsRequest') !== hooks.includes('modelsResponse'))
      return false
    if (manifest.defaults?.catalog && !hooks.includes('modelsRequest'))
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
      const required =
        protocol === 'responsesWebsocket'
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
    (binding) => new Set(binding.protocols).size === binding.protocols.length
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
