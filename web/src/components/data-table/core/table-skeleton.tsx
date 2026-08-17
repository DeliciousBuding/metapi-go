// metapi-go/data-table — ported from newapi
import type { Table } from '@tanstack/react-table'

import { Skeleton } from '@/components/ui/skeleton'
import { TableRow, TableCell } from '@/components/ui/table'
import { cn } from '@/lib/utils'

interface TableSkeletonProps<TData> {
  table: Table<TData>
  rowCount?: number
  rowHeight?: string
  keyPrefix?: string
}

export function TableSkeleton<TData>({
  table,
  rowCount,
  rowHeight = 'h-[52px]',
  keyPrefix = 'skeleton',
}: TableSkeletonProps<TData>) {
  const visibleColumns = table.getVisibleLeafColumns()

  const finalRowCount =
    rowCount ?? Math.min(table.getState().pagination?.pageSize || 20, 20)

  return (
    <>
      {Array.from({ length: finalRowCount }, (_, rowIndex) => (
        <TableRow
          key={`${keyPrefix}-${rowIndex}`}
          className={cn(rowHeight, 'border-b')}
        >
          {visibleColumns.map((column) => {
            const isSelectColumn = column.id === 'select'
            const columnSize = column.getSize()
            const barWidthPx = Math.min(Math.max(columnSize * 0.6, 32), 140)

            return (
              <TableCell key={column.id} className='py-3'>
                <Skeleton
                  className={cn(
                    'h-4 rounded-sm',
                    isSelectColumn ? 'size-4' : undefined
                  )}
                  style={
                    isSelectColumn ? undefined : { width: barWidthPx }
                  }
                />
              </TableCell>
            )
          })}
        </TableRow>
      ))}
    </>
  )
}
