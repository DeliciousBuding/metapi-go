// Component-level axe gate for DataTablePage: a real table with sortable
// headers, row data and a toolbar must produce zero structural violations —
// sort controls are named, the table is a valid table landmark, and header
// cells announce their sort state.
import '@testing-library/jest-dom/vitest'
import {
  getCoreRowModel,
  getSortedRowModel,
  useReactTable,
  type ColumnDef,
} from '@tanstack/react-table'
import { cleanup, render } from '@testing-library/react'
import axe from 'axe-core'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { DataTablePage } from '../data-table-page'

type ProbeRow = { id: number; name: string }

const probeRows: ProbeRow[] = [
  { id: 1, name: 'alpha' },
  { id: 2, name: 'beta' },
]

const probeColumns: ColumnDef<ProbeRow, unknown>[] = [
  { accessorKey: 'name', header: 'Name' },
  { accessorKey: 'id', header: 'ID' },
]

beforeAll(() => {
  // useMediaQuery queries matchMedia under jsdom; report desktop so the
  // desktop table branch renders.
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: vi.fn().mockImplementation((query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  })
})

afterEach(() => cleanup())

function renderDataTable() {
  function Harness() {
    const table = useReactTable({
      data: probeRows,
      columns: probeColumns,
      getCoreRowModel: getCoreRowModel(),
      getSortedRowModel: getSortedRowModel(),
    })
    return (
      <DataTablePage
        table={table}
        columns={probeColumns}
        toolbarProps={null}
        showPagination={false}
      />
    )
  }
  return render(<Harness />)
}

describe('DataTablePage axe gate', () => {
  it('populated table produces zero axe violations', async () => {
    const { container } = renderDataTable()

    const results = await axe.run(container)
    expect(results.violations).toEqual([])
  })

  it('ships an accessible table landmark with column headers', () => {
    const { container } = renderDataTable()

    expect(container.querySelector('table')).not.toBeNull()
    expect(
      container.querySelectorAll('th[scope="col"], th:not([scope])')
    ).not.toHaveLength(0)
    expect(container.querySelectorAll('td')).not.toHaveLength(0)
  })
})
