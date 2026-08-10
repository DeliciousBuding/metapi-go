// metapi-go features/token-routes — entity types for the route configuration
// domain. Mirrors the legacy `web/pages/token-routes/types.ts` shapes but is
// self-contained: the shared `tokenRouteContract` / `tokenRoutePatterns`
// modules are not yet ported into `web/src/shared/`, so the route-mode enum,
// the decision-snapshot projection, and the pattern helpers live here and in
// `utils.ts` until those shared modules land.
//
// The summary row is what `GET /api/routes/summary` returns and what the
// data-table list renders. The full `RouteRow` (with channels) is what
// `GET /api/routes/:id/channels` returns and what the detail sheet renders.
// Zero-channel placeholder rows (built by `buildZeroChannelPlaceholderRoutes`
// in `@/lib/helpers/zeroChannelRoutes`) reuse `RouteSummaryRow` with
// `kind: 'zero_channel'` and a stable negative id.

export type RouteRowKind = 'persisted' | 'zero_channel'

export type RouteMode = 'pattern' | 'explicit_group'

export type RouteRoutingStrategy = 'weighted' | 'round_robin' | 'stable_first'

// Channel-level strategy for OAuth route units (re-exported for the detail
// sheet; the canonical `OAuthRouteUnitStrategy` also lives in `@/lib/api`).
export type OAuthRouteUnitStrategy = 'round_robin' | 'stick_until_unavailable'

// ---------------------------------------------------------------------------
// Decision snapshot — a defensive projection of the server's RouteDecision.
// The backend payload is loosely typed, so only the fields the UI touches are
// declared; everything else falls back to `unknown`. When the shared contract
// module lands, this can be replaced by `import type { RouteDecision }`.
// ---------------------------------------------------------------------------

export type RouteDecisionCandidate = {
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
// Route channel — the per-route account+token binding (detail view + form)
// ---------------------------------------------------------------------------

export type RouteChannelRouteUnitMember = {
  accountId: number
  username: string | null
  siteName: string | null
}

export type RouteChannelRouteUnit = {
  id: number | string
  name: string | null
  strategy: OAuthRouteUnitStrategy
  memberCount: number
  members?: RouteChannelRouteUnitMember[]
}

export type RouteChannelAccount = {
  username: string | null
  accessToken?: string | null
  extraConfig?: string | null
  credentialMode?: string | null
}

export type RouteChannelSite = {
  id: number
  name: string | null
  platform: string | null
}

export type RouteChannelToken = {
  id: number
  name: string
  accountId: number
  enabled: boolean
  isDefault: boolean
}

export type RouteChannel = {
  id: number
  routeId?: number
  accountId: number
  tokenId: number | null
  sourceModel?: string | null
  priority: number
  weight: number
  enabled: boolean
  manualOverride: boolean
  successCount: number
  failCount: number
  cooldownUntil?: string | null
  account?: RouteChannelAccount
  site?: RouteChannelSite
  token?: RouteChannelToken | null
  oauthRouteUnitId?: number | null
  routeUnit?: RouteChannelRouteUnit | null
}

// ---------------------------------------------------------------------------
// Route row — full route with channels (detail view)
// ---------------------------------------------------------------------------

export type RouteRow = {
  id: number
  modelPattern: string
  displayName?: string | null
  displayIcon?: string | null
  routeMode?: RouteMode | null
  sourceRouteIds?: number[]
  modelMapping?: string | null
  routingStrategy?: RouteRoutingStrategy | null
  contextLength?: number | null
  decisionSnapshot?: RouteDecision | null
  decisionRefreshedAt?: string | null
  enabled: boolean
  channels: RouteChannel[]
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

// ---------------------------------------------------------------------------
// Channel ref — lightweight channel reference used by the form draft and the
// "add channels" flow. Mirrors the backend `batchAddChannels` payload shape.
// ---------------------------------------------------------------------------

export type ChannelRef = {
  accountId: number
  tokenId?: number
  sourceModel?: string
}

export type RouteChannelDraft = ChannelRef

// ---------------------------------------------------------------------------
// Create / update payload (POST/PUT /api/routes). The backend `addRoute` /
// `updateRoute` accept sparse `any` bodies, so `routingStrategy` and
// `modelMapping` are sent alongside the core legacy keys even though the
// legacy UI only wrote them via separate partial updates — the server treats
// unknown/extra keys gracefully.
// ---------------------------------------------------------------------------

export type RouteFormPayload = {
  routeMode: RouteMode
  modelPattern?: string
  displayName?: string
  displayIcon?: string
  contextLength?: number | null
  sourceRouteIds?: number[]
  routingStrategy?: RouteRoutingStrategy
  modelMapping?: string
}

// ---------------------------------------------------------------------------
// Dialog state machine (mirrors the accounts feature's union-typed state)
// ---------------------------------------------------------------------------

export type RoutesDialogType = 'create' | 'edit' | 'delete' | null

// ---------------------------------------------------------------------------
// Row action callbacks handed to the columns hook
// ---------------------------------------------------------------------------

export interface RouteRowActions {
  onEdit: (route: RouteSummaryRow) => void
  onDelete: (route: RouteSummaryRow) => void
  onToggleEnabled: (route: RouteSummaryRow) => void
  onViewDetail: (route: RouteSummaryRow) => void
  onClearCooldown: (route: RouteSummaryRow) => void
  onRefreshDecision: (route: RouteSummaryRow) => void
}
