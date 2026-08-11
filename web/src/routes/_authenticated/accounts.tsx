// metapi-go/routes — accounts list.
//
// `validateSearch` validates the URL search state the accounts page reads via
// `window.location.search`: `page` / `pageSize` (1-based, coerced from the
// string URL), and `q` / `status` / `site` (strings; `status` + `site` are
// comma-separated lists the page splits itself). Keeping them as strings
// (not arrays) preserves the URL shape the page writes via
// `history.replaceState`, so no search-param normalization rewrites the URL.
//
// `loader` prefetches the accounts snapshot (`accountQueryKeys.snapshot()`),
// which the backend returns as `{ accounts, sites, generatedAt }` — the
// embedded `sites` array powers the page's site filter, so a single prefetch
// covers both. `lazyRouteComponent` code-splits the page.

import { createFileRoute, lazyRouteComponent } from '@tanstack/react-router'
import { z } from 'zod'

import { accountQueryKeys } from '@/features/accounts'
import { api } from '@/lib/api'

const accountsSearchSchema = z.object({
  page: z.coerce.number().int().positive().optional(),
  pageSize: z.coerce.number().int().min(1).max(200).optional(),
  q: z.string().optional(),
  status: z.string().optional(),
  site: z.string().optional(),
})

export const Route = createFileRoute('/_authenticated/accounts')({
  validateSearch: accountsSearchSchema,
  loader: async ({ context }) => {
    await context.queryClient.prefetchQuery({
      queryKey: accountQueryKeys.snapshot(),
      queryFn: () => api.getAccountsSnapshot(),
    })
  },
  component: lazyRouteComponent(
    () => import('@/features/accounts/components/accounts-page'),
    'AccountsPage'
  ),
})
