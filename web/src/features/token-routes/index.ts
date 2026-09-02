// metapi-go features/token-routes — public barrel.
//
// Consumers should import only from here:
//   import { RoutesPage, useRoutes, type RouteSummaryRow } from '@/features/token-routes'
//
// `export type` is used for all type-only re-exports (isolatedModules-safe).

export {
  routeQueryKeys,
  useRebuildRoutes,
  useRefreshRouteDecisions,
  // Dashboard onboarding checklist reads the route count off the same query
  // key + queryFn shape as this page, so the summary is fetched once and
  // shared instead of poisoned by a count-only variant of the same key.
  useRoutes,
} from './api'
export { routesSearchSchema } from './lib/routes-schema'
