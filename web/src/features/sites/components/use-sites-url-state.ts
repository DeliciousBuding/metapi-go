import {
  encodeSorting,
  useUrlTableState,
  type UrlTableState,
  type UrlTableStateUpdate,
} from '@/components/data-table'
import { asStringParam, parseSortingParam } from '@/lib/helpers/searchParams'

import { sitesSearchSchema } from '../lib/sites-schema'

type SitesUrlFilters = { status: string | undefined }

function readSearch(searchString?: string): UrlTableState<SitesUrlFilters> {
  if (typeof window === 'undefined') {
    return {
      q: '',
      pageIndex: 0,
      pageSize: 20,
      sorting: [],
      filters: { status: undefined },
    }
  }
  const params = new URLSearchParams(searchString ?? window.location.search)
  const parsed = sitesSearchSchema.safeParse({
    q: params.get('q') ?? undefined,
    page: params.get('page') ?? undefined,
    pageSize: params.get('pageSize') ?? undefined,
    sort: params.get('sort') ?? undefined,
    status: params.get('status') ?? undefined,
  })
  if (!parsed.success) {
    return {
      q: '',
      pageIndex: 0,
      pageSize: 20,
      sorting: [],
      filters: { status: undefined },
    }
  }
  const data = parsed.data
  return {
    q: asStringParam(data.q) ?? '',
    pageIndex: data.page ?? 0,
    pageSize: data.pageSize ?? 20,
    sorting: parseSortingParam(data.sort),
    filters: { status: asStringParam(data.status) },
  }
}

function buildHref(
  next: UrlTableStateUpdate<SitesUrlFilters>,
  currentSearch?: string
): string {
  const current = readSearch(currentSearch)
  const merged: UrlTableState<SitesUrlFilters> = {
    ...current,
    ...next,
    filters: { ...current.filters, ...next.filters },
  }
  const params = new URLSearchParams()
  if (merged.q) params.set('q', merged.q)
  if (merged.pageIndex > 0) params.set('page', String(merged.pageIndex))
  if (merged.pageSize !== 20) params.set('pageSize', String(merged.pageSize))
  const sortString = encodeSorting(merged.sorting)
  if (sortString) params.set('sort', sortString)
  if (merged.filters.status) params.set('status', merged.filters.status)
  // Preserve the one-shot `create`/`edit` deep-link params across table-state
  // navigations (page-clamp / sort / filter) until the page's consumption
  // effect strips them — mirrors the accounts page's buildAccountsHref guard
  // for its guided-flow `siteId`/`create` params. Reads the router search
  // string (not window.location, which lags until the transition commits —
  // right after the strip an old snapshot would resurrect the params).
  const searchParams = new URLSearchParams(
    currentSearch ?? window.location.search
  )
  const guidedCreate = searchParams.get('create')
  if (guidedCreate) params.set('create', guidedCreate)
  const guidedEdit = searchParams.get('edit')
  if (guidedEdit) params.set('edit', guidedEdit)
  const queryString = params.toString()
  return queryString ? `/sites?${queryString}` : '/sites'
}

/**
 * The "feature useSearch" stage. Reads the URL on every render (cheap) and
 * hands the data-table controlled state + navigation-backed setters. The
 * shared {@link useUrlTableState} owns the router subscription and the
 * URL-sync guard.
 */
export function useSitesUrlState() {
  return useUrlTableState<SitesUrlFilters>({
    basePath: '/sites',
    read: readSearch,
    buildHref,
    toColumnFilters: (filters) =>
      filters.status ? [{ id: 'status', value: [filters.status] }] : [],
    fromColumnFilters: (filters) => {
      const statusEntry = filters.find((filter) => filter.id === 'status')
      return {
        filters: {
          status: Array.isArray(statusEntry?.value)
            ? (statusEntry.value as string[])[0]
            : (statusEntry?.value as string | undefined),
        },
      }
    },
  })
}
