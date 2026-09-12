// metapi-go/data-table — useDataTable: the table's state layer.
//
// The only place this package touches `useReactTable`. It wires the row models
// to the page's filtering mode, hands TanStack the server's row count, and owns
// the two column preferences that survive a reload.
//
// Who owns which state is the part worth being precise about, because this is
// where the three-stage URL sync (route `validateSearch` → feature `useSearch` →
// `useDataTable`) meets the table:
//
//   URL-owned       sorting, pagination, column filters and the global filter
//                   are *controlled* here — the feature passes the value it
//                   read from the route plus an `onChange` that navigates, and
//                   the table keeps no copy of its own. That is what makes a
//                   reload or a back-navigation restore the view.
//   table-owned     row selection is left to TanStack. Nothing outside the
//                   table reads it, and a second copy would only be a second
//                   thing to keep in sync.
//   storage-owned   column visibility and sizing are uncontrolled but
//                   persisted: they are presentation, not a shareable view.
//
// Every setter below keeps the previous reference when an update changes
// nothing. That is load-bearing rather than a micro-optimisation — see
// `isShallowEqual`.
import {
  type ColumnDef,
  type ColumnFiltersState,
  type ColumnSizingState,
  type OnChangeFn,
  type PaginationState,
  type SortingState,
  type TableOptions,
  type Updater,
  type VisibilityState,
  getCoreRowModel,
  getFacetedRowModel,
  getFacetedUniqueValues,
  getFilteredRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
} from '@tanstack/react-table'
import * as React from 'react'

/** Where a page that does not control pagination starts. */
const DEFAULT_PAGINATION: PaginationState = { pageIndex: 0, pageSize: 20 }
const EMPTY_SORTING: SortingState = []
const EMPTY_VISIBILITY: VisibilityState = {}
const EMPTY_SIZING: ColumnSizingState = {}

/**
 * Shared empty column-filters value for tables without filtering.
 *
 * The table state must hold a *stable* `columnFilters` reference: a fresh
 * `[]` per render (e.g. `options.columnFilters ?? []`) makes TanStack's
 * `getFilteredRowModel` memo recompute on every render, whose invalidation
 * hook queues `resetPageIndex()` in a microtask. The reset then produces a
 * new pagination object via `setUncontrolledValue`, which re-renders and
 * re-queues the reset — an unbounded microtask render loop (the mobile
 * settings keys-page freeze: the sync sheet-open flush interleaves with the
 * microtask chain and pegs the main thread). One module-level reference
 * keeps the memo stable; TanStack never mutates the passed array.
 */
const EMPTY_COLUMN_FILTERS: ColumnFiltersState = []

/** Column sizing is written debounced: a resize drag changes it per pointer move. */
const COLUMN_SIZING_PERSIST_DELAY_MS = 250

type UseDataTableOptions<TData> = Pick<
  TableOptions<TData>,
  | 'autoResetPageIndex'
  | 'enableColumnResizing'
  | 'enableRowSelection'
  | 'getRowId'
  | 'globalFilterFn'
  | 'manualFiltering'
  | 'manualPagination'
  | 'manualSorting'
> & {
  data: TData[]
  columns: ColumnDef<TData, unknown>[]

  /**
   * Total row count reported by the server. Drives the page count and is handed
   * to TanStack as `rowCount`, so `manualPagination` pages show a real pager.
   */
  totalCount?: number

  /**
   * Called whenever the table's page count changes, so a page can pull itself
   * back into range — e.g. after deleting the last row of the last page.
   */
  ensurePageInRange?: (pageCount: number) => void

  // --- URL-owned (controlled) state -------------------------------------

  sorting?: SortingState
  onSortingChange?: OnChangeFn<SortingState>
  pagination?: PaginationState
  onPaginationChange?: OnChangeFn<PaginationState>
  columnFilters?: ColumnFiltersState
  onColumnFiltersChange?: OnChangeFn<ColumnFiltersState>
  globalFilter?: string
  onGlobalFilterChange?: OnChangeFn<string>

  // --- storage-owned column preferences ---------------------------------

  /** Visibility to start from; what is in storage wins over it. */
  initialColumnVisibility?: VisibilityState
  /** localStorage key holding column visibility. Omit to keep it in memory. */
  columnVisibilityStorageKey?: string
  /** localStorage key holding column widths. Omit to keep them in memory. */
  columnSizingStorageKey?: string
}

/**
 * Shallow structural equality (one level, reference-equal leaves) over plain
 * objects and arrays.
 *
 * A no-op table update must keep the previous reference: TanStack's
 * `resetPageIndex` emits `{ ...old, pageIndex }` even when the page index is
 * unchanged, and re-rendering on that fresh object is what re-fuels the
 * auto-reset microtask loop described on `EMPTY_COLUMN_FILTERS`. Returning the
 * same reference lets React's eager state bail-out skip the render entirely.
 */
