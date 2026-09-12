// metapi-go/data-table — DataTableHeader: the <thead>, and the column resizer
// that lives in it.
//
// Two jobs that belong together because they share the header cell:
//
//   rendering   each `th` gets its column's content, its `aria-sort`, and the
//               class its column resolves to (pinning lives there, not here).
//               Widths are not this file's business: the fixed-height shell lays
//               them out through `data-table-colgroup` so header and body agree,
//               and the plain shell leaves the table to auto-layout.
//               Content-sized columns get no resizer — resizing `actions` is
//               meaningless when its width is defined by its content.
//   resizing    pointer drag (TanStack's own handler), plus a keyboard path the
//               drag handler cannot provide: arrows step the width (Shift for a
//               coarser step), Enter/Space re-fits the column to its content.
//
// The re-fit measures rather than guesses. A column's natural width is the widest
// cell in it, and the only way to know that is to lay the content out unconstrained
// — so each cell is cloned off-screen at `width: max-content` and its scrollWidth
// read. Cloning is what makes it safe: measuring the live cell would require
// undoing the table's own layout first.
import {
  flexRender,
  type Header,
  type Table as TanstackTable,
} from '@tanstack/react-table'
import type { KeyboardEvent } from 'react'
import { useTranslation } from 'react-i18next'

import { TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { cn } from '@/lib/utils'

import { DataTableColumnHeader } from './column-header'
import { columnLabel } from './column-label'
import { isContentSizedColumn } from './table-sizing'
import type { DataTableColumnClassName } from './types'

type DataTableHeaderProps<TData> = {
  table: TanstackTable<TData>
  className?: string
  getColumnClassName?: DataTableColumnClassName
}

export function DataTableHeader<TData>({
  table,
  className,
  getColumnClassName,
}: DataTableHeaderProps<TData>) {
  const { t } = useTranslation()

  return (
    <TableHeader className={className}>
      {table.getHeaderGroups().map((headerGroup) => (
        <TableRow key={headerGroup.id}>
          {headerGroup.headers.map((header) => (
            <TableHead
              key={header.id}
              colSpan={header.colSpan}
              data-column-id={header.column.id}
              aria-sort={getAriaSort(header)}
              className={cn(
                'relative',
                getColumnClassName?.(header.column.id, 'header')
              )}
            >
              {renderHeaderContent(header)}
              {shouldRenderColumnResizer(table, header) && (
                <div
                  role='separator'
                  aria-orientation='vertical'
                  aria-label={t('Resize column')}
                  aria-valuenow={Math.round(header.getSize())}
                  data-column-resizer
                  tabIndex={0}
                  onDoubleClick={(event) => {
                    event.preventDefault()
                    autoSizeColumn(event.currentTarget, table, header)
                  }}
                  onMouseDown={header.getResizeHandler()}
                  onTouchStart={header.getResizeHandler()}
                  onKeyDown={(event) =>
                    handleColumnResizeKeyDown(event, table, header)
                  }
                  className={cn(
                    'absolute top-0 right-0 h-full w-2 cursor-col-resize touch-none select-none',
                    'after:bg-border hover:after:bg-primary after:absolute after:top-2 after:right-0 after:h-[calc(100%-1rem)] after:w-px after:transition-colors',
                    header.column.getIsResizing() && 'after:bg-primary'
                  )}
                />
              )}
            </TableHead>
          ))}
        </TableRow>
      ))}
    </TableHeader>
  )
}

function handleColumnResizeKeyDown<TData>(
  event: KeyboardEvent<HTMLDivElement>,
  table: TanstackTable<TData>,
  header: Header<TData, unknown>
) {
  const step = event.shiftKey ? 50 : 10

  if (event.key === 'ArrowLeft') {
    event.preventDefault()
    resizeColumnByKeyboard(table, header, -step)
    return
  }

  if (event.key === 'ArrowRight') {
    event.preventDefault()
    resizeColumnByKeyboard(table, header, step)
    return
  }

  if (event.key === 'Enter' || event.key === ' ') {
    event.preventDefault()
    autoSizeColumn(event.currentTarget, table, header)
  }
}

function resizeColumnByKeyboard<TData>(
  table: TanstackTable<TData>,
  header: Header<TData, unknown>,
  delta: number
) {
  table.setColumnSizing((previous) => ({
    ...previous,
    [header.column.id]: getClampedColumnSize(
      header,
      header.column.getSize() + delta
    ),
  }))
}

function autoSizeColumn<TData>(
  resizerElement: HTMLElement,
  table: TanstackTable<TData>,
  header: Header<TData, unknown>
) {
  const measuredSize = measureColumnContentWidth(
    resizerElement,
    header.column.id
  )

  if (measuredSize === undefined) {
    return
  }

  table.setColumnSizing((previous) => ({
    ...previous,
    [header.column.id]: getClampedColumnSize(header, measuredSize),
  }))
}

function getClampedColumnSize<TData>(
  header: Header<TData, unknown>,
  nextSize: number
) {
  const { minSize, maxSize } = header.column.columnDef

  if (typeof minSize === 'number' && nextSize < minSize) {
    return minSize
  }

  if (typeof maxSize === 'number' && nextSize > maxSize) {
    return maxSize
  }

  return nextSize
}

/** The widest laid-out width among the column's own cells, or undefined if
 *  there is nothing to measure. `undefined` (not 0) so the caller can tell
 *  "nothing to go on" from "measured as zero" and leave the width alone. */
function measureColumnContentWidth(
  resizerElement: HTMLElement,
  columnId: string
) {
  const tableElement = resizerElement.closest('table')
  if (!tableElement) return undefined

  // Matched on the dataset rather than by an attribute selector: column ids are
  // arbitrary strings, and comparing them in JS needs no escaping.
  const cells = [
    ...tableElement.querySelectorAll<HTMLElement>('[data-column-id]'),
  ].filter((cell) => cell.dataset.columnId === columnId)
  if (cells.length === 0) return undefined

  const measuredWidth = cells.reduce(
    (widest, cell) => Math.max(widest, measureElementWidth(cell)),
    0
  )

  return measuredWidth > 0 ? Math.ceil(measuredWidth) : undefined
}

/**
 * The width `element` would take if nothing constrained it.
 *
 * Measured on an off-screen clone: the live cell is inside a table whose layout
 * is exactly what we are trying to look past, and the resizer handle inside it is
 * removed first so the handle's own 8px never counts as content.
 */
function measureElementWidth(element: HTMLElement) {
  const clone = element.cloneNode(true) as HTMLElement
  clone.querySelectorAll('[data-column-resizer]').forEach((handle) => {
    handle.remove()
  })

  Object.assign(clone.style, {
    height: 'auto',
    left: '-10000px',
    maxWidth: 'none',
    minWidth: '0',
    pointerEvents: 'none',
    position: 'absolute',
    top: '0',
    visibility: 'hidden',
    whiteSpace: 'nowrap',
    width: 'max-content',
  })

  document.body.append(clone)
  const width = clone.scrollWidth
  clone.remove()

  return width
}

function shouldRenderColumnResizer<TData>(
  table: TanstackTable<TData>,
  header: Header<TData, unknown>
) {
  return (
    table.options.enableColumnResizing === true &&
    !header.isPlaceholder &&
    header.column.getCanResize() &&
    !isContentSizedColumn(header.column.id)
  )
}

/** Sortable columns announce their state on the th (aria-sort) so AT users
 * hear the direction from the header itself; non-sortable columns stay
 * silent — aria-sort is meaningless on a th that cannot sort. */
function getAriaSort<TData>(header: Header<TData, unknown>) {
  if (!header.column.getCanSort()) return undefined
  const sorted = header.column.getIsSorted()
  if (sorted === 'asc') return 'ascending'
  if (sorted === 'desc') return 'descending'
  return 'none'
}

/**
 * What goes inside the `th`.
 *
 * A column that can be named in plain text — a string `header`, or the
 * `meta.label` a column declares when its header is a component — is wrapped in
 * `DataTableColumnHeader`, so sorting comes for free without the feature writing
 * the dropdown itself. Anything else (a function header, including TanStack's
 * accessor-key fallback) is rendered as the column asked. The naming rule is
 * shared with the card layout and the column toggle (`core/column-label.ts`).
 */
function renderHeaderContent<TData>(header: Header<TData, unknown>) {
  if (header.isPlaceholder) return null

  const label = columnLabel(header.column.columnDef)
  if (label !== null) {
    return <DataTableColumnHeader column={header.column} title={label} />
  }

  return flexRender(header.column.columnDef.header, header.getContext())
}
