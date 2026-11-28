import type {
  UrlTableState,
  UrlTableStateUpdate,
} from '@/components/data-table'
import { asStringParam } from '@/lib/helpers/searchParams'

import { proxyLogsSearchSchema } from '../lib/proxy-logs-schema'

export type RequestFilters = {
  status: string
  downstreamKeyId: string
  from: string
  to: string
  requestId: string
}

export function readGatewayRequestsSearch(
  search: string
): UrlTableState<RequestFilters> {
  const params = new URLSearchParams(search)
  const parsed = proxyLogsSearchSchema.parse(Object.fromEntries(params))
  return {
    q: asStringParam(parsed.q) ?? '',
    pageIndex: parsed.page,
    pageSize: parsed.pageSize,
    sorting: [],
    filters: {
      status: parsed.requestStatus,
      downstreamKeyId:
        parsed.downstreamKeyId === undefined
          ? ''
          : String(parsed.downstreamKeyId),
      from: asStringParam(parsed.from) ?? '',
      to: asStringParam(parsed.to) ?? '',
      requestId: asStringParam(parsed.requestId) ?? '',
    },
  }
}

export function buildGatewayRequestsHref(
  next: UrlTableStateUpdate<RequestFilters>,
  currentSearch?: string,
  basePath = '/proxy-logs'
): string {
  const current = readGatewayRequestsSearch(
    currentSearch ?? window.location.search
  )
  const merged = {
    ...current,
    ...next,
    filters: { ...current.filters, ...next.filters },
  }
  const params = new URLSearchParams()
  if (merged.q) params.set('q', merged.q)
  if (merged.pageIndex > 0) params.set('page', String(merged.pageIndex))
  if (merged.pageSize !== 20) params.set('pageSize', String(merged.pageSize))
  if (merged.filters.status !== 'all')
    params.set('requestStatus', merged.filters.status)
  for (const key of ['downstreamKeyId', 'from', 'to', 'requestId'] as const) {
    if (merged.filters[key]) params.set(key, merged.filters[key])
  }
  if (basePath === '/audit-logs') params.set('category', 'access')
  return `${basePath}${params.size ? `?${params}` : ''}`
}

export type UpdateGatewaySearch = (
  next: UrlTableStateUpdate<RequestFilters>
) => void
