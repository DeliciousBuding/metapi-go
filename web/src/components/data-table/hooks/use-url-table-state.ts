// metapi-go/data-table — shared URL-synced table state hook.
//
// List pages mirror table state (search / page / page-size / sorting / faceted
// filters) to the URL so deep links restore the exact view. This hook owns the
// TanStack Router plumbing and, critically, keeps every callback handed to
// TanStack Table referentially stable. Table libraries may call controlled
// state callbacks from effects; rebuilding those callbacks every render can
// turn URL synchronization into a render/navigation feedback loop.

import { useLocation, useNavigate } from '@tanstack/react-router'
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

export type UrlTableStateOptions<TFilters> = {
  /** Canonical pathname for the page (e.g. '/sites'). */
  basePath: string
  /** Parse validated URL state from a raw search string. */
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
 *
 * The page-supplied parser/serializer functions are intentionally read through
 * refs. Callers commonly declare them inline; allowing those function
 * identities into callback dependency arrays recreates every table callback on
 * every render, which can produce an update-depth loop with controlled tables.
 */
export function useUrlTableState<TFilters>(
  options: UrlTableStateOptions<TFilters>
) {
  const navigate = useNavigate()
  const searchStr = useLocation({ select: (loc) => loc.searchStr })

  const navigateRef = React.useRef(navigate)
  const readRef = React.useRef(options.read)
  const buildHrefRef = React.useRef(options.buildHref)
  const toColumnFiltersRef = React.useRef(options.toColumnFilters)
  const fromColumnFiltersRef = React.useRef(options.fromColumnFilters)

  navigateRef.current = navigate
  readRef.current = options.read
  buildHrefRef.current = options.buildHref
  toColumnFiltersRef.current = options.toColumnFilters
  fromColumnFiltersRef.current = options.fromColumnFilters

  const search = React.useMemo(() => readRef.current(searchStr), [searchStr])
  const columnFilters = React.useMemo(
    () => toColumnFiltersRef.current(search.filters),
    [search.filters]
  )
  const pagination = React.useMemo<PaginationState>(
    () => ({ pageIndex: search.pageIndex, pageSize: search.pageSize }),
    [search.pageIndex, search.pageSize]
  )

  const searchRef = React.useRef(search)
  const columnFiltersRef = React.useRef(columnFilters)
  searchRef.current = search
  columnFiltersRef.current = columnFilters

  // Navigation guard: controlled table callbacks can fire while a route is
  // unmounting. Only synchronize when both the current and target pathname are
  // this table page. Also suppress no-op replaces; besides avoiding needless
  // history work, this removes another possible render loop edge.
  const syncUrl = React.useCallback(
    (next: Partial<UrlTableState<TFilters>>) => {
      if (typeof window === 'undefined') return
      if (window.location.pathname !== options.basePath) return

      const href = buildHrefRef.current(next)
      const target = new URL(href, window.location.origin)
      if (target.pathname !== options.basePath) return

      const currentHref = `${window.location.pathname}${window.location.search}`
      const targetHref = `${target.pathname}${target.search}`
      if (currentHref === targetHref) return

      void navigateRef.current({ href, replace: true })
    },
    [options.basePath]
  )

  const onGlobalFilterChange = React.useCallback(
    (updater: Updater<string>) => {
      const current = searchRef.current
      syncUrl({
        q: resolveUpdater(updater, current.q),
        pageIndex: 0,
      })
    },
    [syncUrl]
  )

  const onPaginationChange = React.useCallback(
    (updater: Updater<PaginationState>) => {
      const current = searchRef.current
      const next = resolveUpdater(updater, {
        pageIndex: current.pageIndex,
        pageSize: current.pageSize,
      })
      syncUrl({ pageIndex: next.pageIndex, pageSize: next.pageSize })
    },
    [syncUrl]
  )

  const onSortingChange = React.useCallback(
    (updater: Updater<SortingState>) => {
      const current = searchRef.current
      syncUrl({
        sorting: resolveUpdater(updater, current.sorting),
        pageIndex: 0,
      })
    },
    [syncUrl]
  )

  const onColumnFiltersChange = React.useCallback(
    (updater: Updater<ColumnFiltersState>) => {
      const next = resolveUpdater(updater, columnFiltersRef.current)
      syncUrl({
        ...fromColumnFiltersRef.current(next),
        pageIndex: 0,
      })
    },
    [syncUrl]
  )

  // Clamp stale deep-linked pages when the filtered data shrinks.
  const ensurePageInRange = React.useCallback(
    (pageCount: number) => {
      if (pageCount <= 0) return
      const maxIndex = pageCount - 1
      if (searchRef.current.pageIndex > maxIndex) {
        syncUrl({ pageIndex: maxIndex })
      }
    },
    [syncUrl]
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
    ensurePageInRange,
  }
}
