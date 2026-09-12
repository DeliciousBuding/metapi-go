// metapi-go/features/token-routes — public barrel.
//
// Consumers should import only from here:
//   import { useRoutes, useRebuildRoutes, routesSearchSchema } from '@/features/token-routes'
//
// `RoutesPage` is deliberately not exported: `routes/_authenticated/token-routes.tsx`
// loads it directly (route files are the composition root), so a feature that
// only needs the contract below does not pull the page into its own chunk.
// `RouteSummaryRow` and the other route-decision types are not here either —
// lib helpers need them too, so they live in
// `@/lib/helpers/token-route-contract`.

export {
  routeQueryKeys,
  useClearRouteCooldown,
  useRebuildRoutes,
  useRefreshRouteDecisions,
  // Dashboard onboarding checklist reads the route count off the same query
  // key + queryFn shape as this page, so the summary is fetched once and
  // shared instead of poisoned by a count-only variant of the same key.
  useRoutes,
} from './api'
export { routesSearchSchema } from './lib/routes-schema'

// Rebuild handoff: an acknowledged rebuild stores its task reference so the
// routes page can pick the task up after navigation instead of showing a stale
// table. The writer is this feature's own `api.ts` (via `useRebuildRoutes`
// above), not the section that triggered the rebuild. These two symbols are
// published because the settings allowlist section's test asserts and resets
// the stored reference from outside the feature; no production code outside
// `features/token-routes` reads the raw storage key.
export {
  ROUTE_REBUILD_STORAGE_KEY,
  rememberRouteRebuild,
} from './lib/route-rebuild-reference'
