// metapi-go/data-table — pinned (sticky) columns.
//
// A pinned column has to survive two things ordinary cells do not: horizontal
// scrolling under it, and the row background changing on hover and on selection.
// So it is `sticky` + `whitespace-nowrap` + an opaque background that tracks the
// row state, at a higher z-index than its neighbours, plus a soft edge shadow so
// the reader can tell content is continuing underneath.
//
// Pinning is declared on the column (`meta.pinned`) next to the rest of the
// column's presentation, and read here — there is no second, page-level list to
// keep in sync with it.

import type { Table as TanstackTable } from '@tanstack/react-table'

import { cn } from '@/lib/utils'

import type { DataTableColumnClassName, PinnedSide } from './types'

/**
 * Which side each pinned column sticks to, or undefined when nothing is pinned.
 * Undefined (not an empty map) so the resolver below can skip the lookup
 * entirely on the common, unpinned table.
 */
export function getPinnedSides<TData>(
  table: TanstackTable<TData>
): Map<string, PinnedSide> | undefined {
  const pinned: [string, PinnedSide][] = []
  for (const column of table.getAllColumns()) {
    const side = column.columnDef.meta?.pinned
    if (side) pinned.push([column.id, side])
  }

  return pinned.length === 0 ? undefined : new Map(pinned)
}

/**
 * The class resolver the view hands to every header and cell: the sticky
 * treatment for pinned columns, nothing for the rest — so it is free to apply to
 * all of them rather than making the caller ask which are pinned.
 */
export function pinnedColumnClasses(
  pinnedSides: Map<string, PinnedSide> | undefined
): DataTableColumnClassName {
  return (columnId, kind) => {
    const side = pinnedSides?.get(columnId)
    return side ? pinnedClassName(side, kind) : undefined
  }
}

function pinnedClassName(side: PinnedSide, kind: 'header' | 'cell') {
  // `--foreground` is OKLCH, so `hsl(var(--foreground))` would be invalid and the
  // whole shadow dropped. Mixing it at low alpha gives a subtle edge instead of a
  // hard dark line.
  const edgeClassName =
    side === 'left'
      ? 'shadow-[8px_0_10px_-10px_color-mix(in_oklch,var(--foreground)_12%,transparent)]'
      : 'shadow-[-8px_0_10px_-10px_color-mix(in_oklch,var(--foreground)_12%,transparent)]'

  return cn(
    'sticky whitespace-nowrap',
    side === 'left' ? 'left-0' : 'right-0',
    edgeClassName,
    // Opaque, and tracking the row state: a transparent pinned cell would let the
    // scrolled-under columns show through, and a static one would keep the old
    // background while its row highlights.
    kind === 'header'
      ? '[background-color:var(--table-header-bg,var(--table-header))] group-hover:[background-color:var(--table-header-hover)] z-30'
      : 'bg-background z-10 group-hover:bg-(--table-row-hover-bg) group-data-[state=selected]:bg-(--table-row-selected-bg)'
  )
}
