import { z } from 'zod'

export function upstreamGroupSchema(creating: boolean) {
  return z
    .object({
      name: z.string().trim().min(1, 'channels.upstream.required').max(200),
      modelPattern: z.string().trim(),
      grantIds: z.array(z.number().int().positive()),
      mode: z.enum(['failover', 'manual']),
      enabled: z.boolean(),
      activeId: z.number().int().nonnegative(),
    })
    .superRefine((value, ctx) => {
      if (
        creating &&
        (!value.modelPattern ||
          value.modelPattern.length > 200 ||
          /[*?]|^re:/i.test(value.modelPattern))
      ) {
        ctx.addIssue({
          code: 'custom',
          path: ['modelPattern'],
          message: 'channels.group.exactModelRequired',
        })
      }
      if (creating && !value.grantIds.length) {
        ctx.addIssue({
          code: 'custom',
          path: ['grantIds'],
          message: 'channels.group.grantRequired',
        })
      }
      if (value.mode === 'manual' && value.enabled && !value.activeId) {
        ctx.addIssue({
          code: 'custom',
          path: ['activeId'],
          message: 'channels.group.activeRequired',
        })
      }
      if (
        creating &&
        value.activeId &&
        !value.grantIds.includes(value.activeId)
      ) {
        ctx.addIssue({
          code: 'custom',
          path: ['activeId'],
          message: 'channels.group.activeRequired',
        })
      }
    })
}
export type UpstreamGroupValues = z.infer<
  ReturnType<typeof upstreamGroupSchema>
>

export const upstreamGroupBindingSchema = z.object({
  groupId: z.number().int().positive('channels.group.groupRequired'),
  grantId: z.number().int().positive('channels.group.grantRequired'),
})
export type UpstreamGroupBindingValues = z.infer<
  typeof upstreamGroupBindingSchema
>
