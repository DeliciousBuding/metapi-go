import { z } from 'zod'

import {
  key,
  protocol,
  providerProtocol,
  imageTaskProtocol,
  imageQueryPath,
  templateAuth,
  endpointTemplate,
  catalogTemplate,
} from './channel-plugin-endpoints'
import {
  authentication,
  protocolObservations,
} from './channel-plugin-provider-rules'
import type { importedEndpointKeys } from './imported-upstreams'

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
const manifestFields = {
  key,
  name: z.string().min(1),
  version: z.string().min(1),
  options: z.array(option).max(32).optional(),
}
export const wireManifest = z
  .object({
    ...manifestFields,
    apiVersion: z.literal(1),
    protocols: z.array(protocol).min(1).max(6),
    hooks: z
      .array(z.enum(['request', 'response', 'event']))
      .min(1)
      .max(3),
  })
  .strict()
export const providerManifest = z
  .object({
    ...manifestFields,
    apiVersion: z.literal(2),
    kind: z.literal('provider'),
    taskCapture: z.literal('durable').optional(),
    protocols: z.array(providerProtocol).min(1).max(26),
    credentialKinds: z
      .array(z.enum(['api_key', 'oauth', 'gcp_service_account', 'none']))
      .min(1)
      .max(4),
    requestKinds: z
      .array(z.enum(['json', 'multipart']))
      .min(1)
      .max(2)
      .optional(),
    authentication: authentication.optional(),
    imageTasks: z
      .partialRecord(
        imageTaskProtocol,
        z
          .object({
            stream: z.boolean().optional(),
            query: z
              .object({
                path: imageQueryPath,
                auth: templateAuth,
                modelPath: z.literal(false).optional(),
                profile: z.literal('').optional(),
              })
              .strict(),
          })
          .strict()
      )
      .refine((tasks) => Object.keys(tasks).length > 0)
      .optional(),
    hooks: z
      .array(
        z.enum([
          'request',
          'response',
          'event',
          'decodeResponse',
          'decodeEvent',
          'modelsRequest',
          'modelsResponse',
          'submit',
          'query',
          'content',
          'taskResult',
          'deleteTask',
          'remix',
        ])
      )
      .min(1)
      .max(13),
    observations: z
      .partialRecord(providerProtocol, protocolObservations)
      .optional(),
    defaults: z
      .object({
        endpoints: z
          .partialRecord(providerProtocol, endpointTemplate)
          .optional(),
        catalog: catalogTemplate.optional(),
      })
      .strict()
      .optional(),
  })
  .strict()
  .refine((manifest) => {
    if (
      manifest.taskCapture &&
      !manifest.protocols.some((protocol) =>
        ['video', 'seedanceVideo', 'zenmuxVideo'].includes(protocol)
      )
    )
      return false
    const hooks: readonly string[] = manifest.hooks
    if (
      (hooks.includes('deleteTask') || hooks.includes('remix')) &&
      !manifest.protocols.some((protocol) =>
        ['video', 'seedanceVideo', 'zenmuxVideo'].includes(protocol)
      )
    )
      return false
    if (
      manifest.authentication &&
      (manifest.credentialKinds.length !== 1 ||
        manifest.credentialKinds[0] !== 'api_key')
    )
      return false
    if (
      new Set(manifest.protocols).size !== manifest.protocols.length ||
      new Set(hooks).size !== hooks.length ||
      new Set(manifest.credentialKinds).size !== manifest.credentialKinds.length
    )
      return false
    if (
      new Set(manifest.requestKinds ?? []).size !==
      (manifest.requestKinds ?? []).length
    )
      return false
    if (hooks.includes('modelsRequest') !== hooks.includes('modelsResponse'))
      return false
    if (manifest.defaults?.catalog && !hooks.includes('modelsRequest'))
      return false
    const imageTasks = Object.keys(manifest.imageTasks ?? {})
    if (
      imageTasks.some(
        (key) =>
          (key === 'imageVariation' &&
            manifest.imageTasks?.[key]?.stream === true) ||
          !manifest.protocols.includes(
            key as (typeof importedEndpointKeys)[number]
          ) ||
          !manifest.observations?.[key as (typeof importedEndpointKeys)[number]]
            ?.response?.taskIdPath
      )
    )
      return false
    if (
      Object.keys(manifest.observations ?? {}).some(
        (key) =>
          !manifest.protocols.includes(
            key as (typeof importedEndpointKeys)[number]
          )
      )
    )
      return false
    if (
      Object.keys(manifest.defaults?.endpoints ?? {}).some(
        (key) =>
          !manifest.protocols.includes(
            key as (typeof importedEndpointKeys)[number]
          )
      )
    )
      return false
    return manifest.protocols.every((protocol) => {
      let required = ['request', 'response']
      if (imageTasks.includes(protocol)) {
        required = ['submit', 'query', 'taskResult']
      } else if (protocol === 'responsesWebsocket') {
        required = ['request', 'event']
      } else if (['video', 'seedanceVideo', 'zenmuxVideo'].includes(protocol)) {
        required = ['submit', 'query', 'content', 'taskResult']
      }
      return required.every((hook) => hooks.includes(hook))
    })
  })

export type ChannelPluginManifest =
  | z.infer<typeof wireManifest>
  | z.infer<typeof providerManifest>
export type ChannelPluginOption = z.infer<typeof option>
