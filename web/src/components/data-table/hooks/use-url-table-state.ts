// metapi-go/data-table — shared URL-synced table state hook.
//
// The list pages (sites / oauth / models / accounts / checkin / proxy-logs /
// token-routes / …) each mirror table state (search / page / page-size /
// sorting / faceted filters) plus page-specific filters to the URL so a deep
// link restores the exact view. This hook owns the TanStack Router plumbing
// that was previously duplicated per page:
//
//   - subscribing to `location.searchStr` (router does not re-render a route
//     component on same-path search-only navigation otherwise)
//   - the URL-sync guard that stops table callbacks from hijacking an
//     in-flight navigation away from the page
//   - the controlled-state callbacks (globalFilter / pagination / sorting /
//     columnFilters) that serialize changes back to the URL
//   - a generic `updateUrlState` primitive so page-specific, non-table filters
//     (date ranges, account/client selects, latency bounds, …) change in the
//     exact same single-transaction way instead of re-implementing navigate
//     logic per page
//
// Each page supplies only the page-specific parts: how to parse the URL
// (`read`), how to serialize it (`buildHref`), and how page filter values map
// to TanStack column filters (`toColumnFilters` / `fromColumnFilters`).
//
// Stability contract: every returned value and callback keeps a stable
// identity as long as its inputs are unchanged. useDataTable hands these
// straight into useReactTable, and unstable callback identities force the
// table to re-resolve every render — which re-runs TanStack's
// autoResetPageIndex effect and can feed back through the URL into an
// infinite render loop (the accounts page freeze). The page-supplied option
// functions are read through a ref so inline definitions do not break the
// contract.

import { useLocation, useNavigate, useRouterState } from '@tanstack/react-router'
import type {
  ColumnFiltersState,
  PaginationState,
  SortingState,
  Updater,
} from '@tanstack/react-table'
import * as React from 'react'

/** Validated URL state for a list page. `filters` holds page-specific values. */
export type UrlTableState<TFilters> = {
  q: string
  pageIndex: number
  pageSize: number
  sorting: SortingState
  filters: TFilters
}

/**
 * A partial URL-state update. `filters` is itself partial so a caller can
 * change a single page-specific filter (or a single table column filter)
 * without supplying every field; `buildHref` merges it over the current state.
 */
export type UrlTableStateUpdate<TFilters> = Partial<
  Omit<UrlTableState<TFilters>, 'filters'>
> & {
  filters?: Partial<TFilters>
}

