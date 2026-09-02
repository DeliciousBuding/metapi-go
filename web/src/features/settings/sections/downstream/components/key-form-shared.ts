// metapi-go/features/settings/sections/downstream — downstream-key shared
// core: API item types, TanStack query keys, zod form schemas, and the
// value mappers shared by the key sheet form, the table cells, and the
// section. Pure helpers only — no React.
import { z } from 'zod'

import {
  credentialRefSchema,
  parseCredentialRefs,
  parseIdArray,
} from '../lib/credential-refs'

type DownstreamKeyUsage24h = {
  requests?: number
  tokens?: number
  cost?: number
}

export type DownstreamApiKeyItem = {
  id: number
  name: string
  keyMasked?: string
  groupName?: string
  enabled: boolean
  expiresAt?: string | null
  maxCost?: number | null
  usedCost?: number | null
  maxRequests?: number | null
  usedRequests?: number | null
  supportedModels?: string[] | string | null
  allowedRouteIds?: number[] | string | null
  allowedSiteIds?: number[] | string | null
  excludedSiteIds?: number[] | string | null
  // Credential-ref columns: GET returns the stored columns verbatim — a raw
  // JSON string (or null); parsed with parseCredentialRefs before use.
  allowedCredentialRefs?: string | unknown[] | null
  excludedCredentialRefs?: string | unknown[] | null
  usage24h?: DownstreamKeyUsage24h
}

export type DownstreamKeysResponse = { items: DownstreamApiKeyItem[] }

// POST /api/downstream-keys responds with the created row under `item` (the
// handler re-reads the inserted row and adds a camelCase `keyMasked`). Only
// the fields the connect dialog target needs are typed here; the dialog
// fetches the full export payload (endpoint + plaintext key) on its own.
export type CreateDownstreamKeyResponse = {
  success?: boolean
  item?: Pick<DownstreamApiKeyItem, 'id' | 'name' | 'keyMasked'>
}

export const downstreamKeysQueryKeys = {
  all: ['downstream-keys'] as const,
  list: () => [...downstreamKeysQueryKeys.all, 'list'] as const,
}

export const CREATE_FORM_ID = 'settings-downstream-keys-create-form'

// Extract the backend error message from an axios rejection. Admin API
// errors serialize as { error: "..." } (handler/shared/errors.go APIError);
// .message is checked too, mirroring the http-client resolveResponseMessage
// order. Returns undefined for shapes without a usable string.
export function resolveApiErrorMessage(error: unknown): string | undefined {
  const data = (error as { response?: { data?: unknown } } | null)?.response
    ?.data
  if (data && typeof data === 'object') {
    const message = (data as { message?: unknown }).message
    if (typeof message === 'string' && message.trim()) return message
    const errorField = (data as { error?: unknown }).error
    if (typeof errorField === 'string' && errorField.trim()) return errorField
  }
  return undefined
}

export function generateDownstreamSkSuffix(): string {
  const bytes = new Uint8Array(48)
  crypto.getRandomValues(bytes)
  return Array.from(bytes, (byte) => byte.toString(16).padStart(2, '0')).join(
    ''
  )
}

export const createKeySchema = z.object({
  name: z.string().min(1, 'settings.downstream.keys.schema.nameRequired'),
  key: z.string().min(8, 'settings.downstream.keys.schema.keyMinLength'),
  groupName: z.string().optional(),
  maxRequests: z.coerce.number().int().min(0).optional(),
  maxCost: z.coerce.number().min(0).optional(),
  enabled: z.boolean().optional(),
  expiresAt: z.string().optional(),
  description: z.string().optional(),
  supportedModels: z.array(z.string().trim().min(1)).default([]),
  allowedSiteIds: z.array(z.number().int().positive()).default([]),
  allowedCredentialRefs: z.array(credentialRefSchema).default([]),
  excludedCredentialRefs: z.array(credentialRefSchema).default([]),
})

export type CreateKeyFormValues = z.infer<typeof createKeySchema>

// Edit mode reuses the create schema but drops the secret `key` field — the
// key value is never editable here (rotation is a separate action). The
// backend applies a PATCH-style partial update (only fields present in the
// request body are changed; the toggle path already relies on this), so the
// edit payload omits `key` and `description` to preserve both as-is.
export const editKeySchema = createKeySchema.omit({ key: true })

