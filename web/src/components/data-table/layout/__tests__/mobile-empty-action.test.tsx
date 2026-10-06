// Regression test: `DataTablePage`'s `emptyAction` slot must reach the
// mobile card list empty state (audit #1029 batch B). Previously the CTA
// rendered on desktop only — mobile fell back to a plain empty state with
// no action button, stranding empty-state CTAs on 8 entity pages.
import '@testing-library/jest-dom/vitest'
import {
  getCoreRowModel,
  getPaginationRowModel,
  useReactTable,
  type ColumnDef,
  type RowSelectionState,
} from '@tanstack/react-table'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { useState } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'
import { Checkbox } from '@/components/ui/checkbox'

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
      getPaginationRowModel: getPaginationRowModel(),
    })
    return (
      <DataTablePage
        table={table}
        emptyTitle='No widgets'
        emptyAction={<button type='button'>Create widget</button>}
        toolbarProps={null}
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

describe('external-filter empty state', () => {
  it.each([true, false])('clears page-owned filters on mobile=%s', (mobile) => {
    setMediaQuery(mobile)
    function Harness() {
      const [filtered, setFiltered] = useState(true)
      const table = useReactTable({
        data: filtered ? [] : [{ id: 1, name: 'Restored result' }],
        columns: probeColumns,
        getCoreRowModel: getCoreRowModel(),
        getPaginationRowModel: getPaginationRowModel(),
      })
      return (
        <DataTablePage
          table={table}
          emptyTitle='No widgets'
          emptyAction={<button type='button'>Create widget</button>}
          toolbarProps={{
            hasAdditionalFilters: filtered,
            onReset: () => setFiltered(false),
          }}
        />
      )
    }
    render(<Harness />)
    expect(screen.queryByText('No widgets')).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Create widget' })
    ).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Reset filters' }))
    expect(screen.getByText('Restored result')).toBeInTheDocument()
  })
})

describe('mobile card selection', () => {
  it('selects only from the checkbox, not a tap on the card surface', async () => {
    setMediaQuery(true)

    function Harness() {
      const [rowSelection, setRowSelection] = useState<RowSelectionState>({})
      const table = useReactTable({
        data: [{ id: 1, name: 'alpha' }],
        columns: [
          {
            id: 'select',
            cell: ({ row }) => (
              <Checkbox
                checked={row.getIsSelected()}
                onCheckedChange={(checked) =>
                  row.toggleSelected(Boolean(checked))
                }
                aria-label='Select alpha'
              />
            ),
          },
          ...probeColumns,
        ] as ColumnDef<ProbeRow, unknown>[],
        enableRowSelection: true,
        state: { rowSelection },
        onRowSelectionChange: setRowSelection,
        getCoreRowModel: getCoreRowModel(),
        getPaginationRowModel: getPaginationRowModel(),
      })
      return <DataTablePage table={table} toolbarProps={null} />
    }

    const { container } = render(<Harness />)
    const card = screen.getByText('alpha').closest('[data-mobile-card-row]')
    const checkbox = screen.getByRole('checkbox', { name: 'Select alpha' })
    if (!card) throw new Error('mobile card did not render')
    fireEvent.click(card)
    expect(checkbox).toHaveAttribute('aria-checked', 'false')
    fireEvent.click(checkbox)
    await waitFor(() =>
      expect(
        screen.getByRole('checkbox', { name: 'Select alpha' })
      ).toHaveAttribute('aria-checked', 'true')
    )
    expect(container.querySelector('[data-state=selected]')).not.toBeNull()
  })
})
