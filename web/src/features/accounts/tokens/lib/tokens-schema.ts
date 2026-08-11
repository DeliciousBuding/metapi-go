// metapi-go/features/accounts/tokens/lib — RHF + Zod form schema for the
// add/edit account-token dialog embedded inside the account detail sheet.
//
// Error messages are i18next keys (resolved by `<FormMessage>`).

import { z } from 'zod'

// ---------------------------------------------------------------------------
// Schema factory
// ---------------------------------------------------------------------------

export function getAccountTokenFormSchema() {
  return z.object({
    accountId: z
      .number({ message: 'accounts.tokens.schema.accountRequired' })
      .int({ message: 'accounts.tokens.schema.accountRequired' })
      .positive({ message: 'accounts.tokens.schema.accountRequired' }),
    name: z
      .string({ message: 'accounts.tokens.schema.nameRequired' })
      .trim()
      .min(1, { message: 'accounts.tokens.schema.nameRequired' })
      .max(120, { message: 'accounts.tokens.schema.nameTooLong' }),
    token: z
      .string({ message: 'accounts.tokens.schema.valueRequired' })
      .trim()
      .min(1, { message: 'accounts.tokens.schema.valueRequired' }),
    tokenGroup: z.string().trim().optional(),
    quota: z.number().nonnegative({ message: 'accounts.tokens.schema.quotaNonNegative' }).optional(),
    unlimited: z.boolean(),
    expiresAt: z.string().trim().optional(),
    allowedIps: z.string().trim().optional(),
  })
}

export type AccountTokenFormValues = z.infer<
  ReturnType<typeof getAccountTokenFormSchema>
>

// ---------------------------------------------------------------------------
// Defaults
// ---------------------------------------------------------------------------

export function getAccountTokenFormDefaultValues(
  accountId = 0,
): AccountTokenFormValues {
  return {
    accountId,
    name: '',
    token: '',
    tokenGroup: 'default',
    quota: undefined,
    unlimited: true,
    expiresAt: '',
    allowedIps: '',
  }
}

// ---------------------------------------------------------------------------
// Transformers
// ---------------------------------------------------------------------------

export interface AccountTokenPayload {
  accountId: number
  name: string
  token: string
  tokenGroup?: string
  quota?: number
  remainQuota?: number
  unlimitedQuota?: boolean
  expiredTime?: number
  modelLimitsEnabled?: boolean
  allowedIps?: string[]
}

export function transformTokenFormToPayload(
  values: AccountTokenFormValues,
): AccountTokenPayload {
  const allowedIps = values.allowedIps
    ? values.allowedIps
        .split(/[,，\s]+/)
        .map((ip) => ip.trim())
        .filter(Boolean)
    : undefined

  const expiredTime = values.expiresAt
    ? Math.floor(new Date(values.expiresAt).getTime() / 1000)
    : undefined

  return {
    accountId: values.accountId,
    name: values.name,
    token: values.token,
    tokenGroup: values.tokenGroup || undefined,
    quota: values.unlimited ? undefined : values.quota,
    remainQuota: values.unlimited ? undefined : values.quota,
    unlimitedQuota: values.unlimited || undefined,
    expiredTime,
    allowedIps,
  }
}
