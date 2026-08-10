// metapi-go features/token-routes — public barrel.
//
// Consumers should import only from here:
//   import { RoutesPage, useRoutes, type RouteSummaryRow } from '@/features/token-routes'
//
// `export type` is used for all type-only re-exports (isolatedModules-safe).

// --- page + components ---
export { RoutesPage } from './components/routes-page'
export { RouteFormDialog } from './components/route-form-dialog'
export type { RouteAccountOption } from './components/route-form-dialog'
export { RouteDetailSheet } from './components/route-detail-sheet'
export { showRouteCompletionToast } from './components/route-completion-toast'
export { useRoutesColumns } from './components/routes-columns'

// --- route hooks + query keys ---
export {
  routeQueryKeys,
  useRoutes,
  useModelTokenCandidates,
  useRouteChannels,
  useCreateRoute,
  useUpdateRoute,
  useDeleteRoute,
  useBatchUpdateRoutes,
  useClearRouteCooldown,
  useBatchAddChannels,
  useRebuildRoutes,
  useRefreshRouteDecisions,
  useZeroChannelRoutes,
  selectRouteById,
} from './api'
export type {
  ModelTokenCandidatesResponse,
  CreateRouteResult,
  BatchRouteAction,
  BatchAddChannelsResult,
  RebuildRoutesResult,
} from './api'

// --- route entity types + runtime schemas ---
export type {
  RouteRow,
  RouteSummaryRow,
  RouteChannel,
  RouteChannelDraft,
  ChannelRef,
  RouteFormPayload,
  RouteMode,
  RouteRoutingStrategy,
  RouteRowKind,
  RouteDecision,
  RouteDecisionCandidate,
  RouteRowActions,
  RoutesDialogType,
} from './types'

// --- pattern + presentation helpers ---
export {
  isExactModelPattern,
  isRegexModelPattern,
  matchesModelPattern,
  getModelPatternError,
  parseRegexModelPattern,
  normalizeRouteMode,
  isExplicitGroupRoute,
  resolveRouteTitle,
  normalizeRouteDisplayIconValue,
  resolveRouteIcon,
  formatContextLength,
  normalizeRoutingStrategy,
  routingStrategyLabel,
  dedupeChannelDrafts,
  ROUTE_BRAND_ICON_PREFIX,
  ROUTE_ICON_NONE_VALUE,
} from './utils'
export type { ResolvedRouteIcon } from './utils'

// --- route form schema ---
export {
  getRouteFormSchema,
  getRouteFormDefaultValues,
  transformFormToPayload,
  transformRouteToFormValues,
  routesSearchSchema,
} from './lib/routes-schema'
export type { RouteFormValues, RoutesSearch } from './lib/routes-schema'
