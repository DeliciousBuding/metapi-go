// metapi-go/data-table — shared URL-synced table state hook.
//
// The list pages (sites / oauth / site-announcements / models) each mirror
// table state (search / page / page-size / sorting / faceted filters) to the
// URL so a deep link restores the exact view. This hook owns the TanStack
// Router plumbing that was previously duplicated per page:
//
//   - subscribing to `location.searchStr` (router does not re-render a route
//     component on same-path search-only navigation otherwise)
//   - the URL-sync guard that stops table callbacks from hijacking an
//     in-flight navigation away from the page
//   - the four controlled-state callbacks (globalFilter / pagination /
//     sorting / columnFilters) that serialize changes back to the URL
//
// Each page supplies only the page-specific parts: how to parse the URL
// (`read`), how to serialize it (`buildHref`), and how page filter values map
// to TanStack column filters (`toColumnFilters` / `fromColumnFilters`).

import { useLocation, useNavigate } from '@tanstack/react-router'
import type {
  ColumnFiltersState,
  PaginationState,
  SortingState,
  Updater,
} from '@tanstack/react-table'

/** Validated URL state for a list page. `filters` holds page-specific values. */
export type UrlTableState<TFilters> = {
  q: string
  pageIndex: number
  pageSize: number
  sorting: SortingState
  filters: TFilters
}

export type UrlTableStateOptions<TFilters> = {
  /** Base pathname used to build hrefs (e.g. '/sites'). */
  basePath: string
  /** Parse the validated URL state from a raw search string. */
  read: (searchString: string) => UrlTableState<TFilters>
  /** Serialize a partial state update back to a full href (path + query). */
  buildHref: (next: Partial<UrlTableState<TFilters>>) => string
  /** Map page filter values to table column filters. */
  toColumnFilters: (filters: TFilters) => ColumnFiltersState
  /** Map table column filters back to page filter values. */
  fromColumnFilters: (
    filters: ColumnFiltersState
  ) => Partial<UrlTableState<TFilters>>
}

function resolveUpdater<TValue>(
  updater: Updater<TValue>,
  previous: TValue
): TValue {
  return typeof updater === 'function'
    ? (updater as (old: TValue) => TValue)(previous)
    : updater
}

/** Serialize TanStack sorting to a `id:desc` comma-separated string. */
export function encodeSorting(sorting: SortingState): string {
  return sorting
    .map((item) => `${item.id}:${item.desc ? 'desc' : 'asc'}`)
    .join(',')
}

/**
 * Controlled table state whose source of truth is the URL search string.
 * Re-exported fields plug straight into `useDataTable`.
 */
export function useUrlTableState<TFilters>(
  options: UrlTableStateOptions<TFilters>
) {
  const navigate = useNavigate()
  // Subscribe to the router location so search-only navigation re-renders.
  const searchStr = useLocation({ select: (loc) => loc.searchStr })
  const search = options.read(searchStr)

  const columnFilters = options.toColumnFilters(search.filters)

  // URL-sync guard: table state callbacks can fire while the router is
  // navigating away (the useLocation subscription re-renders this page with
  // the *next* location's search string). Without the pathname check the
  // callback would navigate straight back, hijacking the in-flight
  // navigation — the "clicked a sidebar link but the page snapped back"
  // bug. Only sync when we are still on this page.
  function syncUrl(next: Partial<UrlTableState<TFilters>>) {
    const href = options.buildHref(next)
    if (!href.startsWith(window.location.pathname)) return
    navigate({ href, replace: true })
  }

  const onGlobalFilterChange = (updater: Updater<string>) => {
    syncUrl({ q: resolveUpdater(updater, search.q) })
  }
  const onPaginationChange = (updater: Updater<PaginationState>) => {
    const next = resolveUpdater(updater, {
      pageIndex: search.pageIndex,
      pageSize: search.pageSize,
    })
    syncUrl({ pageIndex: next.pageIndex, pageSize: next.pageSize })
  }
  const onSortingChange = (updater: Updater<SortingState>) => {
    syncUrl({ sorting: resolveUpdater(updater, search.sorting) })
  }
  const onColumnFiltersChange = (updater: Updater<ColumnFiltersState>) => {
    const next = resolveUpdater(updater, columnFilters)
    syncUrl(options.fromColumnFilters(next))
  }

  return {
    globalFilter: search.q,
    onGlobalFilterChange,
    pagination: {
      pageIndex: search.pageIndex,
      pageSize: search.pageSize,
    } as PaginationState,
    onPaginationChange,
    sorting: search.sorting,
    onSortingChange,
    columnFilters,
    onColumnFiltersChange,
  }
}
