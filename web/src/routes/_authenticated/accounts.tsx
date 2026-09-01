// metapi-go/routes — accounts list.
//
// `validateSearch` validates the URL search state consumed by the accounts
// page: `page` / `pageSize` (1-based), `q` / `status` / `site` strings, and the
// canonical comma-separated `sort` value. The page reads the raw search string
// through the shared URL-table hook and writes changes back through TanStack
// Router, so direct links and back/forward restore the same table view.
//
//
// loader prefetches the ONE server-paginated accounts page the deep link
// points at (page/pageSize parsed from the URL, same default rules as the
// page). The paged response also embeds sites, so the site filter is
// covered without fetching the full account fleet. The component is
// declared directly; the router autoCodeSplitting splits it in production.

import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'

import { accountQueryKeys, fetchAccountsPage } from '@/features/accounts'
import { AccountsPage } from '@/features/accounts/components/accounts-page'
import {
  encodeSortingParam,
  stringSearchParam,
} from '@/lib/helpers/searchParams'

// Tolerant URL search contract: TanStack Router JSON-parses search values, so
// `?q=123` arrives as a number and `?status=true` as a boolean. The string
// fields accept all three primitives (normalized to string by the page via
// `asStringParam`), and the numerics fall back to sane defaults instead of
// throwing a route error on stale bookmarks / legacy URLs.
//
// `siteId` / `create` are the one-shot deep-link params written by the sites
// guided-flow CTA (`/accounts?siteId=…&create=1`): `siteId` preseeds the
// referenced site in the create dialog, and `create` opens it once before the
// page neutralizes both. `segment` is the optional credential-mode hint of
// the same flow (`segment=apikey` from the "添加 API Key" CTA — the dialog
// opens in apikey mode instead of the session default). `accountId` is the
// one-shot deep-link param written by the dashboard attention items
// (`/accounts?accountId=…` for expired / low-balance accounts): the page
// opens the referenced account's detail sheet once before neutralizing it.
// They accept router-parsed primitives (`create=1` arrives as number `1`)
// and degrade to `undefined` on stale/malformed values.
export const accountsSearchSchema = z.object({
  page: z.coerce.number().int().positive().catch(1).default(1),
  pageSize: z.coerce.number().int().min(1).max(100).catch(20).default(20),
  q: stringSearchParam,
  sort: z
    .union([
      z.string(),
      z.array(z.object({ id: z.string(), desc: z.boolean() })),
    ])
    .optional()
    .transform((value) => encodeSortingParam(value))
    .catch(undefined),
  status: stringSearchParam,
  site: stringSearchParam,
  siteId: z.coerce.number().int().positive().optional().catch(undefined),
  accountId: z.coerce.number().int().positive().optional().catch(undefined),
  create: z.coerce.boolean().optional().catch(undefined),
  segment: z.string().optional().catch(undefined),
})

export const Route = createFileRoute('/_authenticated/accounts')({
  validateSearch: accountsSearchSchema,
  staticData: { title: 'accounts.page.title' },
  loader: async ({ context, location }) => {
    const params = new URLSearchParams(location.searchStr)
    const rawPage = Number(params.get('page') ?? '1')
    const rawPageSize = Number(params.get('pageSize') ?? '20')
    const pageIndex =
      Number.isFinite(rawPage) && rawPage > 0 ? Math.max(0, rawPage - 1) : 0
    const pageSize =
      Number.isFinite(rawPageSize) && rawPageSize > 0
        ? Math.min(100, Math.max(1, rawPageSize))
        : 20
    // Server-side filters prefetched with the same keys the page query uses
    // (#1108): q/status/site straight from the deep-linked URL.
    const q = params.get('q') ?? ''
    const status = params.get('status') ?? ''
    const site = params.get('site') ?? ''
    await context.queryClient.prefetchQuery({
      queryKey: accountQueryKeys.page(pageIndex, pageSize, q, status, site),
      queryFn: () =>
        fetchAccountsPage({ pageIndex, pageSize, q, status, site }),
    })
  },
  component: AccountsPage,
})
