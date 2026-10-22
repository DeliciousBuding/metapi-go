import type {
  UrlTableState,
  UrlTableStateUpdate,
} from '@/components/data-table'

export const requestStatuses = [
  'completed',
  'upstream_failed',
  'output_interrupted',
  'client_cancelled',
  'rejected_no_route',
  'rejected_policy',
  'gateway_failed',
] as const
export const quotaStates = [
  'pending',
  'settled',
  'released',
  'unknown',
  'missing',
] as const
export type RequestStatus = (typeof requestStatuses)[number]
export type QuotaState = (typeof quotaStates)[number]
type Filters = { status?: RequestStatus; quotaState?: QuotaState }

export function readRequestSearch(search: string): UrlTableState<Filters> {
  const params = new URLSearchParams(search)
  const page = Number(params.get('page') ?? 1)
  const status = params.get('status') ?? ''
  const quotaState = params.get('quotaState') ?? ''
  return {
    q: (params.get('q') ?? '').slice(0, 200),
    pageIndex:
      Number.isSafeInteger(page) && page > 0 && page <= 1_000_000
        ? page - 1
        : 0,
    pageSize: 20,
    sorting: [],
    filters: {
      status: requestStatuses.includes(status as RequestStatus)
        ? (status as RequestStatus)
        : undefined,
      quotaState: quotaStates.includes(quotaState as never)
        ? (quotaState as QuotaState)
        : undefined,
    },
  }
}

export function requestSearchHref(
  next: UrlTableStateUpdate<Filters>,
  search = window.location.search
): string {
  const current = readRequestSearch(search)
  const state = {
    ...current,
    ...next,
    filters: { ...current.filters, ...next.filters },
  }
  const params = new URLSearchParams(search)
  params.set('view', 'requests')
  params.set('page', String(state.pageIndex + 1))
  params.delete('requestId')
  for (const [name, value] of Object.entries({
    q: state.q,
    ...state.filters,
  })) {
    if (value) params.set(name, value)
    else params.delete(name)
  }
  return '/user?' + params.toString()
}
