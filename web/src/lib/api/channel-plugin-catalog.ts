import { z } from 'zod'

import { key } from './channel-plugin-endpoints'
import { wireManifest, providerManifest } from './channel-plugin-manifest'
import { request } from './transport'

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

export const channelPluginCatalog = {
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
}
