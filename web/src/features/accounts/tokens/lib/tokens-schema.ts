// metapi-go/features/accounts/tokens/lib — RHF + Zod form schema for the
// add/edit account-token dialog embedded inside the account detail sheet.
//
// Error messages are i18next keys (resolved by `<FormMessage>`).

import { z } from 'zod'

// ---------------------------------------------------------------------------
// Schema factory
// ---------------------------------------------------------------------------

export function getAccountTokenFormSchema() {
  return z
    .object({
      accountId: z
        .number({ message: 'accounts.tokens.schema.accountRequired' })
        .int({ message: 'accounts.tokens.schema.accountRequired' })
        .positive({ message: 'accounts.tokens.schema.accountRequired' }),
      name: z
        .string({ message: 'accounts.tokens.schema.nameRequired' })
        .trim()
        .min(1, { message: 'accounts.tokens.schema.nameRequired' })
        .max(120, { message: 'accounts.tokens.schema.nameTooLong' }),
      // An empty value creates a token upstream on add, or preserves it on edit.
      token: z.string().trim(),
      tokenGroup: z.string().trim().optional(),
      quota: z
        .number()
        .nonnegative({ message: 'accounts.tokens.schema.quotaNonNegative' })
        .optional(),
      unlimited: z.boolean(),
      expiresAt: z.string().trim().optional(),
      allowedIps: z.string().trim().optional(),
    })
    .superRefine((values, ctx) => {
      if (!values.token && !values.unlimited && values.quota === undefined) {
        ctx.addIssue({
          code: 'custom',
          path: ['quota'],
          message: 'accounts.tokens.schema.quotaRequired',
        })
      }
    })
}

export type AccountTokenFormValues = z.infer<
  ReturnType<typeof getAccountTokenFormSchema>
>

// ---------------------------------------------------------------------------
// Defaults
// ---------------------------------------------------------------------------

export function getAccountTokenFormDefaultValues(
  accountId = 0
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
  token?: string
  group?: string
  remainQuota?: number
  unlimitedQuota?: boolean
  expiredTime?: number
  allowIps?: string
}

export function transformTokenFormToPayload(
  values: AccountTokenFormValues
): AccountTokenPayload {
  const base = {
    accountId: values.accountId,
    name: values.name,
    group: values.tokenGroup || undefined,
  }
  // A pasted value is a local import. Its upstream limits are not changed.
  if (values.token.trim()) return { ...base, token: values.token.trim() }

  return {
    ...base,
    remainQuota: values.unlimited ? undefined : values.quota,
    unlimitedQuota: values.unlimited,
    expiredTime: values.expiresAt
      ? Math.floor(new Date(values.expiresAt).getTime() / 1000)
      : undefined,
    allowIps: values.allowedIps
      ? values.allowedIps
          .split(/[,，\s]+/)
          .filter(Boolean)
          .join(',')
      : undefined,
  }
}