export type UrlTableStateOptions<TFilters> = {
  /** Base pathname used to build hrefs (e.g. '/sites'). */
  basePath: string
  /** Parse the validated URL state from a raw search string. */
  read: (searchString: string) => UrlTableState<TFilters>
  /**
   * Serialize a partial state update back to a full href (path + query).
   * Receives the ROUTER's current search string (updates at transition
   * start, unlike `window.location.search` which lags until commit) so
   * page-specific guided-param preservation never resurrects a deep-link
   * param that a consume effect just stripped.
   */
  buildHref: (
    next: UrlTableStateUpdate<TFilters>,
    currentSearch: string
  ) => string
  /** Map page filter values to table column filters. */
  toColumnFilters: (filters: TFilters) => ColumnFiltersState
  /** Map table column filters back to page filter values. */
  fromColumnFilters: (
    filters: ColumnFiltersState
  ) => UrlTableStateUpdate<TFilters>
  /**
   * When true, a globalFilter / columnFilters change also resets the page to 0
   * in the same URL transaction. Server-side paginated pages want an explicit
   * page reset on every filter change (and pass `autoResetPageIndex: false`
   * to useDataTable so TanStack does not fire a second, redundant reset).
   */
  resetPageIndexOnFilterChange?: boolean
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
 * Re-exported fields plug straight into `useDataTable`; `filters` +
 * `updateUrlState` cover the page-specific controls that are not TanStack
 * column filters.
 */
export function useUrlTableState<TFilters>(
  options: UrlTableStateOptions<TFilters>
) {
  const navigate = useNavigate()
  // Subscribe to the router location so search-only navigation re-renders.
  const searchStr = useLocation({ select: (loc) => loc.searchStr })
  // The router's own location pathname, which updates at the START of a
  // navigation rather than at commit. Table callbacks can fire while a
  // navigation is in flight (the useLocation subscription re-renders this
  // page with the *next* location's search string and TanStack's
  // autoResetPageIndex reacts to it); that window is exactly when the
  // updateUrlState guard must NOT write, or the sync replaces the pending
  // navigation and the page snaps back — the modal-CTA deep links and
  // sidebar links both hit it. `window.location.pathname` is only committed
  // AFTER the transition resolves, so it reads the old path during the
  // pending window and lets the hijack through.
  const routerPathname = useRouterState({
    select: (state) => state.location.pathname,
  })
  const resetPageIndexOnFilterChange =
    options.resetPageIndexOnFilterChange ?? false

  // Page-supplied functions are read through refs so inline definitions
  // (recreated every render by the page) do not change any identity below.
  const readRef = React.useRef(options.read)
  readRef.current = options.read
  const buildHrefRef = React.useRef(options.buildHref)
  buildHrefRef.current = options.buildHref
  const toColumnFiltersRef = React.useRef(options.toColumnFilters)
  toColumnFiltersRef.current = options.toColumnFilters
  const fromColumnFiltersRef = React.useRef(options.fromColumnFilters)
  fromColumnFiltersRef.current = options.fromColumnFilters

  const search = React.useMemo(() => readRef.current(searchStr), [searchStr])
  const columnFilters = React.useMemo(
    () => toColumnFiltersRef.current(search.filters),
    [search.filters]
  )
  const pagination = React.useMemo<PaginationState>(
    () => ({
      pageIndex: search.pageIndex,
      pageSize: search.pageSize,
    }),
    [search.pageIndex, search.pageSize]
  )

  // URL-sync guard: table state callbacks can fire while the router is
  // navigating away (the useLocation subscription re-renders this page with
  // the *next* location's search string). Without the pathname check the
  // callback would navigate straight back, hijacking the in-flight
  // navigation — the "clicked a sidebar link but the page snapped back"
  // bug. Only sync when we are still on this page.
  const updateUrlState = React.useCallback(
    (next: UrlTableStateUpdate<TFilters>) => {
      const href = buildHrefRef.current(next, searchStr)
      if (!href.startsWith(routerPathname)) return
      navigate({ href, replace: true })
    },
    [navigate, routerPathname, searchStr]
  )

  const onGlobalFilterChange = React.useCallback(
    (updater: Updater<string>) => {
      updateUrlState({
        q: resolveUpdater(updater, search.q),
        ...(resetPageIndexOnFilterChange ? { pageIndex: 0 } : {}),
      })
    },
    [updateUrlState, search.q, resetPageIndexOnFilterChange]
  )
  const onPaginationChange = React.useCallback(
    (updater: Updater<PaginationState>) => {
      const next = resolveUpdater(updater, pagination)
      updateUrlState({ pageIndex: next.pageIndex, pageSize: next.pageSize })
    },
    [updateUrlState, pagination]
  )
  const onSortingChange = React.useCallback(
    (updater: Updater<SortingState>) => {
      updateUrlState({ sorting: resolveUpdater(updater, search.sorting) })
    },
    [updateUrlState, search.sorting]
  )
  const onColumnFiltersChange = React.useCallback(
    (updater: Updater<ColumnFiltersState>) => {
      const next = resolveUpdater(updater, columnFilters)
      updateUrlState({
        ...fromColumnFiltersRef.current(next),
        ...(resetPageIndexOnFilterChange ? { pageIndex: 0 } : {}),
      })
    },
    [updateUrlState, columnFilters, resetPageIndexOnFilterChange]
  )

  // Clamp the URL page when the data shrinks (deep-linked stale page numbers
  // would otherwise render an empty page with a confusing "no results" empty
  // state). Pass this directly to useDataTable's `ensurePageInRange`.
  const ensurePageInRange = React.useCallback(
    (pageCount: number) => {
      if (pageCount <= 0) return
      const maxIndex = pageCount - 1
      if (search.pageIndex > maxIndex) {
        updateUrlState({ pageIndex: maxIndex })
      }
    },
    [updateUrlState, search.pageIndex]
  )

  return {
    globalFilter: search.q,
    onGlobalFilterChange,
    pagination,
    onPaginationChange,
    sorting: search.sorting,
    onSortingChange,
    columnFilters,
    onColumnFiltersChange,
    filters: search.filters,
    updateUrlState,
    ensurePageInRange,
  }
}
