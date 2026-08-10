// metapi-go/routes — token routes list.
//
// `validateSearch` uses `routesSearchSchema` (the URL-state contract defined
// by the token-routes feature) so the search params are typed for
// `useSearch()`. The page also reads `window.location.search` directly (it
// navigates with string hrefs), and TanStack Router does not rewrite the
// URL based on `validateSearch` transforms unless `search.strict` is enabled,
// so the page's comma-separated filter strings stay intact.
//
// `loader` prefetches the route summary (`routeQueryKeys.summary()`) plus
// the sites list and accounts snapshot — the two lookups that power the
// page's site/account filters — so first paint renders with data. The three
// queries run in parallel. `lazyRouteComponent` code-splits the page.

import { createFileRoute, lazyRouteComponent } from '@tanstack/react-router'

import { api } from '@/lib/api'
import { accountQueryKeys } from '@/features/accounts'
import { sitesKeys } from '@/features/sites'
import { routeQueryKeys, routesSearchSchema } from '@/features/token-routes'

export const Route = createFileRoute('/_authenticated/token-routes')({
  validateSearch: routesSearchSchema,
  loader: async ({ context }) => {
    await Promise.all([
      context.queryClient.prefetchQuery({
        queryKey: routeQueryKeys.summary(),
        queryFn: () => api.getRoutesSummary(),
      }),
      context.queryClient.prefetchQuery({
        queryKey: sitesKeys.list(),
        queryFn: () => api.getSites(),
      }),
      context.queryClient.prefetchQuery({
        queryKey: accountQueryKeys.snapshot(),
        queryFn: () => api.getAccountsSnapshot(),
      }),
    ])
  },
  component: lazyRouteComponent(
    () => import('@/features/token-routes/components/routes-page'),
    'RoutesPage',
  ),
})