// datetime-local input format: "YYYY-MM-DDTHH:MM" in the browser's local
// timezone. The backend stores the value as-is when it cannot parse it as
// RFC3339, so create/edit round-trip the same local string.
function isoToLocalDatetimeInput(iso?: string | null): string {
  if (!iso) return ''
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return ''
  const pad = (value: number) => String(value).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`
}

/**
 * datetime-local input value ("YYYY-MM-DDTHH:MM", browser local time) to the
 * wire format the backend actually enforces.
 *
 * Why this exists: `expires_at` is only enforced when the server can parse it.
 * handler/admin/downstream_keys_normalize.go `normalizeExpiresAt` stores an
 * unparseable value verbatim, and auth/downstream.go `parseISO8601` skips the
 * expiry check entirely on a parse error (TS parity, locked by
 * auth/downstream_test.go TestExpiration_InvalidDateFormat). A bare
 * minute-precision local value parses nowhere in that chain — verified against
 * a live server: a key expiring in 2020 still served /v1/models with HTTP 200.
 * Both parsers accept RFC3339, which needs seconds + an offset, so that is
 * what we send.
 *
 * Empty stays empty on purpose: create stores ""/NULL and update normalizes
 * "" to NULL, both meaning "never expires" — and the update path relies on the
 * key being present in the body to clear an existing expiry.
 */
export function localDatetimeInputToIso(value: string): string {
  if (!value) return ''
  const date = new Date(value)
  // Unparseable input is passed through unchanged rather than dropped, so a
  // value this helper does not understand never silently becomes "no expiry".
  if (Number.isNaN(date.getTime())) return value
  return date.toISOString()
}

export function normalizeModelRules(value: unknown): string[] {
  let rawRules: unknown[] = []
  if (Array.isArray(value)) {
    rawRules = value
  } else if (typeof value === 'string' && value.trim()) {
    try {
      const parsed = JSON.parse(value) as unknown
      rawRules = Array.isArray(parsed) ? parsed : [value]
    } catch {
      rawRules = [value]
    }
  }

  const rules: string[] = []
  const seen = new Set<string>()
  for (const rawRule of rawRules) {
    if (typeof rawRule !== 'string') continue
    const rule = rawRule.trim()
    if (!rule || seen.has(rule)) continue
    if (rule === '*') return ['*']
    seen.add(rule)
    rules.push(rule)
  }
  return rules
}

export function extractMarketplaceModelNames(result: unknown): string[] {
  let rows: unknown = []
  if (Array.isArray(result)) {
    rows = result
  } else if (
    typeof result === 'object' &&
    result !== null &&
    'models' in result
  ) {
    rows = (result as { models?: unknown }).models
  }
  if (!Array.isArray(rows)) return []

  return normalizeModelRules(
    rows.map((row) =>
      typeof row === 'object' && row !== null && 'name' in row
        ? (row as { name?: unknown }).name
        : undefined
    )
  )
}

export function blankKeyFormValues(): CreateKeyFormValues {
  return {
    name: '',
    key: '',
    groupName: '',
    maxRequests: undefined,
    maxCost: undefined,
    enabled: true,
    expiresAt: '',
    description: '',
    supportedModels: [],
    allowedSiteIds: [],
    allowedCredentialRefs: [],
    excludedCredentialRefs: [],
  }
}

export function keyFormValuesFromItem(
  item: DownstreamApiKeyItem
): CreateKeyFormValues {
  return {
    ...blankKeyFormValues(),
    name: item.name,
    groupName: item.groupName ?? '',
    maxRequests: item.maxRequests ?? undefined,
    maxCost: item.maxCost ?? undefined,
    enabled: item.enabled,
    expiresAt: isoToLocalDatetimeInput(item.expiresAt),
    supportedModels: normalizeModelRules(item.supportedModels),
    allowedSiteIds: parseIdArray(item.allowedSiteIds),
    // GET returns the stored ref columns as raw JSON strings — parse them
    // back into typed refs for the tree pickers (round-trip contract).
    allowedCredentialRefs: parseCredentialRefs(item.allowedCredentialRefs),
    excludedCredentialRefs: parseCredentialRefs(item.excludedCredentialRefs),
  }
}
