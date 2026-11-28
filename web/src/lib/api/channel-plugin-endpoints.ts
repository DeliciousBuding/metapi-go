import { z } from 'zod'

import { importedEndpointKeys } from './imported-upstreams'

export const protocol = z.enum([
  'chat',
  'responses',
  'messages',
  'gemini',
  'responsesCompact',
  'responsesWebsocket',
])
export const key = z.string().regex(/^[a-z][a-z0-9-]{0,63}$/)
export const providerProtocol = z.enum(importedEndpointKeys)
export const cursorParameter = z
  .string()
  .regex(/^$|^[A-Za-z][A-Za-z0-9_-]{0,63}$/)
const templatePath = z
  .string()
  .max(8192)
  .refine((path) => {
    if (
      !path.startsWith('/') ||
      path.startsWith('//') ||
      /[?#\\\r\n\0]/.test(path)
    )
      return false
    try {
      const decoded = decodeURIComponent(path)
      return (
        !/[\\\r\n\0]/.test(decoded) &&
        !decoded.split('/').some((part) => part === '.' || part === '..')
      )
    } catch {
      return false
    }
  })
export const templateAuth = z.enum([
  'bearer',
  'x-api-key',
  'x-goog-api-key',
  'none',
])
export const imageTaskProtocol = z.enum([
  'imageGeneration',
  'imageEdit',
  'imageVariation',
  'modelscopeImageGeneration',
])
export const imageQueryPath = templatePath.refine((path) => {
  if (new TextEncoder().encode(path).length > 8192) return false
  try {
    const segments = decodeURIComponent(path).split('/')
    return (
      path.split('{taskId}').length === 2 &&
      segments.filter((segment) => segment === '{taskId}').length === 1 &&
      segments.every(
        (segment) => !/[{}]/.test(segment) || segment === '{taskId}'
      ) &&
      !/[?#]/.test(decodeURIComponent(path)) &&
      !/\p{Cc}/u.test(decodeURIComponent(path))
    )
  } catch {
    return false
  }
})
export const imageQueryUrl = z
  .string()
  .max(9216)
  .refine((value) => {
    if (new TextEncoder().encode(value).length > 9216) return false
    if (!URL.canParse(value)) return false
    const url = new URL(value)
    const path = value.match(/^https?:\/\/[^/?#]+(\/[^?#]*)$/i)?.[1]
    try {
      return (
        ['http:', 'https:'].includes(url.protocol) &&
        !url.username &&
        !url.password &&
        !url.search &&
        !url.hash &&
        path !== undefined &&
        imageQueryPath.safeParse(decodeURIComponent(path)).success
      )
    } catch {
      return false
    }
  }, 'channels.upstream.invalidUrl')
export const endpointTemplate = z
  .object({
    path: templatePath,
    auth: templateAuth,
    modelPath: z.boolean().optional(),
    profile: z.string().max(64).optional(),
  })
  .strict()
export const catalogTemplate = z
  .object({
    path: templatePath,
    auth: templateAuth,
    cursorParameter: cursorParameter.optional(),
  })
  .strict()
