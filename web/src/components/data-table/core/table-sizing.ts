// metapi-go/data-table — the column width model.
//
// Two kinds of column take part:
//   sized          carries a TanStack size and a share of the width budget
//   content-sized  collapses to its content (today: the actions column)
//
// This module is the whole model and nothing else — pure functions, no JSX, so
// it stays on the non-component side of the `only-export-components` boundary.
// `data-table-colgroup.tsx` renders it, `data-table-header.tsx` consults the
// same predicate so a content-sized column is never given an explicit width,
// and `getTableSizeStyle` reserves the budget on the `<table>` itself. The three
// have to agree, which is why they share one definition of "the budget".

import type { Table as TanstackTable } from '@tanstack/react-table'
import type { CSSProperties } from 'react'

/**
 * Columns that size to their content instead of taking a share of the width
 * budget. `actions` is the only one: it holds a fixed set of icon buttons, so a
 * budget share would stretch it with the viewport for no reason.
 */
export function isContentSizedColumn(columnId: string): boolean {
  return columnId === 'actions'
}

/** Sum of the visible sized columns' widths — the table's width budget. */
export function sizedColumnsWidth<TData>(table: TanstackTable<TData>): number {
  return table
    .getVisibleLeafColumns()
    .filter((column) => !isContentSizedColumn(column.id))
    .reduce((total, column) => total + column.getSize(), 0)
}

/**
 * Inline style for the `<table>` element.
 *
 * `min-width: max(100%, budget)` makes the table fill its container but never
 * squeeze the sized columns below their budget — past that point the container
 * scrolls horizontally instead. `table-layout: auto` is what allows a
 * content-sized column to collapse to 1%.
 */
export function getTableSizeStyle<TData>(
  table: TanstackTable<TData>
): CSSProperties {
  return {
    minWidth: `max(100%, ${sizedColumnsWidth(table)}px)`,
    tableLayout: 'auto',
    width: '100%',
  }
}

/**
 * Width for one `<col>`.
 *
 * `1%` is the CSS idiom for "no wider than your content" under
 * `table-layout: auto`. A resizable table needs absolute pixels, because that
 * is what a drag handle has to write back to; otherwise each sized column takes
 * its percentage of the budget, which keeps the table fluid. No budget at all
 * (every visible column content-sized) means no width, i.e. plain auto layout.
 */
export function getColWidth(
  columnId: string,
  columnSize: number,
  budget: number,
  resizable: boolean
): string | undefined {
  if (isContentSizedColumn(columnId)) return '1%'
  if (resizable) return `${columnSize}px`
  if (budget <= 0) return undefined
  return `${(columnSize / budget) * 100}%`
}
