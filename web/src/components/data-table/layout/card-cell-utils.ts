// metapi-go/data-table — ported from newapi
import { flexRender, type Cell, type Table } from '@tanstack/react-table'
import type { ReactNode } from 'react'

/**
 * Shared cell helpers for the column-meta-driven card content used by both the
 * mobile list and the desktop card grid. Kept separate from the card content
 * component so the module exports only non-component utilities.
 */

export function getCellLabel<TData>(cell: Cell<TData, unknown>): string | null {
  const { header, meta } = cell.column.columnDef
  if (typeof header === 'string') {
    return header
  }
  if (meta?.label) {
    return meta.label
  }
  return null
}

export function renderCellContent<TData>(
  cell: Cell<TData, unknown>
): ReactNode {
  const cellRenderer = cell.column.columnDef.cell
  if (cellRenderer) {
    return flexRender(cellRenderer, cell.getContext())
  }
  return cell.getValue() as ReactNode
}

/**
 * Whether any visible column declares `mobileTitle`/`mobileBadge` meta. When
 * true the compact two-tier layout is used; otherwise the condensed
 * label:value fallback layout is used.
 */
export function tableHasCompactMeta<TData>(table: Table<TData>): boolean {
  return table.getVisibleLeafColumns().some((col) => {
    const meta = col.columnDef.meta
    return Boolean(meta?.mobileTitle || meta?.mobileBadge)
  })
}
