// metapi-go/data-table — ported from newapi
import type { Table as TanstackTable } from '@tanstack/react-table'
import type * as React from 'react'

import { isContentSizedColumn } from './content-sized-columns'

export function getTableSizeStyle<TData>(
  table: TanstackTable<TData>
): React.CSSProperties {
  const width = table
    .getVisibleLeafColumns()
    .filter((column) => !isContentSizedColumn(column.id))
    .reduce((total, column) => total + column.getSize(), 0)

  return {
    minWidth: `max(100%, ${width}px)`,
    tableLayout: 'auto',
    width: '100%',
  }
}
