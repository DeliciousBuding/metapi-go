import { z } from 'zod'

import type { ChannelPlugin } from './channel-plugin-catalog'
import { key } from './channel-plugin-endpoints'
import { request } from './transport'

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

export const channelPluginManagement = {
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
