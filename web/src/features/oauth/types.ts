// metapi-go/features/oauth — domain types for the OAuth management feature.
//
// Wraps the OAuth connection + provider types from `lib/api.ts` (the
// backend contract) and adds the feature-level query-key factory plus the
// start-authorization payload type. `OAuthClient` is the row type for the
// connections table — it is `OAuthConnectionInfo` from the API (an
// OAuth-connected upstream account). `OAuthProvider` is an available
// upstream provider (e.g. the platform whose OAuth flow can be started).

import type {
  OAuthConnectionInfo,
  OAuthProviderInfo,
  OAuthStartResponse,
} from '@/lib/api'

/** Row type for the OAuth connections table (an OAuth-connected account). */
export type OAuthClient = OAuthConnectionInfo

/** An available OAuth provider (whose authorization flow can be started). */
export type OAuthProvider = OAuthProviderInfo

/** Result of starting an OAuth flow — contains the URL to open. */
export type OAuthClientStatus = OAuthClient['status']

/**
 * Manual-callback instructions returned by `startOAuthProvider`. Surfaced to
 * the user in the pending panel: the SSH tunnel command (if any) and the
 * callback URL the operator may paste back manually.
 */
export type OAuthStartInstructions = OAuthStartResponse['instructions']

/**
 * Wire payload for `api.startOAuthProvider`. `provider` is required;
 * `projectId` is required only when the selected provider's
 * `requiresProjectId` is true (enforced in the dialog, not the schema).
 */
export type OAuthStartPayload = {
  provider: string
  projectId?: string
  proxyUrl?: string | null
  useSystemProxy?: boolean
}

/**
 * TanStack Query key factory. Centralised so invalidation is grep-able and
 * the keys stay stable across hooks.
 */
export const oauthKeys = {
  all: ['oauth'] as const,
  providers: () => [...oauthKeys.all, 'providers'] as const,
  connections: () => [...oauthKeys.all, 'connections'] as const,
  session: (state: string) => [...oauthKeys.all, 'session', state] as const,
}
