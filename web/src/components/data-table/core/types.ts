// metapi-go/data-table — ported from newapi
import type { Row, Table as TanstackTable } from '@tanstack/react-table'
import type * as React from 'react'

// Column meta extensions consumed across data-table (header auto-render,
// pinned columns, mobile/card layout). Augmented here so the package is
// self-contained — feature code defining columns gets these fields type-checked.
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

export type DataTableColumnClassName = (
  columnId: string,
  kind: 'header' | 'cell'
) => string | undefined

export type DataTablePinnedColumn = {
  columnId: string
  side: 'left' | 'right'
  className?: string
  headerClassName?: string
  cellClassName?: string
}

export type DataTableRenderRowHelpers = {
  getCellClassName: (columnId: string, className?: string) => string | undefined
}

export type DataTableViewProps<TData> = {
  table: TanstackTable<TData>
  isLoading?: boolean
  rows?: Row<TData>[]
  emptyTitle?: string
  emptyDescription?: string
  emptyIcon?: React.ReactNode
  emptyAction?: React.ReactNode
  emptyContent?: React.ReactNode
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
