// metapi-go/routes — sites list.
//
// `validateSearch` uses `sitesSearchSchema` (the URL-state contract defined by
// the sites feature) so the search params are typed and normalized for
// `useSearch()`. The sites page still reads `window.location.search` directly
// (it navigates with string hrefs), and TanStack Router does not rewrite the
// URL based on `validateSearch` transforms unless `search.strict` is enabled,
// so the page's comma-separated `sort` string stays intact.
//
// `loader` prefetches the sites list (`sitesKeys.list()`) so the page renders
// with data on first paint. The component is declared directly; the router
// plugin's `autoCodeSplitting` splits it in production.

import { createFileRoute } from '@tanstack/react-router'

import { sitesKeys, sitesSearchSchema } from '@/features/sites'
import { SitesPage } from '@/features/sites/components/sites-page'
import { api } from '@/lib/api'

export const Route = createFileRoute('/_authenticated/sites')({
  validateSearch: sitesSearchSchema,
  loader: async ({ context }) => {
    await context.queryClient.prefetchQuery({
      queryKey: sitesKeys.list(),
      queryFn: () => api.getSites(),
    })
  },
  component: SitesPage,
})