function isShallowEqual<TValue>(a: TValue, b: TValue): boolean {
  if (Object.is(a, b)) {
    return true
  }
  if (
    typeof a !== 'object' ||
    a === null ||
    typeof b !== 'object' ||
    b === null
  ) {
    return false
  }
  if (Array.isArray(a) || Array.isArray(b)) {
    if (!Array.isArray(a) || !Array.isArray(b) || a.length !== b.length) {
      return false
    }
    return a.every((item, index) => Object.is(item, b[index]))
  }
  const aKeys = Object.keys(a)
  const bKeys = Object.keys(b)
  if (aKeys.length !== bKeys.length) {
    return false
  }
  const aRecord = a as Record<string, unknown>
  const bRecord = b as Record<string, unknown>
  return aKeys.every((key) => Object.is(aRecord[key], bRecord[key]))
}

function resolveUpdater<TValue>(
  updater: Updater<TValue>,
  previous: TValue
): TValue {
  return typeof updater === 'function'
    ? (updater as (old: TValue) => TValue)(previous)
    : updater
}

/** State the table owns alone, with the no-op bail-out above. */
function useTableState<TValue>(
  initialValue: TValue | (() => TValue)
): [TValue, OnChangeFn<TValue>] {
  const [value, setValue] = React.useState<TValue>(initialValue)

  const onChange = React.useCallback<OnChangeFn<TValue>>((updater) => {
    setValue((previous) => {
      const next = resolveUpdater(updater, previous)
      return isShallowEqual(next, previous) ? previous : next
    })
  }, [])

  return [value, onChange]
}

/**
 * State a feature owns through the URL, with the table as fallback for pages
 * that do not.
 *
 * The `onChange` is called through a ref so the setter keeps one identity: a
 * fresh setter per render re-resolves the TanStack table, which re-runs its
 * `autoResetPageIndex` effect and can feed an infinite render loop through the
 * URL sync.
 */
function useControllableTableState<TValue>(
  controlledValue: TValue | undefined,
  defaultValue: TValue,
  onChange: OnChangeFn<TValue> | undefined
): [TValue, OnChangeFn<TValue>] {
  const [uncontrolledValue, setUncontrolledValue] = useTableState(defaultValue)

  const onChangeRef = React.useRef(onChange)
  onChangeRef.current = onChange

  const setValue = React.useCallback<OnChangeFn<TValue>>(
    (updater) => {
      if (controlledValue === undefined) {
        setUncontrolledValue(updater)
      }
      onChangeRef.current?.(updater)
    },
    [controlledValue, setUncontrolledValue]
  )

  return [controlledValue ?? uncontrolledValue, setValue]
}

/**
 * A JSON object read out of localStorage, validated entry by entry.
 *
 * Everything unusable — no key, unreadable storage, invalid JSON, a non-object,
 * an entry `readEntry` rejects — is dropped rather than thrown. These keys are
 * user-editable, and a corrupt column preference must not break the table.
 */
function readStoredRecord<TValue>(
  storageKey: string | undefined,
  readEntry: (value: unknown) => TValue | undefined
): Record<string, TValue> {
  if (!storageKey) return {}

  try {
    const raw = window.localStorage.getItem(storageKey)
    if (!raw) return {}

    const parsed: unknown = JSON.parse(raw)
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return {}
    }

    const record: Record<string, TValue> = {}
    for (const [key, value] of Object.entries(parsed)) {
      const entry = readEntry(value)
      if (entry !== undefined) record[key] = entry
    }
    return record
  } catch {
    return {}
  }
}

function writeStoredRecord(storageKey: string, value: unknown) {
  try {
    window.localStorage.setItem(storageKey, JSON.stringify(value))
  } catch {
    // Private mode and full quotas are ordinary; losing the preference is not
    // worth breaking the table over.
  }
}

/** Stored column visibility: booleans, anything else dropped. */
function readStoredColumnVisibility(
  storageKey: string | undefined
): VisibilityState {
  return readStoredRecord(storageKey, (value) =>
    typeof value === 'boolean' ? value : undefined
  )
}

/**
 * Stored column widths in px: positive finite numbers, anything else dropped.
 * Out-of-range values need no clamping here — TanStack clamps a column's size
 * to its own `minSize`/`maxSize` when it is read back, so a width stored before
 * a column changed cannot escape the bounds it has now.
 */
function readStoredColumnSizing(
  storageKey: string | undefined
): ColumnSizingState {
  return readStoredRecord(storageKey, (value) =>
    typeof value === 'number' && Number.isFinite(value) && value > 0
      ? value
      : undefined
  )
}

type PersistedColumnStateOptions<TValue extends object> = {
  storageKey: string | undefined
  /** Reads and validates what is in storage. Must be a stable reference. */
  read: (storageKey: string | undefined) => TValue
  /** Values the stored ones are merged over. Must be a stable reference. */
  defaults: TValue
  /** Debounce the write; 0 writes on the change itself. */
  debounceMs?: number
}

