// metapi-go/features/proxy-logs — TanStack Query hooks wrapping `lib/api.ts`.
//
// The proxy-logs backend is server-paginated. The list and the facets are
// split across two endpoints so the expensive five-way summary aggregate is
// computed exactly once per page load: `useProxyLogs` hits `view=query`
// (items + total only — no summary/sites/clientOptions payload), while
// `useProxyLogsMeta` (view=meta) is the single owner of the summary strip +
// toolbar facets. The previous wiring fetched the default `full` view (which
// embeds the summary) AND `view=meta`, running the aggregate twice per load.
// `useProxyLog` fetches the detail payload by id.

import { useQuery, type UseQueryOptions } from '@tanstack/react-query'

import { type ProxyLogsQuery, api } from '@/lib/api'

import { proxyLogsKeys, type ProxyLogDetail } from './types'

// Resolve the data type returned by each backend method. The methods return
// `Promise<T>`; `Awaited<...>` unwraps the promise so the query data type is
// the parsed payload, not the promise wrapper.
type ProxyLogsQueryData = Awaited<ReturnType<typeof api.getProxyLogsQuery>>
type ProxyLogsMetaData = Awaited<ReturnType<typeof api.getProxyLogsMeta>>

/**
 * Fetch a page of proxy logs via the list-only `view=query` endpoint
 * (items/total/page/pageSize — the summary aggregate travels exclusively via
 * `useProxyLogsMeta`). The query key embeds the full filter object so cache
 * reuse is exact; the page passes a stable `ProxyLogsQuery` (built from URL
 * state) so re-renders with the same filters don't refetch.
 */
export function useProxyLogs(
  params: ProxyLogsQuery,
  options?: Omit<UseQueryOptions<ProxyLogsQueryData>, 'queryKey' | 'queryFn'>
) {
  return useQuery({
    queryKey: proxyLogsKeys.list(params),
    queryFn: async () => api.getProxyLogsQuery(params),
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
