import { z } from 'zod'

import { request } from './transport'

const key = z.string().regex(/^[a-z][a-z0-9-]{0,63}$/)
const fingerprint = z.string().regex(/^[a-f0-9]{64}$/)
const versionSchema = z
  .object({
    key,
    fingerprint,
    available: z.boolean(),
    taskReferences: z.number().int().nonnegative().safe(),
  })
  .strict()
export type ChannelPluginVersion = z.infer<typeof versionSchema>
export const channelPluginVersionsQueryKey = [
  'channel-plugin-versions',
] as const
export const channelPluginVersionsApi = {
  list: async () => {
    const result = z
      .object({ items: z.array(versionSchema).max(1536) })
      .strict()
      .parse(await request<unknown>('/api/channel-plugin-versions'))
    const identities = result.items.map(
      (item) => `${item.key}:${item.fingerprint}`
    )
    if (new Set(identities).size !== identities.length)
      throw new Error('Invalid channel plugin version index')
    return result
  },
  restore: async (version: ChannelPluginVersion, source: string) => {
    const result = z
      .object({ key, fingerprint, restored: z.literal(true) })
      .strict()
      .parse(
        await request<unknown>('/api/channel-plugin-versions/restore', {
          method: 'POST',
          body: JSON.stringify({
            key: version.key,
            fingerprint: version.fingerprint,
            source,
          }),
        })
      )
    if (
      result.key !== version.key ||
      result.fingerprint !== version.fingerprint
    )
      throw new Error('Invalid channel plugin restore response')
    return result
  },
}