/**
 * Column state that survives a reload: read once per storage key, written back
 * on change.
 *
 * The read is deliberately *not* per render. Reading and parsing localStorage
 * on every render is what the previous shape did (its memo depended on a
 * destructuring default that was a fresh object each time), and it is pure
 * waste: the stored value cannot change while the component is mounted.
 *
 * A write is skipped once after a re-hydration. When the storage key changes
 * under a mounted hook — the settings pages reuse one table component across
 * sections, each with its own key — the freshly read value is pushed into
 * state, and echoing it straight back to storage would be a no-op at best.
 */
function usePersistedColumnState<TValue extends object>({
  storageKey,
  read,
  defaults,
  debounceMs = 0,
}: PersistedColumnStateOptions<TValue>): [TValue, OnChangeFn<TValue>] {
  const [value, setValue] = useTableState<TValue>(() => ({
    ...defaults,
    ...read(storageKey),
  }))

  const hydratedKeyRef = React.useRef(storageKey)
  const skipNextPersistRef = React.useRef(false)

  React.useEffect(() => {
    if (storageKey === hydratedKeyRef.current) return

    hydratedKeyRef.current = storageKey
    skipNextPersistRef.current = true
    setValue(() => ({ ...defaults, ...read(storageKey) }))
  }, [defaults, read, setValue, storageKey])

  React.useEffect(() => {
    if (!storageKey) return

    if (skipNextPersistRef.current) {
      skipNextPersistRef.current = false
      return
    }

    if (debounceMs <= 0) {
      writeStoredRecord(storageKey, value)
      return
    }

    const timer = window.setTimeout(
      () => writeStoredRecord(storageKey, value),
      debounceMs
    )
    return () => window.clearTimeout(timer)
  }, [debounceMs, storageKey, value])

  return [value, setValue]
}

export function useDataTable<TData>(options: UseDataTableOptions<TData>) {
  const {
    data,
    columns,
    totalCount,
    ensurePageInRange,
    manualFiltering,
    manualPagination,
    manualSorting,
  } = options

  const [columnVisibility, onColumnVisibilityChange] =
    usePersistedColumnState<VisibilityState>({
      storageKey: options.columnVisibilityStorageKey,
      read: readStoredColumnVisibility,
      defaults: options.initialColumnVisibility ?? EMPTY_VISIBILITY,
    })

  const [columnSizing, onColumnSizingChange] =
    usePersistedColumnState<ColumnSizingState>({
      storageKey: options.columnSizingStorageKey,
      read: readStoredColumnSizing,
      defaults: EMPTY_SIZING,
      debounceMs: COLUMN_SIZING_PERSIST_DELAY_MS,
    })

  const [sorting, onSortingChange] = useControllableTableState(
    options.sorting,
    EMPTY_SORTING,
    options.onSortingChange
  )
  const [pagination, onPaginationChange] = useControllableTableState(
    options.pagination,
    DEFAULT_PAGINATION,
    options.onPaginationChange
  )

  // Sorting is only offered where it can be honoured: a server-paginated page
  // that does not wire sorting through the URL would render clickable headers
  // that reorder nothing.
  const enableSorting =
    !manualPagination ||
    options.sorting !== undefined ||
    options.onSortingChange !== undefined

  const table = useReactTable({
    data,
    columns,
    rowCount: totalCount,
    pageCount:
      totalCount !== undefined
        ? Math.ceil(totalCount / pagination.pageSize)
        : undefined,
    state: {
      sorting,
      columnVisibility,
      columnSizing,
      columnFilters: options.columnFilters ?? EMPTY_COLUMN_FILTERS,
      globalFilter: options.globalFilter ?? '',
      pagination,
    },
    enableRowSelection: options.enableRowSelection,
    enableSorting,
    getRowId: options.getRowId,
    globalFilterFn: options.globalFilterFn ?? 'auto',
    autoResetPageIndex: options.autoResetPageIndex,
    manualFiltering,
    manualPagination,
    manualSorting,
    enableColumnResizing: options.enableColumnResizing,
    columnResizeMode: 'onChange',
    onSortingChange,
    onColumnVisibilityChange,
    onColumnSizingChange,
    onColumnFiltersChange: options.onColumnFiltersChange,
    onGlobalFilterChange: options.onGlobalFilterChange,
    onPaginationChange,
    getCoreRowModel: getCoreRowModel(),
    // Row models follow the mode: a `manual*` page does that work server-side,
    // and a client-side model here would re-filter or re-sort only the rows of
    // the current page — contradicting the count the server reported.
    getFilteredRowModel: manualFiltering ? undefined : getFilteredRowModel(),
    getPaginationRowModel: manualPagination
      ? undefined
      : getPaginationRowModel(),
    getSortedRowModel:
      manualSorting || manualPagination ? undefined : getSortedRowModel(),
    getFacetedRowModel: manualFiltering ? undefined : getFacetedRowModel(),
    getFacetedUniqueValues: manualFiltering
      ? undefined
      : getFacetedUniqueValues(),
  })

  const resolvedPageCount = table.getPageCount()
  React.useEffect(() => {
    ensurePageInRange?.(resolvedPageCount)
  }, [ensurePageInRange, resolvedPageCount])

  return {
    table,
  }
}
