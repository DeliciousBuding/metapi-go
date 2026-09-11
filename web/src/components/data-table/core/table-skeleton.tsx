// metapi-go/data-table — TableSkeleton: the loading body.
//
// It mirrors the real table's geometry — same visible columns, same row count,
// same row height — so the swap to loaded rows moves as little as possible.
// A generic three-line shimmer in place of the real shape makes the table jump
// when data lands, which reads as a layout bug rather than as loading.
//
// The bars are deliberately not full-width: each one is a fraction of its
// column's own budget, clamped, so a wide column does not get a wall of grey and
// a narrow one does not get an unreadable sliver.

import type { Table } from '@tanstack/react-table'

import { Skeleton } from '@/components/ui/skeleton'
import { TableCell, TableRow } from '@/components/ui/table'
import { cn } from '@/lib/utils'

/**
 * Rows to paint when the caller does not say. Follows the page size so the
 * skeleton matches the table it is standing in for, capped so a 100-row page
 * does not paint 100 shimmer rows — past ~20 the viewport is full and the extra
 * rows are pure work.
 */
const MAX_SKELETON_ROWS = 20
const FALLBACK_ROW_COUNT = 20

/** Bar width as a fraction of the column budget, clamped to a readable range. */
const BAR_WIDTH_RATIO = 0.6
const BAR_MIN_PX = 32
const BAR_MAX_PX = 140

type TableSkeletonProps<TData> = {
  table: Table<TData>
  rowCount?: number
  rowHeight?: string
  /** React key prefix; distinct per table when two skeletons share a page. */
  keyPrefix?: string
}

export function TableSkeleton<TData>({
  table,
  rowCount,
  rowHeight = 'h-[52px]',
  keyPrefix = 'skeleton',
}: TableSkeletonProps<TData>) {
  const visibleColumns = table.getVisibleLeafColumns()
  const rows =
    rowCount ??
    Math.min(
      table.getState().pagination?.pageSize || FALLBACK_ROW_COUNT,
      MAX_SKELETON_ROWS
    )

  return (
    <>
      {Array.from({ length: rows }, (_, rowIndex) => (
        <TableRow
          key={`${keyPrefix}-${rowIndex}`}
          className={cn(rowHeight, 'border-b')}
        >
          {visibleColumns.map((column) => {
            // The selection column holds a checkbox, so its placeholder is the
            // checkbox's square rather than a text bar.
            const isSelectColumn = column.id === 'select'
            const barWidthPx = Math.min(
              Math.max(column.getSize() * BAR_WIDTH_RATIO, BAR_MIN_PX),
              BAR_MAX_PX
            )

            return (
              <TableCell key={column.id} className='py-3'>
                <Skeleton
                  className={cn(
                    'h-4 rounded-sm',
                    isSelectColumn ? 'size-4' : undefined
                  )}
                  style={isSelectColumn ? undefined : { width: barWidthPx }}
                />
              </TableCell>
            )
          })}
        </TableRow>
      ))}
    </>
  )
}
