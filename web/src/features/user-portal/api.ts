import axios from 'axios'
import { z } from 'zod'

import { buildQueryString, request } from '@/lib/api/transport'

const sessionSchema = z.object({
  user: z.object({
    id: z.number().int().positive(),
    username: z.string(),
    displayName: z.string(),
    groupId: z.number().int().positive(),
  }),
  group: z.object({ id: z.number().int().positive(), name: z.string() }),
  expiresAt: z.string().min(1),
})
const keySchema = z.object({
  id: z.number().int().positive(),
  name: z.string(),
  keyMasked: z.string(),
  enabled: z.boolean(),
  expiresAt: z.string().nullable(),
  supportedModels: z.array(z.string()),
  maxCost: z.number().nullable(),
  maxRequests: z.number().nullable(),
  usedCost: z.number().nonnegative(),
  usedRequests: z.number().nonnegative(),
  createdAt: z.string(),
})
const requestSchema = z.object({
  id: z.string().min(1),
  modelRequested: z.string().nullable(),
  terminalStatus: z.string(),
  latencyMs: z.number().nonnegative(),
  startedAt: z.string(),
  endedAt: z.string(),
  attemptCount: z.number().int().nonnegative(),
})
const budgetSchema = z.object({
  limit: z.number().nonnegative(),
  used: z.number().nonnegative(),
  reserved: z.number().nonnegative(),
  unlimited: z.boolean(),
  frozen: z.boolean(),
  unknownRequests: z.number().int().nonnegative(),
})
const modelsSchema = z.object({
  items: z.array(
    z.object({ id: z.string().min(1), routeId: z.number().int().positive() })
  ),
})
const keysSchema = z.object({ items: z.array(keySchema) })
const requestsSchema = z.object({
  items: z.array(requestSchema),
  total: z.number().int().nonnegative(),
  page: z.number().int().positive(),
  pageSize: z.number().int().positive(),
})

export type UserSession = z.infer<typeof sessionSchema>
export type PersonalKey = z.infer<typeof keySchema>
export type PersonalRequest = z.infer<typeof requestSchema>
export type PortalView = 'overview' | 'models' | 'keys' | 'requests' | 'account'
export const portalQueryRoot = ['userPortal'] as const

// Personal sessions must never trigger the administrator's 401 redirect or
// clear its session. This surface owns its anonymous/error states explicitly.
async function userRequest(
  path: string,
  method: 'GET' | 'POST' | 'DELETE' = 'GET',
  body?: unknown
): Promise<unknown> {
  return request('/api/user' + path, {
    method,
    body: body === undefined ? undefined : JSON.stringify(body),
    skipAuthRetry: true,
    skipErrorHandler: true,
    skipBusinessError: true,
    disableDuplicate: true,
  })
}

export const userPortalApi = {
  session: async (): Promise<UserSession | null> => {
    try {
      return sessionSchema.parse(await userRequest('/session'))
    } catch (error) {
      if (
        axios.isAxiosError(error) &&
        [401, 403].includes(error.response?.status ?? 0)
      )
        return null
      throw error
    }
  },
  login: (body: { username: string; password: string }) =>
    userRequest('/auth/login', 'POST', body),
  activate: (body: { username: string; password: string; code: string }) =>
    userRequest('/auth/activate', 'POST', body),
  logout: () => userRequest('/auth/logout', 'POST', {}),
  password: (body: { currentPassword: string; newPassword: string }) =>
    userRequest('/auth/password', 'POST', body),
  models: async () => modelsSchema.parse(await userRequest('/models')),
  keys: async () => keysSchema.parse(await userRequest('/keys')),
  createKey: async (body: {
    name: string
    supportedModels?: string[]
    expiresAt?: string
    maxRequests?: number
  }) =>
    z
      .object({ item: keySchema, key: z.string().min(1) })
      .parse(await userRequest('/keys', 'POST', body)),
  deleteKey: (id: number) => userRequest('/keys/' + id, 'DELETE'),
  requests: async (page: number) =>
    requestsSchema.parse(
      await userRequest('/requests' + buildQueryString({ page, pageSize: 20 }))
    ),
  request: async (id: string) =>
    requestSchema.parse(
      await userRequest('/requests/' + encodeURIComponent(id))
    ),
  budget: async () => budgetSchema.parse(await userRequest('/budget')),
  usage: async () =>
    z
      .object({
        usedRequests: z.number().nonnegative(),
        keyCount: z.number().int().nonnegative(),
      })
      .parse(await userRequest('/usage')),
}
