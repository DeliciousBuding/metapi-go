import { z } from 'zod'

import { request } from './transport'

const protocol = z.enum(['chat', 'responses', 'messages', 'gemini'])
const key = z.string().regex(/^[a-z][a-z0-9-]{0,63}$/)
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
const itemSchema = z
  .object({
    key,
    revision: z.number().int().positive(),
    enabled: z.boolean(),
    manifest: z
      .object({
        apiVersion: z.literal(1),
        key,
        name: z.string().min(1),
        version: z.string().min(1),
        protocols: z.array(protocol).min(1),
        hooks: z.array(z.enum(['request', 'response', 'event'])).min(1),
        options: z.array(option).optional(),
      })
      .optional(),
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
  protocols: z.array(protocol).min(1).max(4),
  options: z
    .record(z.string(), z.union([z.string(), z.boolean(), z.number().finite()]))
    .optional(),
})
export type PluginBinding = z.infer<typeof pluginBindingSchema>
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
