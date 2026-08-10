// metapi-go/data-table — ported from newapi
import type { Table as TanstackTable } from '@tanstack/react-table'

import { isContentSizedColumn } from './content-sized-columns'

export function DataTableColgroup<TData>({
  table,
}: {
  table: TanstackTable<TData>
}) {
  const columns = table.getVisibleLeafColumns()
  const sizedColumns = columns.filter(
    (column) => !isContentSizedColumn(column.id)
  )
  const totalSize = sizedColumns.reduce((sum, col) => sum + col.getSize(), 0)

  return (
    <colgroup>
      {columns.map((column) => {
        const width = getColumnWidth(
          table,
          column.id,
          column.getSize(),
          totalSize
        )

        return <col key={column.id} style={{ width }} />
      })}
    </colgroup>
  )
}

function getColumnWidth<TData>(
  table: TanstackTable<TData>,
  columnId: string,
  columnSize: number,
  totalSize: number
) {
  if (isContentSizedColumn(columnId)) {
    return '1%'
  }

  if (table.options.enableColumnResizing === true) {
    return `${columnSize}px`
  }

  if (totalSize <= 0) {
    return undefined
  }

  return `${(columnSize / totalSize) * 100}%`
}
