import '@testing-library/jest-dom/vitest'
import {
  getCoreRowModel,
  useReactTable,
  type ColumnDef,
} from '@tanstack/react-table'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { DataTablePage } from '../data-table-page'

type Row = { name: string }
const columns: ColumnDef<Row, unknown>[] = [
  { accessorKey: 'name', header: 'Name' },
]

afterEach(cleanup)

function renderPage(count: number) {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: vi.fn().mockImplementation((query: string) => ({
      matches: false,
      media: query,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    })),
  })

  function Harness() {
    const table = useReactTable({
      data: Array.from({ length: count }, (_, index) => ({
        name: `Model ${index + 1}`,
      })),
      columns,
      getCoreRowModel: getCoreRowModel(),
    })
    return <DataTablePage table={table} toolbarProps={null} />
  }

  return render(<Harness />)
}

describe('desktop table height', () => {
  it('lets a sparse list end with its rows', () => {
    renderPage(3)
    expect(screen.getAllByRole('row')).toHaveLength(4)
    expect(screen.getByRole('columnheader').closest('thead')).not.toHaveClass(
      'sticky'
    )
    expect(screen.getByRole('table').parentElement).not.toHaveClass('flex-1')
  })

  it('keeps the sticky, scrollable header for longer lists', () => {
    renderPage(8)
    expect(screen.getAllByRole('row')).toHaveLength(9)
    expect(screen.getByRole('columnheader').closest('thead')).toHaveClass(
      'sticky'
    )
    expect(screen.getByRole('table').parentElement).toHaveClass('overflow-auto')
  })
})
