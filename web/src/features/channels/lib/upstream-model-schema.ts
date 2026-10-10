import { z } from 'zod'

import { upstreamProtocols } from './upstream-config'

const name = z.string().trim().min(1, 'channels.upstream.required').max(255)
export const modelSchema = z.object({ name, enabled: z.boolean() })
export function modelNames(value: string): string[] {
  return value
    .split(/\r?\n/)
    .map((item) => item.trim())
    .filter(Boolean)
}
export const modelsCreateSchema = z.object({
  names: z
    .string()
    .trim()
    .min(1, 'channels.upstream.required')
    .refine(
      (value) =>
        z.array(name).min(1).max(500).safeParse(modelNames(value)).success,
      'channels.catalog.invalidModels'
    )
    .refine(
      (value) => new Set(modelNames(value)).size === modelNames(value).length,
      'channels.catalog.duplicateModels'
    ),
})
export const grantSchema = z.object({
  credentialId: z.number().int().positive('channels.catalog.selectCredential'),
  protocols: z
    .array(
      z
        .number()
        .refine(
          (bit) => upstreamProtocols.some((protocol) => protocol.bit === bit),
          'channels.catalog.selectProtocols'
        )
    )
    .min(1, 'channels.catalog.selectProtocols'),
  enabled: z.boolean(),
})
