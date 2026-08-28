// DataTableHeader aria-sort contract: a sortable column's th announces its
// sort state (none/ascending/descending) so AT users hear direction changes
// from the header itself; non-sortable columns omit aria-sort entirely.
import '@testing-library/jest-dom/vitest'
import {
  createColumnHelper,
  getCoreRowModel,
  getSortedRowModel,
  useReactTable,
  type SortingState,
} from '@tanstack/react-table'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { useState } from 'react'
import { afterEach, describe, expect, it } from 'vitest'

import '@/i18n/config'

import { DataTableHeader } from '../data-table-header'

type ProbeRow = { id: number; name: string }

const probeRows: ProbeRow[] = [
  { id: 1, name: 'alpha' },
  { id: 2, name: 'beta' },
]

function renderHeader(initialSorting: SortingState = []) {
  function Harness() {
    const [sorting, setSorting] = useState<SortingState>(initialSorting)
    const columnHelper = createColumnHelper<ProbeRow>()
    const columns = [
      columnHelper.accessor('name', { header: 'Name' }),
      // enableSorting: false — aria-sort must never appear here.
      columnHelper.accessor('id', { header: 'ID', enableSorting: false }),
    ]
    const table = useReactTable({
      data: probeRows,
      columns,
      state: { sorting },
      onSortingChange: setSorting,
      getCoreRowModel: getCoreRowModel(),
      getSortedRowModel: getSortedRowModel(),
    })
    return (
      <table>
        <DataTableHeader table={table} />
      </table>
    )
  }
  return render(<Harness />)
}

function getHeaderCell(columnId: string): HTMLElement {
  const cell = document.querySelector<HTMLElement>(
    `th[data-column-id="${columnId}"]`
  )
  if (!cell) throw new Error(`th[data-column-id="${columnId}"] not found`)
  return cell
}

afterEach(() => cleanup())

describe('DataTableHeader aria-sort', () => {
  it('marks an unsorted sortable column as aria-sort="none"', () => {
    renderHeader()

    expect(getHeaderCell('name')).toHaveAttribute('aria-sort', 'none')
  })

  it('marks an ascending column as aria-sort="ascending"', () => {
    renderHeader([{ id: 'name', desc: false }])

    expect(getHeaderCell('name')).toHaveAttribute('aria-sort', 'ascending')
  })

  it('marks a descending column as aria-sort="descending"', () => {
    renderHeader([{ id: 'name', desc: true }])

    expect(getHeaderCell('name')).toHaveAttribute('aria-sort', 'descending')
  })

  it('omits aria-sort on non-sortable columns', () => {
    renderHeader()

    expect(getHeaderCell('id')).not.toHaveAttribute('aria-sort')
  })

  it('updates aria-sort when the user toggles sorting from the header', async () => {
    renderHeader()

    expect(getHeaderCell('name')).toHaveAttribute('aria-sort', 'none')

    fireEvent.click(screen.getByRole('button', { name: /Name/ }))
    fireEvent.click(await screen.findByRole('menuitem', { name: /Asc/ }))

    expect(getHeaderCell('name')).toHaveAttribute('aria-sort', 'ascending')
  })
})
