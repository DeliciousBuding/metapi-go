import { useMemo } from 'react'

import {
  useUrlTableState,
  type UrlTableState,
  type UrlTableStateUpdate,
} from '@/components/data-table'
import { asStringParam } from '@/lib/helpers/searchParams'

import {
  proxyLogsSearchSchema,
  type ProxyLogsSearch,
} from '../lib/proxy-logs-schema'
import type { ProxyLogFilters } from '../types'

const DEFAULT_PAGE_SIZE = 20

/** Page-specific URL filters (all strings for URL round-trip simplicity). */
export type ProxyLogsUrlFilters = {
  status: ProxyLogFilters['status']
  siteId: string
  channelId: string
  downstreamKeyId: string
  client: string
  from: string
  to: string
  latencyMin: string
  latencyMax: string
}

/**
 * Parse the raw search string into URL table state, reusing the route's
 * `proxyLogsSearchSchema` so the derived query payload matches the prefetched
 * cache key (no double fetch). Returns schema defaults on a malformed string.
 */
function readProxyLogsSearch(
  searchString: string
): UrlTableState<ProxyLogsUrlFilters> {
  const entries = Object.fromEntries(
    new URLSearchParams(
      searchString.startsWith('?') ? searchString.slice(1) : searchString
    ).entries()
  )
  const parsed = proxyLogsSearchSchema.safeParse(entries)
  const search: ProxyLogsSearch | null = parsed.success ? parsed.data : null
  return {
    q: asStringParam(search?.q) ?? '',
    // The proxy-logs URL is 0-based (`page`), unlike the other list pages.
    pageIndex: search?.page ?? 0,
    pageSize: search?.pageSize ?? DEFAULT_PAGE_SIZE,
    sorting: [],
    filters: {
      status: search?.status ?? 'all',
      siteId: search?.siteId === undefined ? '' : String(search.siteId),
      channelId:
        search?.channelId === undefined ? '' : String(search.channelId),
      downstreamKeyId:
        search?.downstreamKeyId === undefined
          ? ''
          : String(search.downstreamKeyId),
      client: asStringParam(search?.client) ?? '',
      from: asStringParam(search?.from) ?? '',
      to: asStringParam(search?.to) ?? '',
      latencyMin:
        search?.latencyMin === undefined ? '' : String(search.latencyMin),
      latencyMax:
        search?.latencyMax === undefined ? '' : String(search.latencyMax),
    },
  }
}

/** Serialize a partial state update back to the proxy-logs href, merging over
 *  the CURRENT url state so a single filter change preserves all others. */
function buildProxyLogsHref(
  next: UrlTableStateUpdate<ProxyLogsUrlFilters>,
  currentSearch?: string
): string {
  const current = readProxyLogsSearch(currentSearch ?? window.location.search)
  const merged: UrlTableState<ProxyLogsUrlFilters> = {
    ...current,
    ...next,
    filters: { ...current.filters, ...next.filters },
  }
  const params = new URLSearchParams()
  params.set('view', 'attempts')
  if (merged.q) params.set('q', merged.q)
  if (merged.pageIndex > 0) params.set('page', String(merged.pageIndex))
  if (merged.pageSize !== DEFAULT_PAGE_SIZE) {
    params.set('pageSize', String(merged.pageSize))
  }
  if (merged.filters.status !== 'all') {
    params.set('status', merged.filters.status)
  }
  if (merged.filters.siteId) params.set('siteId', merged.filters.siteId)
  // Only a positive channel id is a real filter — the backend reads 0 as
  // "unset", so writing `channelId=0` would leave the URL disagreeing with
  // the rendered (unfiltered) list.
  if (Number(merged.filters.channelId) > 0) {
    params.set('channelId', merged.filters.channelId)
  }
  if (Number(merged.filters.downstreamKeyId) > 0) {
    params.set('downstreamKeyId', merged.filters.downstreamKeyId)
  }
  if (merged.filters.client) params.set('client', merged.filters.client)
  if (merged.filters.from) params.set('from', merged.filters.from)
  if (merged.filters.to) params.set('to', merged.filters.to)
  if (merged.filters.latencyMin) {
    params.set('latencyMin', merged.filters.latencyMin)
  }
  if (merged.filters.latencyMax) {
    params.set('latencyMax', merged.filters.latencyMax)
  }
  const queryString = params.toString()
  return queryString ? `/proxy-logs?${queryString}` : '/proxy-logs'
}

export function useProxyLogsUrlState() {
  const state = useUrlTableState<ProxyLogsUrlFilters>({
    basePath: '/proxy-logs',
    read: readProxyLogsSearch,
    buildHref: buildProxyLogsHref,
    // No TanStack column filters on this page: status / site / client /
    // date / latency are all page-specific controls driven by `filters`.
    toColumnFilters: () => [],
    fromColumnFilters: () => ({}),
    resetPageIndexOnFilterChange: true,
  })

  const { filters, globalFilter, pagination } = state
  const metaPayload = useMemo(
    () => ({
      status: filters.status === 'all' ? undefined : filters.status,
      search: globalFilter.trim() || undefined,
      siteId: filters.siteId ? Number(filters.siteId) : undefined,
      channelId: filters.channelId ? Number(filters.channelId) : undefined,
      downstreamKeyId: filters.downstreamKeyId
        ? Number(filters.downstreamKeyId)
        : undefined,
      client: filters.client || undefined,
      from: filters.from || undefined,
      to: filters.to || undefined,
      latencyMin: filters.latencyMin ? Number(filters.latencyMin) : undefined,
      latencyMax: filters.latencyMax ? Number(filters.latencyMax) : undefined,
    }),
    [filters, globalFilter]
  )
  const queryPayload = useMemo(
    () => ({
      ...metaPayload,
      limit: pagination.pageSize,
      offset: pagination.pageIndex * pagination.pageSize,
    }),
    [metaPayload, pagination]
  )
  return { ...state, metaPayload, queryPayload }
}
