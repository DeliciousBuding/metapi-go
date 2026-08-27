// metapi-go/features/accounts/lib — RHF + Zod form schema for the add/edit
// account dialog. Follows the keys feature's `lib/api-key-form.ts` pattern:
// a schema *factory* (so cross-field rules can run), default-values factory,
// and two transformers (form → API payload, entity → form defaults).
//
// The form is mode-aware (session / apikey / password credential modes).
// Session mode collects an Access Token / optional platformUserId / optional
// sub2api refresh token; apikey mode collects an API Key; password mode
// collects the site login username + password (bound via the separate
// POST /api/accounts/login endpoint, not the accounts create payload).
// Conditional required-field rules live in `superRefine` rather than a
// discriminated union so the RHF field array stays stable across mode
// switches.
//
// Error messages are i18next keys (resolved by `<FormMessage>`).

import { z } from 'zod'

import type { Account, AccountPayload, CredentialMode } from '../types'

// ---------------------------------------------------------------------------
// Schema factory
// ---------------------------------------------------------------------------

export function getAccountFormSchema(requireCredential = true) {
  return z
    .object({
      siteId: z
        .number({ message: 'accounts.schema.siteRequired' })
        .int({ message: 'accounts.schema.siteRequired' })
        .positive({ message: 'accounts.schema.siteRequired' }),
      credentialMode: z.enum(['session', 'apikey', 'password']),
      username: z.string().trim().optional(),
      password: z.string().trim().optional(),
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
        if (
          requireCredential &&
          (!value.accessToken || value.accessToken.length === 0)
        ) {
          ctx.addIssue({
            code: 'custom',
            path: ['accessToken'],
            message: 'accounts.schema.accessTokenRequired',
          })
        }
      } else if (value.credentialMode === 'apikey') {
        if (
          requireCredential &&
          (!value.apiToken || value.apiToken.length === 0)
        ) {
          ctx.addIssue({
            code: 'custom',
            path: ['apiToken'],
            message: 'accounts.schema.apiKeyRequired',
          })
        }
      } else {
        if (!value.username || value.username.length === 0) {
          ctx.addIssue({
            code: 'custom',
            path: ['username'],
            message: 'accounts.schema.usernameRequired',
          })
        }
        if (!value.password || value.password.length === 0) {
          ctx.addIssue({
            code: 'custom',
            path: ['password'],
            message: 'accounts.schema.passwordRequired',
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
    password: '',
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

// Password mode has no accounts create/update payload: it posts
// `{siteId, username, password}` to POST /api/accounts/login instead, so the
// transform returns undefined and the caller routes to the login mutation.
export function transformFormToPayload(
  values: AccountFormValues,
  operation: 'create' | 'update' = 'create'
): AccountPayload | undefined {
  if (values.credentialMode === 'password') return undefined

  const tags = parseTagsInput(values.tags)
  const extraConfig = values.proxyUrl
    ? JSON.stringify({ proxyUrl: values.proxyUrl })
    : undefined

  if (values.credentialMode === 'session') {
    return {
      siteId: values.siteId,
      credentialMode: 'session',
      accessToken: values.accessToken || undefined,
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
    apiToken:
      operation === 'update' && values.apiToken ? values.apiToken : undefined,
    accessTokens:
      operation === 'create' && values.apiToken ? [values.apiToken] : undefined,
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
    password: '',
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

// ---------------------------------------------------------------------------
// Verify-token payload builder
// ---------------------------------------------------------------------------

type AccountVerifyPayload = {
  siteId: number
  accessToken: string
  platformUserId?: number
  proxyUrl?: string
  credentialMode: 'session' | 'apikey'
}

type AccountVerifyPayloadError = 'site' | 'token'

export type AccountVerifyPayloadResult =
  | { ok: true; payload: AccountVerifyPayload }
  | { ok: false; error: AccountVerifyPayloadError }

/**
 * Build the inline verify-token payload from the current unsaved form values.
 * Password mode is never verified inline — it binds through the real login
 * submit path — so its empty session token resolves to the `token` error and
 * the caller skips the verify action entirely for password mode.
 */
export function buildAccountVerifyPayload(
  values: AccountFormValues
): AccountVerifyPayloadResult {
  if (!values.siteId || values.siteId <= 0) {
    return { ok: false, error: 'site' }
  }
  const token =
    values.credentialMode === 'apikey' ? values.apiToken : values.accessToken
  if (!token || !token.trim()) {
    return { ok: false, error: 'token' }
  }
  const platformUserId =
    values.platformUserId && values.platformUserId > 0
      ? values.platformUserId
      : undefined
  const proxyUrl = values.proxyUrl?.trim() || undefined

  return {
    ok: true,
    payload: {
      siteId: values.siteId,
      accessToken: token.trim(),
      ...(platformUserId === undefined ? {} : { platformUserId }),
      ...(proxyUrl === undefined ? {} : { proxyUrl }),
      credentialMode: values.credentialMode === 'apikey' ? 'apikey' : 'session',
    },
  }
}
