// metapi-go/data-table — pinned (sticky) columns.
//
// A pinned column has to survive two things ordinary cells do not: horizontal
// scrolling under it, and the row background changing on hover and on selection.
// So it is `sticky` + `whitespace-nowrap` + an opaque background that tracks the
// row state, at a higher z-index than its neighbours, plus a soft edge shadow so
// the reader can tell content is continuing underneath.
//
// The class map is built once per render (`getPinnedColumnMap`) and then merged
// with whatever per-column class the feature supplied
// (`getResolvedColumnClassNameFromMap`), so a feature can style a cell without
// knowing whether it is pinned.

import { cn } from '@/lib/utils'

import type { DataTableColumnClassName, DataTablePinnedColumn } from './types'

/** Pinned columns by id, or undefined when nothing is pinned. */
export function getPinnedColumnMap(pinnedColumns?: DataTablePinnedColumn[]) {
  if (!pinnedColumns?.length) return undefined

  return new Map(pinnedColumns.map((column) => [column.columnId, column]))
}

/**
 * The class resolver the view hands to every header and cell: the feature's own
 * class first, then — only for a pinned column — the sticky treatment. Unpinned
 * columns get the feature's class untouched, so this is free to apply everywhere.
 */
export function getResolvedColumnClassNameFromMap(
  getColumnClassName?: DataTableColumnClassName,
  pinnedColumnById?: Map<string, DataTablePinnedColumn>
): DataTableColumnClassName {
  return (columnId, kind) => {
    const customClassName = getColumnClassName?.(columnId, kind)
    const pinnedColumn = pinnedColumnById?.get(columnId)

    if (!pinnedColumn) return customClassName

    return cn(customClassName, pinnedColumnClassName(pinnedColumn, kind))
  }
}

function pinnedColumnClassName(
  pinnedColumn: DataTablePinnedColumn,
  kind: 'header' | 'cell'
) {
  // `--foreground` is OKLCH, so `hsl(var(--foreground))` would be invalid and the
  // whole shadow dropped. Mixing it at low alpha gives a subtle edge instead of a
  // hard dark line.
  const edgeClassName =
    pinnedColumn.side === 'left'
      ? 'shadow-[8px_0_10px_-10px_color-mix(in_oklch,var(--foreground)_12%,transparent)]'
      : 'shadow-[-8px_0_10px_-10px_color-mix(in_oklch,var(--foreground)_12%,transparent)]'

  return cn(
    'sticky whitespace-nowrap',
    pinnedColumn.side === 'left' ? 'left-0' : 'right-0',
    edgeClassName,
    // Opaque, and tracking the row state: a transparent pinned cell would let the
    // scrolled-under columns show through, and a static one would keep the old
    // background while its row highlights.
    kind === 'header'
      ? '[background-color:var(--table-header-bg,var(--table-header))] group-hover:[background-color:var(--table-header-hover)] z-30'
      : 'bg-background z-10 group-hover:bg-(--table-row-hover-bg) group-data-[state=selected]:bg-(--table-row-selected-bg)',
    pinnedColumn.className,
    kind === 'header'
      ? pinnedColumn.headerClassName
      : pinnedColumn.cellClassName
  )
}
