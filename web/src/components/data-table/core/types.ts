// metapi-go/data-table — the package's shared vocabulary: the `ColumnMeta`
// augmentation every column definition is type-checked against, and the props of
// the view layer.
//
// The augmentation is the important half. A feature declares `mobileTitle`,
// `pinned` or `label` on a column and the responsive layout, the pinning map and
// the column toggle all read it; augmenting TanStack's `ColumnMeta` here is what
// makes those fields typed at the declaration site instead of being stringly
// keyed lookups that fail silently. It lives in this package so the data-table
// stays self-contained.
import type { Row, Table as TanstackTable } from '@tanstack/react-table'
import type * as React from 'react'

declare module '@tanstack/react-table' {
  interface ColumnMeta<TData, TValue> {
    /** Header label fallback when `header` is a function (used by column-header auto-render + view-options). */
    label?: string
    /** Pin this column to a sticky edge (resolved by data-table-view). */
    pinned?: 'left' | 'right'
    /** Card/mobile: render this column's cell as the card title (left, larger text). */
    mobileTitle?: boolean
    /** Card/mobile: render this column's cell inline with the title (right, e.g. status badge). */
    mobileBadge?: boolean
    /** Card/mobile: hide this column's cell in card content. */
    mobileHidden?: boolean
    /** Card/mobile: sort order within the card's field list (ascending; null/undefined sinks to bottom). */
    mobileOrder?: number
  }
}

/** Resolves a header's or cell's class from its column id. */
export type DataTableColumnClassName = (
  columnId: string,
  kind: 'header' | 'cell'
) => string | undefined

/**
 * A column pinned to a sticky edge. `className` applies to both header and cell;
 * the two specific ones are appended after it, so they win.
 */
export type DataTablePinnedColumn = {
  columnId: string
  side: 'left' | 'right'
  className?: string
  headerClassName?: string
  cellClassName?: string
}

/** What `renderRow` gets besides the row: the pinned-aware class resolver. */
export type DataTableRenderRowHelpers = {
  getCellClassName: (columnId: string, className?: string) => string | undefined
}

/**
 * The view layer's configuration. Broad by design — it is the single place a
 * feature customises table chrome (empty copy, skeleton shape, pinned columns,
 * split header, per-part class names) without forking the renderer.
 */
export type DataTableViewProps<TData> = {
  table: TanstackTable<TData>
  isLoading?: boolean
  rows?: Row<TData>[]
  emptyTitle?: string
  emptyDescription?: string
  emptyIcon?: React.ReactNode
  emptyAction?: React.ReactNode
  emptyContent?: React.ReactNode
  filteredEmptyTitle?: string
  filteredEmptyDescription?: string
  emptyCellClassName?: string
  skeletonKeyPrefix?: string
  skeletonRowHeight?: string
  renderRow?: (
    row: Row<TData>,
    helpers: DataTableRenderRowHelpers
  ) => React.ReactNode
  getRowClassName?: (row: Row<TData>) => string | undefined
  getColumnClassName?: DataTableColumnClassName
  pinnedColumns?: DataTablePinnedColumn[]
  applyHeaderSize?: boolean
  tableClassName?: string
  tableHeaderClassName?: string
  tableHeaderRowClassName?: string
  tableBodyClassName?: string
  tableBodyRowClassName?: string
  splitHeader?: boolean
  splitHeaderScrollClassName?: string
  bodyContainerClassName?: string
  containerClassName?: string
  containerProps?: Omit<React.ComponentProps<'div'>, 'className' | 'children'>
  tableContainerClassName?: string
  colgroup?: React.ReactNode
}
