// metapi-go/data-table — cell helpers behind the card renderings.
//
// The mobile card list and the desktop card grid both render a row as
// label/value pairs instead of as cells, and both drive that off the column meta
// features already declare. These helpers are the shared half; they live apart
// from `card-row-content.tsx` so this module exports no components (the
// `react-refresh/only-export-components` boundary).

import { flexRender, type Cell, type Table } from '@tanstack/react-table'
import type { ReactNode } from 'react'

/**
 * The label to show beside a cell in a card layout: the column's plain-string
 * header when it has one, else the explicit `meta.label` a column declares when
 * its header is a sort/filter component with no string to reuse. `null` when
 * neither exists, which tells the caller to render the value on its own.
 */
export function getCellLabel<TData>(cell: Cell<TData, unknown>): string | null {
  const { header, meta } = cell.column.columnDef

  if (typeof header === 'string') return header
  return meta?.label || null
}

/** Runs the column's `cell` renderer, falling back to the raw value. */
export function renderCellContent<TData>(
  cell: Cell<TData, unknown>
): ReactNode {
  const renderer = cell.column.columnDef.cell

  return renderer
    ? flexRender(renderer, cell.getContext())
    : (cell.getValue() as ReactNode)
}

/**
 * Whether any visible column declares `mobileTitle` / `mobileBadge` meta.
 *
 * When one does, the card list uses the compact two-tier layout those fields
 * describe; when none does it falls back to the condensed label/value layout.
 * Decided per table rather than per row so every card in a list has the same
 * shape.
 */
export function tableHasCompactMeta<TData>(table: Table<TData>): boolean {
  return table.getVisibleLeafColumns().some(({ columnDef }) => {
    const { mobileBadge, mobileTitle } = columnDef.meta ?? {}
    return Boolean(mobileTitle || mobileBadge)
  })
}
