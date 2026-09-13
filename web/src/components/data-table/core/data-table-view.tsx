// metapi-go/data-table — DataTableView: the <table> itself.
//
// Two shells, one body. Pages default to the fixed-height shell (`splitHeader`):
// the body scrolls inside a bounded container while the header stays put, which
// is what keeps column headings readable on a hundred-row page. A page that
// would rather grow with its rows turns it off and gets a plain table.
//
// The shells differ only in their scroll container. Caption, colgroup, header
// and body are the same nodes in both, so the two cannot drift.
//
// Under the header there are exactly three things to render: the loading
// skeleton, the empty state, or rows — the feature's own `renderRow` when it
// needs one (expandable rows, row-level navigation), `DataTableRow` otherwise.
import type { Row, Table as TanstackTable } from '@tanstack/react-table'
import type { CSSProperties, ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Table, TableBody } from '@/components/ui/table'
import { formatInt } from '@/lib/format'
import { cn } from '@/lib/utils'

import { getPinnedSides, pinnedColumnClasses } from './column-pinning'
import { DataTableColgroup } from './data-table-colgroup'
import { DataTableHeader } from './data-table-header'
import { DataTableRow } from './data-table-row'
import { TableEmpty } from './table-empty'
import { getTableSizeStyle } from './table-sizing'
import { TableSkeleton } from './table-skeleton'
import type { DataTableColumnClassName, DataTableViewProps } from './types'

/**
 * A sticky header has to be opaque or the rows scrolling under it show through.
 * The colour travels as a custom property so a pinned header cell — which paints
 * its own opaque background — reads the same value instead of hardcoding a
 * second one (see `column-pinning`).
 */
const STICKY_HEADER_BACKGROUND =
  '**:data-[slot=table-header]:[--table-header-bg:var(--table-header)] **:data-[slot=table-header]:bg-(--table-header-bg)'

/** What the `ui/table` primitive paints on the <table> element itself. */
const TABLE_CLASS = 'w-full caption-bottom text-sm tabular-nums'

export function DataTableView<TData>(props: DataTableViewProps<TData>) {
  const { t } = useTranslation()
  const { table, splitHeader } = props
  // Read on every render rather than memoised on `table`: hiding a column in the
  // view menu changes this count while the table object keeps its identity.
  const rows = table.getRowModel().rows
  const colSpan = table.getVisibleLeafColumns().length
  const getColumnClassName = pinnedColumnClasses(getPinnedSides(table))

  const { pageIndex, pageSize } = table.getState().pagination
  const totalRows = table.getRowCount()
  const caption = t('dataTable.summary', {
    start: formatInt(totalRows === 0 ? 0 : pageIndex * pageSize + 1),
    end: formatInt(pageIndex * pageSize + rows.length),
    total: formatInt(totalRows),
  })

  // The column budget is only worth computing — and only means anything — when
  // the header is separated from the body and the two have to agree on widths.
  const sizing: { colgroup?: ReactNode; style?: CSSProperties } = splitHeader
    ? {
        colgroup: <DataTableColgroup table={table} />,
        style: getTableSizeStyle(table),
      }
    : {}

  const tableContent = (
    <>
      <caption className='sr-only'>{caption}</caption>
      {sizing.colgroup}
      <DataTableHeader
        table={table}
        className={splitHeader ? 'sticky top-0 z-10' : undefined}
        getColumnClassName={getColumnClassName}
      />
      <TableBody>
        {renderBody(props, rows, colSpan, getColumnClassName)}
      </TableBody>
    </>
  )

  return (
    <div
      className={cn(
        'overflow-hidden rounded-lg border',
        props.containerClassName
      )}
    >
      {splitHeader ? (
        <div className='flex h-full min-h-0 flex-col'>
          <div
            className={cn(
              'min-h-0 flex-1 overflow-auto',
              STICKY_HEADER_BACKGROUND
            )}
          >
            {/* The split shell cannot use the `ui/table` primitive: that
                primitive brings its own `overflow-y-hidden` container, and a
                sticky header inside it has nothing to stick to. */}
            <table
              data-slot='table'
              data-table-stagger='true'
              className={TABLE_CLASS}
              style={sizing.style}
            >
              {tableContent}
            </table>
          </div>
        </div>
      ) : (
        <Table style={sizing.style}>{tableContent}</Table>
      )}
    </div>
  )
}

function renderBody<TData>(
  props: DataTableViewProps<TData>,
  rows: Row<TData>[],
  colSpan: number,
  getColumnClassName: DataTableColumnClassName
): ReactNode {
  if (props.isLoading) {
    return (
      <TableSkeleton table={props.table} keyPrefix={props.skeletonKeyPrefix} />
    )
  }

  if (rows.length === 0) {
    return renderEmptyState(props, colSpan)
  }

  return rows.map((row) =>
    props.renderRow
      ? props.renderRow(row, {
          // A custom row still has to look pinned where the default one would,
          // so the resolver is handed over rather than reimplemented.
          getCellClassName: (columnId, className) =>
            cn(getColumnClassName(columnId, 'cell'), className),
        })
      : renderDefaultRow(props.table, row, getColumnClassName)
  )
}

function renderEmptyState<TData>(
  props: DataTableViewProps<TData>,
  colSpan: number
) {
  const state = props.table.getState()

  return (
    <TableEmpty
      colSpan={colSpan}
      title={props.emptyTitle}
      description={props.emptyDescription}
      icon={props.emptyIcon}
      isFiltered={
        (state.columnFilters ?? []).length > 0 || Boolean(state.globalFilter)
      }
      onClearFilters={() => {
        props.table.resetColumnFilters()
        props.table.resetGlobalFilter()
      }}
    >
      {props.emptyAction}
    </TableEmpty>
  )
}

function renderDefaultRow<TData>(
  table: TanstackTable<TData>,
  row: Row<TData>,
  getColumnClassName: DataTableColumnClassName
) {
  return (
    <DataTableRow
      key={row.id}
      row={row}
      getColumnClassName={getColumnClassName}
      cellRenderColumns={table.options.columns}
    />
  )
}
