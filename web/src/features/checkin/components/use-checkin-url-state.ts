import type { ColumnFiltersState } from '@tanstack/react-table'

import {
  type UrlTableState,
  type UrlTableStateUpdate,
  useUrlTableState,
} from '@/components/data-table'
import { asStringParam } from '@/lib/helpers/searchParams'

import {
  DEFAULT_CHECKIN_PAGE_SIZE,
  parseCheckinSearch,
  parseFilterValues,
} from '../lib/checkin-schema'

/** Page-specific URL filters: comma-separated lists + date range + account. */
type CheckinUrlFilters = {
  status: string
  reason: string
  site: string
  accountId: string
  from: string
  to: string
}

/**
 * Parse the raw search string into URL table state. Reuses the route's
 * `parseCheckinSearch` (same schema the loader uses) so the page's derived
 * query payload exactly matches the prefetched cache key — no double fetch.
 */
function readCheckinSearch(
  searchString: string
): UrlTableState<CheckinUrlFilters> {
  const search = parseCheckinSearch(searchString)
  return {
    q: asStringParam(search.q) ?? '',
    pageIndex: search.page - 1,
    pageSize: search.pageSize,
    sorting: [],
    filters: {
      status: asStringParam(search.status) ?? '',
      reason: asStringParam(search.reason) ?? '',
      site: asStringParam(search.site) ?? '',
      accountId: search.accountId === undefined ? '' : String(search.accountId),
      from: asStringParam(search.from) ?? '',
      to: asStringParam(search.to) ?? '',
    },
  }
}

/** Serialize a partial state update back to the checkin href, merging over
 *  the CURRENT url state so a single filter change preserves all others. */
function buildCheckinHref(
  next: UrlTableStateUpdate<CheckinUrlFilters>
): string {
  const current = readCheckinSearch(window.location.search)
  const merged: UrlTableState<CheckinUrlFilters> = {
    ...current,
    ...next,
    filters: { ...current.filters, ...next.filters },
  }
  const params = new URLSearchParams()
  if (merged.q) params.set('q', merged.q)
  if (merged.pageIndex > 0) params.set('page', String(merged.pageIndex + 1))
  if (merged.pageSize !== DEFAULT_CHECKIN_PAGE_SIZE) {
    params.set('pageSize', String(merged.pageSize))
  }
  if (merged.filters.status) params.set('status', merged.filters.status)
  if (merged.filters.reason) params.set('reason', merged.filters.reason)
  if (merged.filters.site) params.set('site', merged.filters.site)
  if (merged.filters.accountId) {
    params.set('accountId', merged.filters.accountId)
  }
  if (merged.filters.from) params.set('from', merged.filters.from)
  if (merged.filters.to) params.set('to', merged.filters.to)
  const queryString = params.toString()
  return queryString ? `/checkin?${queryString}` : '/checkin'
}

export function useCheckinUrlState() {
  return useUrlTableState<CheckinUrlFilters>({
    basePath: '/checkin',
    read: readCheckinSearch,
    buildHref: buildCheckinHref,
    toColumnFilters: (filters) => {
      const out: ColumnFiltersState = []
      const statusValues = parseFilterValues(filters.status)
      const reasonValues = parseFilterValues(filters.reason)
      const siteValues = parseFilterValues(filters.site)
      if (statusValues.length) out.push({ id: 'status', value: statusValues })
      if (reasonValues.length) out.push({ id: 'reason', value: reasonValues })
      if (siteValues.length) out.push({ id: 'site', value: siteValues })
      return out
    },
    fromColumnFilters: (filters) => {
      const statusEntry = filters.find((filter) => filter.id === 'status')
      const reasonEntry = filters.find((filter) => filter.id === 'reason')
      const siteEntry = filters.find((filter) => filter.id === 'site')
      return {
        filters: {
          status: Array.isArray(statusEntry?.value)
            ? statusEntry.value.join(',')
            : '',
          reason: Array.isArray(reasonEntry?.value)
            ? reasonEntry.value.join(',')
            : '',
          site: Array.isArray(siteEntry?.value)
            ? siteEntry.value.join(',')
            : '',
        },
      }
    },
    resetPageIndexOnFilterChange: true,
  })
}
