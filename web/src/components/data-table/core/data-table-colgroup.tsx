// metapi-go/data-table — DataTableColgroup: declares column widths once for the
// whole table instead of repeating them on every cell of every row.
//
// Pure render half of the sizing model; the widths themselves come from
// `table-sizing.ts`, which the header cells consult too.

import type { Table as TanstackTable } from '@tanstack/react-table'

import { getColWidth, sizedColumnsWidth } from './table-sizing'

export function DataTableColgroup<TData>({
  table,
}: {
  table: TanstackTable<TData>
}) {
  const budget = sizedColumnsWidth(table)
  const resizable = table.options.enableColumnResizing === true

  return (
    <colgroup>
      {table.getVisibleLeafColumns().map((column) => (
        <col
          key={column.id}
          style={{
            width: getColWidth(column.id, column.getSize(), budget, resizable),
          }}
        />
      ))}
    </colgroup>
  )
}
