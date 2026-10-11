import { z } from 'zod'

import type { UpstreamConnectInput } from '@/lib/api/upstream-catalog'
import type { UpstreamPreset } from '@/lib/api/upstream-presets'

import { connectionSchema } from './upstream-config'

export function upstreamCreateSchema(preset: UpstreamPreset | null) {
  return z
    .object({
      presetId: z.string().min(1, 'channels.create.selectProvider'),
      apiKey: z.string(),
      baseUrl: z
        .string()
        .trim()
        .refine(
          (value) =>
            !value || connectionSchema.shape.baseUrl.safeParse(value).success,
          'channels.upstream.invalidUrl'
        ),
      name: z.string().trim(),
      useSystemProxy: z.boolean(),
      channelProxy: z.string().trim(),
    })
    .superRefine((value, ctx) => {
      if (
        (preset?.credentialMode ?? 'apiKey') === 'apiKey' &&
        !value.apiKey.trim()
      ) {
        ctx.addIssue({
          code: 'custom',
          path: ['apiKey'],
          message: 'channels.upstream.required',
        })
      }
      if ((preset?.requiresBaseUrl ?? !preset?.defaultUrl) && !value.baseUrl) {
        ctx.addIssue({
          code: 'custom',
          path: ['baseUrl'],
          message: 'channels.upstream.required',
        })
      }
    })
}
export type UpstreamCreateValues = z.infer<
  ReturnType<typeof upstreamCreateSchema>
>
export const upstreamCreateDefaults: UpstreamCreateValues = {
  presetId: '',
  apiKey: '',
  baseUrl: '',
  name: '',
  useSystemProxy: false,
  channelProxy: '',
}
export function upstreamCreatePayload(
  values: UpstreamCreateValues,
  preset: UpstreamPreset
): UpstreamConnectInput {
  return {
    presetId: values.presetId,
    ...(values.apiKey.trim() ? { apiKey: values.apiKey } : {}),
    ...(values.baseUrl &&
    (preset.requiresBaseUrl || values.baseUrl !== preset.defaultUrl)
      ? { baseUrl: values.baseUrl }
      : {}),
    ...(values.name ? { name: values.name } : {}),
    ...(values.useSystemProxy ? { useSystemProxy: true } : {}),
    ...(values.channelProxy ? { channelProxy: values.channelProxy } : {}),
  }
}
