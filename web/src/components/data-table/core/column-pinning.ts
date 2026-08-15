// metapi-go/data-table — ported from newapi
import { cn } from '@/lib/utils'

import type { DataTableColumnClassName, DataTablePinnedColumn } from './types'

export function getResolvedColumnClassNameFromMap(
  getColumnClassName?: DataTableColumnClassName,
  pinnedColumnById?: Map<string, DataTablePinnedColumn>
): DataTableColumnClassName {
  return (columnId, kind) => {
    const customClassName = getColumnClassName?.(columnId, kind)
    const pinnedColumn = pinnedColumnById?.get(columnId)

    if (!pinnedColumn) {
      return customClassName
    }

    return cn(customClassName, getPinnedColumnClassName(pinnedColumn, kind))
  }
}

export function getPinnedColumnMap(pinnedColumns?: DataTablePinnedColumn[]) {
  if (!pinnedColumns?.length) {
    return undefined
  }

  return new Map(pinnedColumns.map((column) => [column.columnId, column]))
}

function getPinnedColumnClassName(
  pinnedColumn: DataTablePinnedColumn,
  kind: 'header' | 'cell'
) {
  // `--foreground` is OKLCH, so `hsl(var(--foreground))` is invalid and the
  // whole shadow is dropped. Mix the foreground at low alpha for a subtle
  // pinned-column edge instead of a hard dark line.
  const edgeClassName =
    pinnedColumn.side === 'left'
      ? 'shadow-[8px_0_10px_-10px_color-mix(in_oklch,var(--foreground)_12%,transparent)]'
      : 'shadow-[-8px_0_10px_-10px_color-mix(in_oklch,var(--foreground)_12%,transparent)]'

  return cn(
    'sticky whitespace-nowrap',
    pinnedColumn.side === 'left' ? 'left-0' : 'right-0',
    edgeClassName,
    kind === 'header'
      ? '[background-color:var(--table-header-bg,var(--table-header))] group-hover:[background-color:var(--table-header-hover)] z-30'
      : 'bg-background z-10 group-hover:[background-color:color-mix(in_oklch,var(--muted)_50%,var(--background))] group-data-[state=selected]:bg-muted',
    pinnedColumn.className,
    kind === 'header'
      ? pinnedColumn.headerClassName
      : pinnedColumn.cellClassName
  )
}
