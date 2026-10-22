import { z } from 'zod'

import { confirmHeader, request } from './transport'

const id = z.number().int().positive()
const userSchema = z.object({
  id,
  username: z.string(),
  displayName: z.string(),
  groupId: id,
  enabled: z.boolean(),
  createdAt: z.string(),
  updatedAt: z.string(),
})
const groupSchema = z.object({
  id,
  slug: z.string(),
  name: z.string(),
  enabled: z.boolean(),
  createdAt: z.string(),
  updatedAt: z.string(),
})
const directory = <T extends z.ZodType>(item: T) =>
  z.object({ items: z.array(item), limit: z.number().int().positive() })
const grantSchema = z.object({
  groupId: id,
  routeId: id,
  createdAt: z.string(),
})
const amount = z.number().finite().nonnegative()
const budgetSchema = z.object({
  limit: amount,
  used: amount,
  reserved: amount,
  unlimited: z.boolean(),
  frozen: z.boolean(),
  unknownRequests: z.number().int().nonnegative(),
})
const pricingSchema = z.object({ multiplier: z.number().finite().positive() })
const routeSchema = z.object({
  id,
  modelPattern: z.string(),
  displayName: z.string().nullable(),
  // The existing route-summary API preserves SQLite 0/1 and PostgreSQL booleans.
  enabled: z
    .union([z.boolean(), z.literal(0), z.literal(1)])
    .transform((value) => value === true || value === 1),
})
const successSchema = z.object({ success: z.literal(true) })

export type GatewayUser = z.infer<typeof userSchema>
export type GatewayUserGroup = z.infer<typeof groupSchema>
export type UserBudget = z.infer<typeof budgetSchema>
export type GroupPricing = z.infer<typeof pricingSchema>
export type UserInput = {
  username: string
  displayName: string
  groupId: number
  enabled: boolean
}
export type UserUpdate = Omit<UserInput, 'username'>
export type GroupInput = { slug: string; name: string; enabled: boolean }
export type GroupUpdate = Omit<GroupInput, 'slug'>

async function write(
  path: string,
  body: unknown,
  confirmToken?: string,
  method: 'POST' | 'PUT' | 'DELETE' = 'PUT'
) {
  return request<unknown>(path, {
    method,
    body: JSON.stringify(body),
    headers: confirmHeader(confirmToken),
  })
}

/** Strict wire parsing: malformed directories and grants must never become empty authorization lists. */
export const gatewayUsersApi = {
  users: async () =>
    directory(userSchema).parse(await request('/api/admin/users')),
  groups: async () =>
    directory(groupSchema).parse(await request('/api/admin/user-groups')),
  createUser: async (body: UserInput, token?: string) =>
    userSchema.parse(await write('/api/admin/users', body, token, 'POST')),
  updateUser: async (userId: number, body: UserUpdate, token?: string) =>
    successSchema.parse(await write(`/api/admin/users/${userId}`, body, token)),
  createGroup: async (body: GroupInput, token?: string) =>
    groupSchema.parse(
      await write('/api/admin/user-groups', body, token, 'POST')
    ),
  updateGroup: async (groupId: number, body: GroupUpdate, token?: string) =>
    successSchema.parse(
      await write(`/api/admin/user-groups/${groupId}`, body, token)
    ),
  grants: async (groupId: number) =>
    z
      .object({ items: z.array(grantSchema) })
      .parse(await request(`/api/admin/user-groups/${groupId}/routes`)),
  publicRoutes: async () =>
    z.array(routeSchema).parse(await request('/api/routes/summary')),
  grant: async (groupId: number, routeId: number, token?: string) =>
    successSchema.parse(
      await write(
        `/api/admin/user-groups/${groupId}/routes`,
        { routeId },
        token,
        'POST'
      )
    ),
  revoke: async (groupId: number, routeId: number, token?: string) =>
    successSchema.parse(
      await write(
        `/api/admin/user-groups/${groupId}/routes/${routeId}`,
        undefined,
        token,
        'DELETE'
      )
    ),
  budget: async (userId: number) =>
    budgetSchema.parse(await request(`/api/admin/users/${userId}/budget`)),
  holds: async (userId: number, offset = 0) =>
    z
      .object({
        items: z.array(
          z.object({
            requestId: z.string().min(1),
            state: z.enum(['pending', 'unknown']),
            reserved: amount,
            totalTokens: z.number().int().nonnegative().nullable(),
            createdAt: z.string(),
            multiplier: z.number().positive(),
          })
        ),
        limit: z.number().int().positive(),
        offset: z.number().int().nonnegative(),
      })
      .parse(
        await request(
          `/api/admin/users/${userId}/budget/holds?offset=${offset}`
        )
      ),
  reconcile: async (
    userId: number,
    body: { requestId: string; totalTokens: number; confirmTerminated: true },
    token?: string
  ) =>
    budgetSchema.parse(
      await write(
        `/api/admin/users/${userId}/budget/reconcile`,
        body,
        token,
        'POST'
      )
    ),
  setBudget: async (
    userId: number,
    body: Pick<UserBudget, 'limit' | 'unlimited'>,
    token?: string
  ) =>
    budgetSchema.parse(
      await write(`/api/admin/users/${userId}/budget`, body, token)
    ),
  pricing: async (groupId: number) =>
    pricingSchema.parse(
      await request(`/api/admin/user-groups/${groupId}/pricing`)
    ),
  setPricing: async (groupId: number, body: GroupPricing, token?: string) =>
    pricingSchema.parse(
      await write(`/api/admin/user-groups/${groupId}/pricing`, body, token)
    ),
  // Activation secrets are only consumed by component-local memory, never useQuery/useMutation.
  activate: async (userId: number, token?: string) =>
    z
      .object({
        code: z.string().min(1),
        expiresAt: z.iso.datetime({ offset: true }),
      })
      .parse(
        await write(`/api/admin/users/${userId}/activation`, {}, token, 'POST')
      ),
}
