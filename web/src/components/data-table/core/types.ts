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
    /** Pin this column to a sticky edge (resolved by column-pinning). */
    pinned?: PinnedSide
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

/** The edge a pinned column sticks to. Declared per column via `meta.pinned`. */
export type PinnedSide = 'left' | 'right'

/** What `renderRow` gets besides the row: the pinned-aware class resolver. */
export type DataTableRenderRowHelpers = {
  getCellClassName: (columnId: string, className?: string) => string | undefined
}

/**
 * The view layer's configuration: what the body shows while loading or empty,
 * how a row is rendered, and which of the two shells to use.
 *
 * Deliberately short. It used to carry a class-name override per table part, an
 * explicit pinned-column list beside `meta.pinned`, and slots for a custom
 * colgroup / empty cell / skeleton row height — none of which any page set. An
 * override nobody uses is not flexibility, it is a second way to be wrong.
 */
export type DataTableViewProps<TData> = {
  table: TanstackTable<TData>
  /** Renders the skeleton body instead of rows. */
  isLoading?: boolean
  emptyTitle?: string
  emptyDescription?: string
  /** Extra content under the empty-state copy, e.g. a create button. */
  emptyAction?: React.ReactNode
  /** Per-entity empty-state icon (defaults to a generic database glyph). */
  emptyIcon?: React.ReactNode
  /** React key prefix for skeleton rows; distinct per table when two share a page. */
  skeletonKeyPrefix?: string
  /** Replaces the default `<tr>`/`<td>` mapping — expanded rows, row navigation. */
  renderRow?: (
    row: Row<TData>,
    helpers: DataTableRenderRowHelpers
  ) => React.ReactNode
  /** Per-column class resolver. Pinning classes are appended to its result. */
  getColumnClassName?: DataTableColumnClassName
  /**
   * Sticky-header shell: the body scrolls inside a fixed-height container while
   * the header stays put. Off renders a plain table that grows with its rows.
   */
  splitHeader?: boolean
  /** Class for the bordered container the whole table sits in. */
  containerClassName?: string
}
