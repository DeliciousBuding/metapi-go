import type { ColumnFiltersState } from '@tanstack/react-table'

import {
  encodeSorting,
  type UrlTableState,
  type UrlTableStateUpdate,
  useUrlTableState,
} from '@/components/data-table'
import { parseSortingParam } from '@/lib/helpers/searchParams'

const DEFAULT_PAGE_SIZE = 20

/** Page-specific URL filters: comma-separated status + site id lists. */
type AccountsUrlFilters = {
  status: string
  site: string
}

/**
 * Parse the raw search string into URL table state. The accounts URL uses a
 * 1-based `page` param (route schema: accountsSearchSchema); `q` / `status` /
 * `site` are plain strings.
 */
function readAccountsSearch(
  searchString: string
): UrlTableState<AccountsUrlFilters> {
  const params = new URLSearchParams(searchString)
  const rawPage = Number(params.get('page') ?? '1')
  const rawPageSize = Number(
    params.get('pageSize') ?? String(DEFAULT_PAGE_SIZE)
  )
  const pageIndex =
    Number.isFinite(rawPage) && rawPage > 0 ? Math.max(0, rawPage - 1) : 0
  const pageSize =
    Number.isFinite(rawPageSize) && rawPageSize > 0
      ? Math.min(100, Math.max(1, rawPageSize))
      : DEFAULT_PAGE_SIZE
  return {
    q: params.get('q') ?? '',
    pageIndex,
    pageSize,
    sorting: parseSortingParam(params.get('sort') ?? undefined),
    filters: {
      status: params.get('status') ?? '',
      site: params.get('site') ?? '',
    },
  }
}

/** Serialize a partial state update back to the 1-based accounts href,
 *  merging over the CURRENT url state (so a pagination sync preserves q /
 *  status / site — stripping them re-triggered the toolbar's debounced
 *  search commit and looped the renderer). */
function buildAccountsHref(
  next: UrlTableStateUpdate<AccountsUrlFilters>,
  currentSearch?: string
): string {
  const currentSearchString = currentSearch ?? window.location.search
  const currentParams = new URLSearchParams(currentSearchString)
  const current = readAccountsSearch(currentSearchString)
  const merged: UrlTableState<AccountsUrlFilters> = {
    ...current,
    ...next,
    filters: { ...current.filters, ...next.filters },
  }
  const params = new URLSearchParams()
  if (merged.q) params.set('q', merged.q)
  if (merged.pageIndex > 0) params.set('page', String(merged.pageIndex + 1))
  if (merged.pageSize !== DEFAULT_PAGE_SIZE) {
    params.set('pageSize', String(merged.pageSize))
  }
  const sortString = encodeSorting(merged.sorting)
  if (sortString) params.set('sort', sortString)
  if (merged.filters.status) params.set('status', merged.filters.status)
  if (merged.filters.site) params.set('site', merged.filters.site)
  // Transient deep-link params are preserved across table-state syncs until
  // the page's one-shot consume effects have stripped them (a toolbar
  // commit or page clamp firing first would otherwise drop the deep link).
  // Reads the router search string (not window.location, which lags until the
  // transition commits — right after the strip, a stale snapshot would
  // resurrect siteId/create and reopen the dialog on the next reload).
  const guidedSiteId = currentParams.get('siteId')
  const guidedCreate = currentParams.get('create')
  const attentionAccountId = currentParams.get('accountId')
  if (guidedSiteId) params.set('siteId', guidedSiteId)
  if (guidedCreate) params.set('create', guidedCreate)
  if (attentionAccountId) params.set('accountId', attentionAccountId)
  const queryString = params.toString()
  return queryString ? `/accounts?${queryString}` : '/accounts'
}

/** The "feature useSearch" stage, mirroring the sites page. */
export function useAccountsUrlState() {
  return useUrlTableState<AccountsUrlFilters>({
    basePath: '/accounts',
    read: readAccountsSearch,
    buildHref: buildAccountsHref,
    toColumnFilters: (filters) => {
      const out: ColumnFiltersState = []
      const statusValues = filters.status.split(',').filter(Boolean)
      const siteIds = filters.site.split(',').filter(Boolean)
      if (statusValues.length) out.push({ id: 'status', value: statusValues })
      if (siteIds.length) out.push({ id: 'site', value: siteIds })
      return out
    },
    fromColumnFilters: (filters) => {
      const statusEntry = filters.find((filter) => filter.id === 'status')
      const siteEntry = filters.find((filter) => filter.id === 'site')
      return {
        filters: {
          status: Array.isArray(statusEntry?.value)
            ? statusEntry.value.join(',')
            : '',
          site: Array.isArray(siteEntry?.value)
            ? siteEntry.value.join(',')
            : '',
        },
      }
    },
  })
}
