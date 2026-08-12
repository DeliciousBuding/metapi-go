// metapi-go/features/accounts/lib — RHF + Zod form schema for the add/edit
// account dialog. Follows the keys feature's `lib/api-key-form.ts` pattern:
// a schema *factory* (so cross-field rules can run), default-values factory,
// and two transformers (form → API payload, entity → form defaults).
//
// The form is mode-aware (session vs apikey credential mode). Session mode
// collects an Access Token / optional platformUserId / optional sub2api
// refresh token; apikey mode collects an API Key. Conditional required-field
// rules live in `superRefine` rather than a discriminated union so the RHF
// field array stays stable across mode switches.
//
// Error messages are i18next keys (resolved by `<FormMessage>`).

import { z } from 'zod'

import type { Account, AccountPayload, CredentialMode } from '../types'

// ---------------------------------------------------------------------------
// Schema factory
// ---------------------------------------------------------------------------

export function getAccountFormSchema() {
  return z
    .object({
      siteId: z
        .number({ message: 'accounts.schema.siteRequired' })
        .int({ message: 'accounts.schema.siteRequired' })
        .positive({ message: 'accounts.schema.siteRequired' }),
      credentialMode: z.enum(['session', 'apikey']),
      username: z.string().trim().optional(),
      accessToken: z.string().trim().optional(),
      apiToken: z.string().trim().optional(),
      platformUserId: z.number().int().positive().optional(),
      status: z.enum(['active', 'disabled', 'expired']),
      checkinEnabled: z.boolean(),
      unitCost: z.number().nonnegative().optional(),
      proxyUrl: z
        .string()
        .trim()
        .optional()
        .refine((value) => !value || /^https?:\/\/.+/.test(value), {
          message: 'accounts.schema.invalidProxyUrl',
        }),
      refreshToken: z.string().trim().optional(),
      tokenExpiresAt: z.number().int().positive().optional(),
      skipModelFetch: z.boolean(),
      tags: z.string().trim().optional(),
    })
    .superRefine((value, ctx) => {
      if (value.credentialMode === 'session') {
        if (!value.accessToken || value.accessToken.length === 0) {
          ctx.addIssue({
            code: 'custom',
            path: ['accessToken'],
            message: 'accounts.schema.accessTokenRequired',
          })
        }
      } else {
        if (!value.apiToken || value.apiToken.length === 0) {
          ctx.addIssue({
            code: 'custom',
            path: ['apiToken'],
            message: 'accounts.schema.apiKeyRequired',
          })
        }
      }
    })
}

export type AccountFormValues = z.infer<ReturnType<typeof getAccountFormSchema>>

// ---------------------------------------------------------------------------
// Defaults
// ---------------------------------------------------------------------------

export function getAccountFormDefaultValues(
  mode: CredentialMode = 'session'
): AccountFormValues {
  return {
    siteId: 0,
    credentialMode: mode,
    username: '',
    accessToken: '',
    apiToken: '',
    platformUserId: undefined,
    status: 'active',
    checkinEnabled: false,
    unitCost: undefined,
    proxyUrl: '',
    refreshToken: '',
    tokenExpiresAt: undefined,
    skipModelFetch: false,
    tags: '',
  }
}

// ---------------------------------------------------------------------------
// Transformers
// ---------------------------------------------------------------------------

function parseTagsInput(raw: string | undefined): string[] | undefined {
  if (!raw || !raw.trim()) return undefined
  return raw
    .split(/[,，\s]+/)
    .map((tag) => tag.trim())
    .filter(Boolean)
}

export function transformFormToPayload(
  values: AccountFormValues
): AccountPayload {
  const tags = parseTagsInput(values.tags)
  const extraConfig = values.proxyUrl
    ? JSON.stringify({ proxyUrl: values.proxyUrl })
    : undefined

  if (values.credentialMode === 'session') {
    return {
      siteId: values.siteId,
      credentialMode: 'session',
      accessToken: values.accessToken,
      username: values.username || undefined,
      platformUserId: values.platformUserId,
      status: values.status,
      checkinEnabled: values.checkinEnabled,
      unitCost: values.unitCost,
      proxyUrl: values.proxyUrl || undefined,
      refreshToken: values.refreshToken || undefined,
      tokenExpiresAt: values.tokenExpiresAt,
      tags,
      extraConfig,
    }
  }

  return {
    siteId: values.siteId,
    credentialMode: 'apikey',
    accessTokens: values.apiToken ? [values.apiToken] : [],
    username: values.username || undefined,
    status: values.status,
    checkinEnabled: values.checkinEnabled,
    unitCost: values.unitCost,
    skipModelFetch: values.skipModelFetch,
    tags,
    extraConfig,
  }
}

export function transformAccountToFormValues(
  account: Account
): Partial<AccountFormValues> {
  return {
    siteId: account.siteId,
    credentialMode: account.credentialMode,
    username: account.username ?? '',
    accessToken: '',
    apiToken: '',
    platformUserId: undefined,
    status: account.status,
    checkinEnabled: account.checkinEnabled ?? false,
    unitCost: account.unitCost ?? undefined,
    proxyUrl: extractProxyUrl(account.extraConfig),
    refreshToken: '',
    tokenExpiresAt: undefined,
    skipModelFetch: false,
    tags: Array.isArray(account.tags) ? account.tags.join(', ') : '',
  }
}

function extractProxyUrl(extraConfig: string | null | undefined): string {
  if (!extraConfig) return ''
  try {
    const parsed = JSON.parse(extraConfig) as { proxyUrl?: unknown }
    if (typeof parsed.proxyUrl === 'string') return parsed.proxyUrl
  } catch {
    // not JSON — leave blank
  }
  return ''
}
