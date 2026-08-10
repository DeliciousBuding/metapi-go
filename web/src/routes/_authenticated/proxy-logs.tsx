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
// payload from the URL search so the prefetched pages are reused. The
// loader reads `window.location.search` and safe-parses it with
// `proxyLogsSearchSchema` (the same schema the page uses) because TanStack
// Router's loader context does not expose the validated `search` object in
// this version; on a malformed URL the prefetch is skipped and the page
// fetches on mount (its own safe-parse falls back to defaults). Latency
// range is client-side only (not part of the backend query) so it is
// intentionally absent from the prefetch payload.

import { createFileRoute, lazyRouteComponent } from '@tanstack/react-router'

import { api } from '@/lib/api'
import {
  proxyLogsKeys,
  proxyLogsSearchSchema,
  type ProxyLogsSearch,
} from '@/features/proxy-logs'

const DEFAULT_PROXY_LOGS_PAGE_SIZE = 20

/**
 * Read the proxy-logs URL search state via the feature schema. Returns
 * `null` on a malformed URL so the loader can skip the prefetch (the page
 * owns the fallback). Mirrors the page's `readUrlState` safe-parse approach.
 */
function readProxyLogsUrlSearch(): ProxyLogsSearch | null {
  if (typeof window === 'undefined') return null
  const entries = Object.fromEntries(
    new URLSearchParams(window.location.search).entries(),
  )
  const parsed = proxyLogsSearchSchema.safeParse(entries)
  return parsed.success ? parsed.data : null
}

export const Route = createFileRoute('/_authenticated/proxy-logs')({
  validateSearch: proxyLogsSearchSchema,
  loader: async ({ context }) => {
    const search = readProxyLogsUrlSearch()
    if (!search) return

    const pageIndex = search.page ?? 0
    const pageSize = search.pageSize ?? DEFAULT_PROXY_LOGS_PAGE_SIZE
    const status = search.status === 'all' ? undefined : search.status
    const searchText = search.q?.trim() || undefined
    const siteId = search.siteId ?? undefined
    const client = search.client || undefined
    const from = search.from || undefined
    const to = search.to || undefined

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
  component: lazyRouteComponent(
    () => import('@/features/proxy-logs/components/proxy-logs-page'),
    'ProxyLogsPage',
  ),
})
