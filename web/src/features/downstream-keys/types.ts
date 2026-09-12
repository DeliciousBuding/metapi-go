// metapi-go/features/downstream-keys — the downstream API-key wire contract.
//
// This lives in the feature that owns the domain: the dashboard onboarding
// checklist and the token-routes "next step" strip read the same key list
// through the barrel, so the contract cannot sit inside one consumer's UI.
//
// Types and query keys only — no React and no fetching; the calls themselves go
// through `@/lib/api`.

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
