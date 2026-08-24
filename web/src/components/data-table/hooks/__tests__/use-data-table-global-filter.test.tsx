// Regression test: `useDataTable` must apply the global filter by default.
//
// The table passed `globalFilterFn: options.globalFilterFn` straight through,
// so a consumer that supplied `globalFilter` but no custom `globalFilterFn`
// (every list page except token-routes) silently disabled TanStack's global
// filter — the search input updated the URL but the row model never filtered.
// That is the models-page "search returns all rows" bug. The default must
// resolve to TanStack's 'auto' mode (the framework default) so a plain
// `globalFilter` filters string columns.
import '@testing-library/jest-dom/vitest'
import type { ColumnDef } from '@tanstack/react-table'
import { cleanup, render } from '@testing-library/react'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'
import { useDataTable } from '@/components/data-table'

type ProbeRow = { id: number; name: string }
type ProbeTable = ReturnType<typeof useDataTable<ProbeRow>>['table']

function requireTable(table: ProbeTable | null): ProbeTable {
  if (table === null) throw new Error('table instance was not initialized')
  return table
}

const rows: ProbeRow[] = [
  { id: 1, name: 'openai-mock-model-000' },
  { id: 2, name: 'openai-mock-model-005' },
  { id: 3, name: 'alpha' },
]

const columns: ColumnDef<ProbeRow, unknown>[] = [
  { accessorKey: 'name', header: 'Name' },
]

beforeAll(() => {
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

describe('useDataTable global filter default', () => {
  it('filters rows when globalFilter is supplied without a custom globalFilterFn', () => {
    let tableInstance: ProbeTable | null = null

    function Harness() {
      const { table } = useDataTable<ProbeRow>({
        data: rows,
        columns,
        globalFilter: 'mock-model-005',
        getRowId: (row) => String(row.id),
      })
      tableInstance = table
      return <span>harness</span>
    }

    render(<Harness />)

    const names = requireTable(tableInstance)
      .getFilteredRowModel()
      .rows.map((row) => row.original.name)
    expect(names).toEqual(['openai-mock-model-005'])
  })

  it('still respects a consumer-provided globalFilterFn', () => {
    let tableInstance: ProbeTable | null = null

    function Harness() {
      const { table } = useDataTable<ProbeRow>({
        data: rows,
        columns,
        globalFilter: 'mock-model-005',
        globalFilterFn: 'includesString',
        getRowId: (row) => String(row.id),
      })
      tableInstance = table
      return <span>harness</span>
    }

    render(<Harness />)

    const names = requireTable(tableInstance)
      .getFilteredRowModel()
      .rows.map((row) => row.original.name)
    expect(names).toEqual(['openai-mock-model-005'])
  })
})
