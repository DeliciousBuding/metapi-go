// Regression test: `DataTablePage`'s `emptyAction` slot must reach the
// mobile card list empty state (audit #1029 batch B). Previously the CTA
// rendered on desktop only — mobile fell back to a plain empty state with
// no action button, stranding empty-state CTAs on 8 entity pages.
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

type ProbeRow = { id: number; name: string }

const probeColumns: ColumnDef<ProbeRow, unknown>[] = [
  { accessorKey: 'name', header: 'Name' },
]

function setMediaQuery(matches: boolean) {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: vi.fn().mockImplementation((query: string) => ({
      matches,
      media: query,
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  })
}

afterEach(() => cleanup())

function renderEmptyPage() {
  function Harness() {
    const table = useReactTable({
      data: [] as ProbeRow[],
      columns: probeColumns,
      getCoreRowModel: getCoreRowModel(),
    })
    return (
      <DataTablePage
        table={table}
        columns={probeColumns}
        emptyTitle='No widgets'
        emptyAction={<button type='button'>Create widget</button>}
        toolbarProps={null}
        showPagination={false}
      />
    )
  }
  return render(<Harness />)
}

describe('DataTablePage emptyAction on mobile', () => {
  it('renders the empty action in the mobile card list', () => {
    setMediaQuery(true)
    renderEmptyPage()

    expect(screen.getByText('No widgets')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Create widget' })
    ).toBeInTheDocument()
  })

  it('renders the empty action on desktop', () => {
    setMediaQuery(false)
    renderEmptyPage()

    expect(screen.getByText('No widgets')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Create widget' })
    ).toBeInTheDocument()
  })
})
