// metapi-go/features/proxy-logs — TanStack Query hooks wrapping `lib/api.ts`.
//
// The proxy-logs backend is server-paginated (`getProxyLogs` returns
// items + total + page + pageSize), so `useProxyLogs` accepts the full
// `ProxyLogsQuery` (limit/offset/status/search/client/siteId/from/to) and
// the list page drives it from the URL-synced filter state. `useProxyLog`
// fetches the detail payload by id. `useProxyLogsMeta` returns the
// clientOptions / summary / sites facets used by the toolbar.

import { useQuery, type UseQueryOptions } from '@tanstack/react-query'

import { type ProxyLogsQuery, api } from '@/lib/api'

import { proxyLogsKeys, type ProxyLogDetail } from './types'

// Resolve the data type returned by each backend method. The methods return
// `Promise<T>`; `Awaited<...>` unwraps the promise so the query data type is
// the parsed payload, not the promise wrapper.
type ProxyLogsResponseData = Awaited<ReturnType<typeof api.getProxyLogs>>
type ProxyLogsMetaData = Awaited<ReturnType<typeof api.getProxyLogsMeta>>

/**
 * Fetch a page of proxy logs. The query key embeds the full filter object so
 * cache reuse is exact; the page passes a stable `ProxyLogsQuery` (built
 * from URL state) so re-renders with the same filters don't refetch.
 */
export function useProxyLogs(
  params: ProxyLogsQuery,
  options?: Omit<UseQueryOptions<ProxyLogsResponseData>, 'queryKey' | 'queryFn'>
) {
  return useQuery({
    queryKey: proxyLogsKeys.list(params),
    queryFn: async () => api.getProxyLogs(params),
    placeholderData: (previous) => previous,
    ...options,
  })
}

/**
 * Fetch a single proxy log's detail payload (billing / route / channel /
 * http status + optional raw bodies when the backend exposes them).
 */
export function useProxyLog(
  id: number | null,
  options?: Omit<UseQueryOptions<ProxyLogDetail>, 'queryKey' | 'queryFn'>
) {
  return useQuery<ProxyLogDetail>({
    queryKey:
      id === null ? ['proxy-logs', 'detail', 'none'] : proxyLogsKeys.detail(id),
    queryFn: async () => {
      const result = await api.getProxyLogDetail(id as number)
      return result as unknown as ProxyLogDetail
    },
    enabled: id !== null,
    ...options,
  })
}

/**
 * Fetch the proxy-logs meta facets (clientOptions / summary / sites) for the
 * toolbar dropdowns and the summary strip. Accepts the filter subset that
 * the meta endpoint honours (everything except limit/offset).
 */
export function useProxyLogsMeta(
  params: Omit<ProxyLogsQuery, 'limit' | 'offset'>,
  options?: Omit<UseQueryOptions<ProxyLogsMetaData>, 'queryKey' | 'queryFn'>
) {
  return useQuery({
    queryKey: proxyLogsKeys.meta(params),
    queryFn: async () => api.getProxyLogsMeta(params),
    placeholderData: (previous) => previous,
    ...options,
  })
}

export type { ProxyLogDetail }
