import { z } from 'zod'

import type { UpstreamChannelCreate } from '@/lib/api/upstream-catalog'

import { connectionSchema } from './upstream-config'

export const upstreamCreateSchema = connectionSchema
  .pick({
    name: true,
    baseUrl: true,
    endpointConfig: true,
    useSystemProxy: true,
  })
  .extend({
    provider: z.string().trim().min(1, 'channels.create.selectProvider'),
    enabled: z.boolean(),
  })
  .refine((value) => Object.keys(value.endpointConfig).length > 0, {
    path: ['endpointConfig'],
    message: 'channels.create.endpointsRequired',
  })
  .refine(
    (value) =>
      value.provider === 'opencode_go' ||
      value.endpointConfig.chat?.profile !== 'opencode-go',
    {
      path: ['provider'],
      message: 'channels.create.openCodeProviderRequired',
    }
  )

export type UpstreamCreateValues = z.infer<typeof upstreamCreateSchema>

export const upstreamCreateDefaults: UpstreamCreateValues = {
  name: '',
  provider: '',
  baseUrl: '',
  endpointConfig: {},
  enabled: false,
  useSystemProxy: false,
}

export function upstreamCreatePayload(
  values: UpstreamCreateValues
): UpstreamChannelCreate & { dialect: 'generic' } {
  return { ...values, dialect: 'generic' }
}
