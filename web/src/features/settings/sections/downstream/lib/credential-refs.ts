// metapi-go/features/settings/sections/downstream/lib — credential-ref
// policy helpers for downstream API keys (#1026 UI follow-up).
//
// Contract SSOT: docs/api.md → Downstream API Keys → Credential & site scope.
//   - a ref is {kind, siteId, accountId, tokenId?} with
//     kind ∈ account_token | default_api_key
//   - GET responses return the stored columns as raw JSON strings (or null)
//     → parseCredentialRefs
//   - create/update request bodies use real arrays → serializeCredentialRefs
//   - empty/omitted means "unrestricted" for that dimension

import { z } from 'zod'

/**
 * Form-level schema (create/edit sheet). The discriminated union encodes the
 * two legal shapes: account_token requires a positive tokenId,
 * default_api_key carries none. The picker only produces conforming refs;
 * the schema keeps hand-imported/legacy values honest before submit.
 */
export const credentialRefSchema = z.discriminatedUnion('kind', [
  z.object({
    kind: z.literal('account_token'),
    siteId: z.number().int().positive(),
    accountId: z.number().int().positive(),
    tokenId: z.number().int().positive(),
  }),
  z.object({
    kind: z.literal('default_api_key'),
    siteId: z.number().int().positive(),
    accountId: z.number().int().positive(),
  }),
])

/** Single source of truth: the type is exactly the schema's inference. */
export type CredentialRef = z.infer<typeof credentialRefSchema>

/** Stable identity used for dedupe and selection bookkeeping. */
export function credentialRefKey(ref: CredentialRef): string {
  return ref.kind === 'account_token'
    ? `account_token:${ref.siteId}:${ref.accountId}:${ref.tokenId}`
    : `default_api_key:${ref.siteId}:${ref.accountId}`
}

function toPositiveInt(value: unknown): number | null {
  return typeof value === 'number' && Number.isInteger(value) && value > 0
    ? value
    : null
}

/**
 * Normalize one stored entry. Invalid entries return null (dropped on READ —
 * the backend rejects them on WRITE, so a stored row can only carry garbage
 * from pre-validation legacy data; the UI must not crash on it). Entries
 * persisted by the legacy TS version without a kind are treated with
 * default_api_key semantics, mirroring the backend's read compatibility.
 */
function normalizeEntry(raw: unknown): CredentialRef | null {
  if (raw === null || typeof raw !== 'object' || Array.isArray(raw)) {
    return null
  }
  const entry = raw as Record<string, unknown>
  const siteId = toPositiveInt(entry.siteId)
  const accountId = toPositiveInt(entry.accountId)
  if (siteId === null || accountId === null) return null

  const kind =
    entry.kind === undefined || entry.kind === null
      ? 'default_api_key'
      : entry.kind
  if (kind === 'account_token') {
    const tokenId = toPositiveInt(entry.tokenId)
    if (tokenId === null) return null
    return { kind, siteId, accountId, tokenId }
  }
  if (kind === 'default_api_key') {
    return { kind, siteId, accountId }
  }
  return null
}

/**
 * Parse the stored column value into refs. Accepts null/undefined/empty
 * (→ unrestricted → []), a parsed array, or the raw JSON string the GET
 * endpoints return. Malformed entries are dropped; duplicates collapse.
 */
export function parseCredentialRefs(value: unknown): CredentialRef[] {
  let rows: unknown[] = []
  if (Array.isArray(value)) {
    rows = value
  } else if (typeof value === 'string' && value.trim()) {
    try {
      const parsed = JSON.parse(value) as unknown
      rows = Array.isArray(parsed) ? parsed : []
    } catch {
      rows = []
    }
  }

  const seen = new Set<string>()
  const refs: CredentialRef[] = []
  for (const row of rows) {
    const ref = normalizeEntry(row)
    if (!ref) continue
    const key = credentialRefKey(ref)
    if (seen.has(key)) continue
    seen.add(key)
    refs.push(ref)
  }
  return refs
}

/**
 * Canonicalize refs for the wire: dedupe, stable order (siteId, accountId,
 * kind, tokenId), and exact shapes (no tokenId key on default_api_key).
 * An empty result means "unrestricted" — callers send it as [] and the
 * backend stores NULL.
 */
export function serializeCredentialRefs(
  refs: readonly CredentialRef[]
): CredentialRef[] {
  const seen = new Set<string>()
  const unique: CredentialRef[] = []
  for (const ref of refs) {
    const key = credentialRefKey(ref)
    if (seen.has(key)) continue
    seen.add(key)
    unique.push(
      ref.kind === 'account_token'
        ? {
            kind: ref.kind,
            siteId: ref.siteId,
            accountId: ref.accountId,
            tokenId: ref.tokenId,
          }
        : { kind: ref.kind, siteId: ref.siteId, accountId: ref.accountId }
    )
  }
  return unique.sort((left, right) => {
    if (left.siteId !== right.siteId) return left.siteId - right.siteId
    if (left.accountId !== right.accountId) {
      return left.accountId - right.accountId
    }
    if (left.kind !== right.kind) return left.kind.localeCompare(right.kind)
    return credentialRefTokenId(left) - credentialRefTokenId(right)
  })
}

function credentialRefTokenId(ref: CredentialRef): number {
  return ref.kind === 'account_token' ? ref.tokenId : 0
}

/**
 * Parse a stored ID-array column (allowedSiteIds / excludedSiteIds /
 * allowedRouteIds). GET responses return these as raw JSON strings (or
 * null); legacy rows may already be arrays. Invalid / non-positive entries
 * are dropped; duplicates collapse. Mirrors the previous keys-section
 * normalizeSiteIds/normalizeRouteIds behavior.
 */
export function parseIdArray(value: unknown): number[] {
  let rows: unknown[] = []
  if (Array.isArray(value)) {
    rows = value
  } else if (typeof value === 'string' && value.trim()) {
    try {
      const parsed = JSON.parse(value) as unknown
      rows = Array.isArray(parsed) ? parsed : []
    } catch {
      rows = []
    }
  }

  const seen = new Set<number>()
  const ids: number[] = []
  for (const row of rows) {
    if (typeof row !== 'number' || !Number.isInteger(row) || row <= 0) {
      continue
    }
    if (seen.has(row)) continue
    seen.add(row)
    ids.push(row)
  }
  return ids
}
