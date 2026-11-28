import { z } from 'zod'

import {
  pluginBindingSchema,
  providerBindingSchema,
  type ProviderBinding,
} from '@/lib/api/channel-plugins'
import type { ImportedEndpointConfig } from '@/lib/api/imported-upstreams'
import { isValidMapObject, parseStringMap } from '@/lib/helpers/string-map'

import { httpUrl, endpointConfigSchema } from './upstream-endpoint-schema'

export const connectionSchema = z
  .object({
    pluginBindings: z.array(pluginBindingSchema).max(8).optional(),
    providerBinding: z.custom<ProviderBinding | null | undefined>(
      (value) =>
        providerBindingSchema.nullable().optional().safeParse(value).success,
      'channels.plugins.invalidProviderBinding'
    ),
    streamPolicy: z.enum(['unlimited', 'require', 'forbid']),
    rateLimit: z.object({
      rpm: channelLimitNumber(),
      tpm: channelLimitNumber(),
      maxConcurrent: channelLimitNumber(),
      queueSize: channelLimitNumber(),
      // Matches channellimit.Config.Validate's int64 time.Duration bound.
      queueTimeoutMs: channelLimitNumber().max(
        9_223_372_036_854,
        'channels.upstream.invalidQueueTimeout'
      ),
    }),
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
  .superRefine((value, ctx) => {
    const error = providerEndpointError(
      value.providerBinding,
      value.endpointConfig
    )
    if (error)
      ctx.addIssue({
        code: 'custom',
        path: ['providerBinding'],
        message: error,
      })
  })
export type ConnectionValues = z.infer<typeof connectionSchema>

export function providerEndpointError(
  binding: ProviderBinding | null | undefined,
  endpoints: ImportedEndpointConfig
): string | undefined {
  if (!binding) return
  if (binding.protocols.some((protocol) => !endpoints[protocol]))
    return 'channels.plugins.providerProtocolMismatch'
  for (const [protocol, query] of Object.entries(binding.imageQueries ?? {})) {
    const url = endpoints[protocol as keyof ImportedEndpointConfig]?.url
    if (
      !url ||
      !URL.canParse(url) ||
      !URL.canParse(query.url) ||
      new URL(url).origin !== new URL(query.url).origin
    )
      return 'channels.plugins.imageQueryOriginMismatch'
  }
  const catalog = binding.catalog
  if (
    catalog &&
    !binding.protocols.some((protocol) => {
      const url = endpoints[protocol]?.url
      if (!url || !URL.canParse(url) || !URL.canParse(catalog.url)) return false
      const endpoint = new URL(url)
      if (endpoint.protocol === 'ws:') endpoint.protocol = 'http:'
      if (endpoint.protocol === 'wss:') endpoint.protocol = 'https:'
      return endpoint.origin === new URL(catalog.url).origin
    })
  )
    return 'channels.plugins.catalogOriginMismatch'
}

function channelLimitNumber() {
  return z
    .number({ error: 'channels.upstream.invalidLimit' })
    .int('channels.upstream.invalidLimit')
    .min(0, 'channels.upstream.invalidLimit')
}
