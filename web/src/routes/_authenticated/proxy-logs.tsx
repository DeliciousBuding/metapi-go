// metapi-go/routes — proxy logs list.
//
// `validateSearch` uses `proxyLogsSearchSchema` (the URL-state contract for
// the proxy-logs page). The schema's `sort` field transforms the
// comma-separated URL string into a SortingState array for the validated
// `search` object; TanStack Router (non-strict default) does not rewrite the
// URL on transforms, so the page's own `window.location.search` reads stay
// consistent.
//
// `loader` prefetches both the proxy-logs page (`proxyLogsKeys.list(...)`)
// and the meta facets (`proxyLogsKeys.meta(...)`) the page's `useProxyLogs`
// / `useProxyLogsMeta` will request, building the same `ProxyLogsQuery`
// payload from the router's `location.searchStr` (SSR-safe, rather than the
// global `window.location.search`) so the prefetched pages are reused and the
// cache key matches the page's first fetch. Latency range is client-side only
// (not part of the backend query) so it is intentionally absent from the
// prefetch payload.

import { createFileRoute } from '@tanstack/react-router'

import {
  proxyLogsKeys,
  proxyLogsSearchSchema,
  type ProxyLogsSearch,
} from '@/features/proxy-logs'
import { ProxyLogsPage } from '@/features/proxy-logs/components/proxy-logs-page'
import { api } from '@/lib/api'
import { asStringParam } from '@/lib/helpers/searchParams'

const DEFAULT_PROXY_LOGS_PAGE_SIZE = 20

/**
 * Safe-parse a raw search string with the feature schema. Returns `null` on a
 * malformed string so the loader can skip the prefetch (the page owns the
 * fallback). Pure — no `window` access — so it is shared by the loader (router
 * `location.searchStr`) and mirrors the page's `readUrlState` approach.
 */
function parseProxyLogsSearch(searchStr: string): ProxyLogsSearch | null {
  const entries = Object.fromEntries(
    new URLSearchParams(
      searchStr.startsWith('?') ? searchStr.slice(1) : searchStr
    ).entries()
  )
  const parsed = proxyLogsSearchSchema.safeParse(entries)
  return parsed.success ? parsed.data : null
}

export const Route = createFileRoute('/_authenticated/proxy-logs')({
  validateSearch: proxyLogsSearchSchema,
  staticData: { title: 'proxyLogs.page.title' },
  loader: async ({ context, location }) => {
    const search = parseProxyLogsSearch(location.searchStr)
    if (!search) return

    const pageIndex = search.page ?? 0
    const pageSize = search.pageSize ?? DEFAULT_PROXY_LOGS_PAGE_SIZE
    const status = search.status === 'all' ? undefined : search.status
    const searchText = asStringParam(search.q)?.trim() || undefined
    const siteId = search.siteId ?? undefined
    const client = asStringParam(search.client) || undefined
    const from = asStringParam(search.from) || undefined
    const to = asStringParam(search.to) || undefined

    const queryPayload = {
      limit: pageSize,
      offset: pageIndex * pageSize,
      status,
      search: searchText,
      siteId,
      client,
      from,
      to,
    }
    const metaPayload = { status, search: searchText, siteId, client, from, to }

    await Promise.all([
      context.queryClient.prefetchQuery({
        queryKey: proxyLogsKeys.list(queryPayload),
        queryFn: () => api.getProxyLogs(queryPayload),
      }),
      context.queryClient.prefetchQuery({
        queryKey: proxyLogsKeys.meta(metaPayload),
        queryFn: () => api.getProxyLogsMeta(metaPayload),
      }),
    ])
  },
  component: ProxyLogsPage,
})
