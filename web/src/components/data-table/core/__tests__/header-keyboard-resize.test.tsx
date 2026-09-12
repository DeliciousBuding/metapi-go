// metapi-go/data-table — the header's keyboard resize path.
//
// Column resizing has two inputs and only one of them is pointer-based: the drag
// handler TanStack supplies cannot be reached from a keyboard, so the handle is
// a focusable `role="separator"` that steps the width on arrows (Shift for a
// coarser step) and reports the current width through `aria-valuenow`.
//
// jsdom has no layout engine, so a width cannot be observed as pixels here.
// What is observable — and what the colgroup turns into `<col>` widths on a real
// page — is the column-sizing state the handle writes, which is what these pin.
import '@testing-library/jest-dom/vitest'
import {
  getCoreRowModel,
  useReactTable,
  type ColumnDef,
} from '@tanstack/react-table'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'

import '@/i18n/config'

import { DataTableHeader } from '../data-table-header'

type ProbeRow = { name: string }
type ProbeTable = ReturnType<typeof useReactTable<ProbeRow>>

const columns: ColumnDef<ProbeRow, unknown>[] = [
  { accessorKey: 'name', header: 'Name', size: 200 },
  // Content-sized: no share of the width budget, so no handle to resize it with.
  { id: 'actions', header: 'Actions' },
]

function requireTable(table: ProbeTable | null): ProbeTable {
  if (table === null) throw new Error('table instance was not initialized')
  return table
}

function renderHeader() {
  let tableInstance: ProbeTable | null = null

  function Harness() {
    const table = useReactTable({
      data: [{ name: 'alpha' }],
      columns,
      getCoreRowModel: getCoreRowModel(),
      enableColumnResizing: true,
      columnResizeMode: 'onChange',
    })
    tableInstance = table
    return (
      <table>
        <DataTableHeader table={table} />
      </table>
    )
  }

  render(<Harness />)
  return { table: () => requireTable(tableInstance) }
}

afterEach(() => cleanup())

describe('DataTableHeader keyboard resize', () => {
  it('offers a handle on sized columns only', () => {
    renderHeader()

    const handles = screen.getAllByRole('separator')
    expect(handles).toHaveLength(1)
    expect(handles[0].closest('th')?.getAttribute('data-column-id')).toBe(
      'name'
    )
  })

  it('steps the width by 10px, and by 50px with Shift held', () => {
    const { table } = renderHeader()
    const handle = screen.getByRole('separator')

    fireEvent.keyDown(handle, { key: 'ArrowRight' })
    expect(table().getState().columnSizing).toEqual({ name: 210 })

    fireEvent.keyDown(handle, { key: 'ArrowRight', shiftKey: true })
    expect(table().getState().columnSizing).toEqual({ name: 260 })

    fireEvent.keyDown(handle, { key: 'ArrowLeft' })
    expect(table().getState().columnSizing).toEqual({ name: 250 })
  })

  it('leaves the width alone for keys that do not resize', () => {
    const { table } = renderHeader()

    fireEvent.keyDown(screen.getByRole('separator'), { key: 'ArrowUp' })
    expect(table().getState().columnSizing).toEqual({})
  })

  it('announces the current width, and the new one after a step', () => {
    const { table } = renderHeader()
    const handle = screen.getByRole('separator')

    expect(handle).toHaveAttribute('aria-valuenow', '200')

    fireEvent.keyDown(handle, { key: 'ArrowRight' })
    expect(table().getState().columnSizing).toEqual({ name: 210 })
    expect(screen.getByRole('separator')).toHaveAttribute(
      'aria-valuenow',
      '210'
    )
  })
})
