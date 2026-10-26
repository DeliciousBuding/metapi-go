import { z } from 'zod'

import type { ImportedEndpointConfig } from '@/lib/api/imported-upstreams'
import type { UpstreamConnectInput } from '@/lib/api/upstream-catalog'
import type { UpstreamPreset } from '@/lib/api/upstream-presets'

import {
  connectionSchema,
  headersForRequest,
  upstreamProtocols,
} from './upstream-config'

export function upstreamCreateSchema(preset: UpstreamPreset | null) {
  return z
    .object({
      presetId: z.string().min(1, 'channels.create.selectProvider'),
      protocols: z.array(z.custom<UpstreamPreset['protocols'][number]>()),
      customEndpoints: z.boolean(),
      endpointConfig: z.custom<ImportedEndpointConfig>(),
      apiKey: z.string(),
      gcpServiceAccount: z.string(),
      projectId: z.string().trim(),
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
      customHeaders: connectionSchema.shape.customHeaders,
      paramOverride: connectionSchema.shape.paramOverride,
    })
    .superRefine((value, ctx) => {
      if (value.customEndpoints) {
        if (
          !connectionSchema.shape.endpointConfig.safeParse(value.endpointConfig)
            .success
        ) {
          ctx.addIssue({
            code: 'custom',
            path: ['endpointConfig'],
            message: 'channels.upstream.invalidEndpoints',
          })
        } else if (!Object.values(value.endpointConfig).some(Boolean)) {
          ctx.addIssue({
            code: 'custom',
            path: ['endpointConfig'],
            message: 'channels.create.customEndpointRequired',
          })
        }
      } else if (!value.protocols.length) {
        ctx.addIssue({
          code: 'custom',
          path: ['protocols'],
          message: 'channels.catalog.selectProtocols',
        })
      } else if (
        new Set(value.protocols).size !== value.protocols.length ||
        value.protocols.some(
          (key) =>
            !preset?.protocols.includes(key) ||
            !upstreamProtocols.some((protocol) => protocol.key === key)
        )
      ) {
        ctx.addIssue({
          code: 'custom',
          path: ['protocols'],
          message: 'channels.create.invalidProtocols',
        })
      }
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
      if (
        preset?.credentialMode === 'gcpServiceAccount' &&
        !value.gcpServiceAccount.trim()
      ) {
        ctx.addIssue({
          code: 'custom',
          path: ['gcpServiceAccount'],
          message: 'channels.upstream.required',
        })
      }
      if (
        preset?.requiresProject &&
        !value.customEndpoints &&
        !value.projectId
      ) {
        ctx.addIssue({
          code: 'custom',
          path: ['projectId'],
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
  protocols: [],
  customEndpoints: false,
  endpointConfig: {},
  apiKey: '',
  gcpServiceAccount: '',
  projectId: '',
  baseUrl: '',
  name: '',
  useSystemProxy: false,
  channelProxy: '',
  customHeaders: '',
  paramOverride: '',
}
export function upstreamCreatePayload(
  values: UpstreamCreateValues,
  preset: UpstreamPreset
): UpstreamConnectInput {
  const customHeaders = headersForRequest(values.customHeaders)
  return {
    presetId: values.presetId,
    ...(values.customEndpoints
      ? { endpointConfig: values.endpointConfig }
      : { protocols: values.protocols }),
    ...(values.apiKey.trim() ? { apiKey: values.apiKey } : {}),
    ...(values.gcpServiceAccount.trim()
      ? { gcpServiceAccount: values.gcpServiceAccount.trim() }
      : {}),
    ...(!values.customEndpoints && values.projectId
      ? { projectId: values.projectId }
      : {}),
    ...(values.baseUrl &&
    (preset.requiresBaseUrl || values.baseUrl !== preset.defaultUrl)
      ? { baseUrl: values.baseUrl }
      : {}),
    ...(values.name ? { name: values.name } : {}),
    ...(values.useSystemProxy ? { useSystemProxy: true } : {}),
    ...(values.channelProxy ? { channelProxy: values.channelProxy } : {}),
    ...(customHeaders ? { customHeaders } : {}),
    ...(values.paramOverride.trim()
      ? { paramOverride: values.paramOverride }
      : {}),
  }
}
