// metapi-go features/accounts/tokens/lib — RHF + Zod form schema for the
// add/edit account-token dialog embedded inside the account detail sheet.

import { z } from 'zod'

// ---------------------------------------------------------------------------
// Schema factory
// ---------------------------------------------------------------------------

export function getAccountTokenFormSchema() {
  return z.object({
    accountId: z
      .number({ message: '请选择所属账号' })
      .int({ message: '请选择所属账号' })
      .positive({ message: '请选择所属账号' }),
    name: z
      .string({ message: '请填写令牌名称' })
      .trim()
      .min(1, { message: '请填写令牌名称' })
      .max(120, { message: '令牌名称过长' }),
    token: z
      .string({ message: '请填写令牌值' })
      .trim()
      .min(1, { message: '请填写令牌值' }),
    tokenGroup: z.string().trim().optional(),
    quota: z.number().nonnegative({ message: '额度需为非负数' }).optional(),
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
