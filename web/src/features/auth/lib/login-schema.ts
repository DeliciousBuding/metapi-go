// metapi-go/features/auth — login form schema (Zod).
// Single admin-token field. The error message is an i18next key that
// FormMessage resolves via t().

import { z } from 'zod'

export const loginFormSchema = z.object({
  token: z.string().min(1, 'auth.login.tokenRequired'),
})

export type LoginFormValues = z.infer<typeof loginFormSchema>
