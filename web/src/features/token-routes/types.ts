// metapi-go features/token-routes — entity types for the route configuration
// domain. Mirrors the legacy `web/pages/token-routes/types.ts` shapes but is
// self-contained: the shared `tokenRouteContract` / `tokenRoutePatterns`
// modules are not yet ported into `web/src/shared/`, so the route-mode enum,
// the decision-snapshot projection, and the pattern helpers live here and in
// `utils.ts` until those shared modules land.
//
// S5 boundary inversion: the summary-row contract shared with
// `@/lib/helpers/zeroChannelRoutes` moved to
// `@/lib/helpers/token-route-contract` (lib ↛ features, rule 1); this file
// re-exports it so existing consumers keep their import paths.
//
// The summary row is what `GET /api/routes/summary` returns and what the
// data-table list renders. The full `RouteRow` (with channels) is what
// `GET /api/routes/:id/channels` returns and what the detail sheet renders.
// Zero-channel placeholder rows (built by `buildZeroChannelPlaceholderRoutes`
// in `@/lib/helpers/zeroChannelRoutes`) reuse `RouteSummaryRow` with
// `kind: 'zero_channel'` and a stable negative id.

import type {
  RouteDecision,
  RouteMode,
  RouteRoutingStrategy,
  RouteSummaryRow,
} from '@/lib/helpers/token-route-contract'

export type {
  RouteDecision,
  RouteMode,
  RouteRoutingStrategy,
  RouteSummaryRow,
} from '@/lib/helpers/token-route-contract'

// Channel-level strategy for OAuth route units (re-exported for the detail
// sheet; the canonical `OAuthRouteUnitStrategy` also lives in `@/lib/api`).
type OAuthRouteUnitStrategy = 'round_robin' | 'stick_until_unavailable'

// ---------------------------------------------------------------------------
// Route channel — the per-route account+token binding (detail view + form)
// ---------------------------------------------------------------------------

type RouteChannelRouteUnitMember = {
  accountId: number
  username: string | null
  siteName: string | null
}

type RouteChannelRouteUnit = {
  id: number | string
  name: string | null
  strategy: OAuthRouteUnitStrategy
  memberCount: number
  members?: RouteChannelRouteUnitMember[]
}

type RouteChannelAccount = {
  username: string | null
  accessToken?: string | null
  extraConfig?: string | null
  credentialMode?: string | null
}

type RouteChannelSite = {
  id: number
  name: string | null
  platform: string | null
}

type RouteChannelToken = {
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
// Channel ref — lightweight channel reference used by the form draft and the
// "add channels" flow. Mirrors the backend `batchAddChannels` payload shape.
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

export interface RouteRowActions {
  onEdit: (route: RouteSummaryRow) => void
  onDelete: (route: RouteSummaryRow) => void
  onToggleEnabled: (route: RouteSummaryRow) => void
  onViewDetail: (route: RouteSummaryRow) => void
  onClearCooldown: (route: RouteSummaryRow) => void
  onRefreshDecision: (route: RouteSummaryRow) => void
}
