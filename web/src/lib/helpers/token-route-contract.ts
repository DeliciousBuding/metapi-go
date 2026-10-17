// metapi-go/lib — token-route summary contract.
//
// Data contract shared by the token-routes feature and lib helpers
// (zeroChannelRoutes builds placeholder rows of this shape). Pure types —
// rule 5 of docs/internal/web-package-boundaries.md. The feature keeps
// re-exporting them for its existing consumers (features/token-routes/
// types.ts); richer feature-owned types (RouteRow, RouteChannel, …) stay
// in the feature.

type RouteRowKind = 'persisted' | 'zero_channel'

export type RouteMode = 'pattern' | 'explicit_group'

export type RouteRoutingStrategy =
  | 'weighted'
  | 'round_robin'
  | 'stable_first'
  | 'least_busy'
  | 'lowest_latency'
  | 'lowest_cost'

// ---------------------------------------------------------------------------
// Decision snapshot — a defensive projection of the server's RouteDecision.
// The backend payload is loosely typed, so only the fields the UI touches are
// declared; everything else falls back to `unknown`. When the shared contract
// module lands, this can be replaced by `import type { RouteDecision }`.
// ---------------------------------------------------------------------------

type RouteDecisionCandidate = {
  candidateRef?: {
    kind: 'account' | 'direct' | 'oauth_unit'
    id: number
    channelId?: number
    groupId?: number
    grantId?: number
    credentialId?: number
    accountId?: number
    siteId?: number
    tokenId?: number
  }
  label?: string
  actualModel?: string
  eligibility?: 'eligible' | 'blocked' | 'notEvaluated'
  blockers?: Array<{ code: string; message: string }>
  probabilityEvaluated?: boolean
  /** RFC3339 cooling deadline when the candidate is cooled — route_channels
   *  rows for account/oauth_unit candidates, upstream_grants for direct
   *  grant members. Shown beside the kind-dispatched recovery action. */
  cooldownUntil?: string | null
  oauthMembers?: Array<{
    memberId: number
    accountId: number
    siteId: number
    username: string
    siteName: string
    eligibility: 'eligible' | 'blocked' | 'notEvaluated'
    blockers: Array<{ code: string; message: string }>
    cooldownUntil?: string | null
  }> | null
  channelId?: number
  accountId?: number
  tokenId?: number | null
  username?: string | null
  siteName?: string | null
  sourceModel?: string | null
  probability?: number
  weight?: number
  priority?: number
  reasonText?: string | null
  reasonColor?: string | null
  disabled?: boolean
}

export type RouteDecision = {
  mode?: 'adminPreview'
  requestedModel?: string
  actualModel?: string
  matched?: boolean
  routeId?: number
  modelPattern?: string
  checkedAt?: string
  requiredUpstreamProtocol?: number
  allowUpstreamProtocolConversion?: boolean
  notEvaluated?: string[]
  routingStrategy?: RouteRoutingStrategy
  summary?: string[]
  model?: string
  matchedRouteId?: number | null
  matchedRoutePattern?: string | null
  candidates?: RouteDecisionCandidate[]
  selectedChannelId?: number | null
  reasonText?: string | null
  generatedAt?: string | null
  [key: string]: unknown
}

// ---------------------------------------------------------------------------
// Route summary row — list row (GET /api/routes/summary)
// ---------------------------------------------------------------------------

export type RouteSummaryRow = {
  id: number
  modelPattern: string
  displayName: string | null
  displayIcon: string | null
  routeMode?: RouteMode | null
  sourceRouteIds?: number[]
  modelMapping: string | null
  routingStrategy?: RouteRoutingStrategy | null
  contextLength?: number | null
  enabled: boolean
  channelCount: number
  enabledChannelCount: number
  siteNames: string[]
  decisionSnapshot: RouteDecision | null
  decisionRefreshedAt: string | null
  kind?: RouteRowKind
  readOnly?: boolean
  isVirtual?: boolean
}
