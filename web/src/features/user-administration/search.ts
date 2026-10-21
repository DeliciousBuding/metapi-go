import { z } from 'zod'

export const userAdminSearchSchema = z.object({
  view: z.enum(['users', 'groups']).optional().catch(undefined),
  q: z.string().optional().catch(undefined),
  status: z.enum(['enabled', 'disabled']).optional().catch(undefined),
})
