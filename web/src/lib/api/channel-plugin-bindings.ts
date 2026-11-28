import { z } from 'zod'

import {
  key,
  protocol,
  providerProtocol,
  imageTaskProtocol,
  imageQueryUrl,
  templateAuth,
  cursorParameter,
} from './channel-plugin-endpoints'
import type { importedEndpointKeys } from './imported-upstreams'

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
