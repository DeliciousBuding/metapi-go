// Regression test for the useDataTable auto-reset render loop.
//
// The mobile settings keys-page freeze (375px viewport, trusted pointer click
// on the sidebar hamburger) traced back to an unbounded microtask loop inside
// the data-table state layer: TanStack's `getFilteredRowModel` memo
// recomputed on every render (the table state carried a fresh
// `options.columnFilters ?? []` reference each pass), each recompute queued
// `resetPageIndex()` in a microtask, and the reset always produced a new
// `{ ...old, pageIndex }` pagination object — re-rendering the table and
// re-queuing the reset. With the sync sheet-open flush interleaving, the
// main thread pegged. Two invariants now break the cycle:
//
//   1. a module-level `EMPTY_COLUMN_FILTERS` reference keeps the filtered
//      row model memo stable when the table has no filters;
//   2. `useControllableTableState` returns the previous reference for
//      shallow-equal no-op updates (e.g. a page-index reset to the current
//      value), so React's eager state bail-out skips the re-render.
//
// The full browser loop does not reproduce under jsdom (the test renderer
// flushes the reset's re-render synchronously inside the reset microtask, so
// TanStack's `queued` flag dedupes the next reset), but both link
// invariants are environment-independent and directly observable through
// the table state references below.
import '@testing-library/jest-dom/vitest'
import type { ColumnDef } from '@tanstack/react-table'
import { act, cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'
import { DataTablePage, useDataTable } from '@/components/data-table'

type ProbeRow = { id: number; name: string }
type ProbeTable = ReturnType<typeof useDataTable<ProbeRow>>['table']

function requireTable(table: ProbeTable | null): ProbeTable {
  if (table === null) {
    throw new Error('table instance was not initialized')
  }
  return table
}

const rowsA: ProbeRow[] = [
  { id: 1, name: 'alpha' },
  { id: 2, name: 'beta' },
]

const probeColumns: ColumnDef<ProbeRow, unknown>[] = [
  { accessorKey: 'name', header: 'Name' },
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

describe('useDataTable auto-reset state stability', () => {
  it('keeps the columnFilters reference stable across renders', () => {
    let tableInstance: ProbeTable | null = null
    let renderCount = 0

    function Harness() {
      renderCount += 1
      const { table } = useDataTable<ProbeRow>({
        data: rowsA,
        columns: probeColumns,
        getRowId: (row) => String(row.id),
      })
      tableInstance = table
      return (
        <DataTablePage
          table={table}
          columns={probeColumns}
          toolbarProps={null}
        />
      )
    }

    const view = render(<Harness />)
    const filtersAfterFirstRender =
      requireTable(tableInstance).getState().columnFilters

    view.rerender(<Harness />)
    const filtersAfterRerender =
      requireTable(tableInstance).getState().columnFilters

    // A fresh `?? []` per render was the loop's fuel: the filtered row model
    // memo recomputed on every render and re-queued the page-index reset.
    expect(filtersAfterRerender).toBe(filtersAfterFirstRender)
    expect(renderCount).toBeGreaterThanOrEqual(2)
  })

  it('keeps the pagination reference when a reset is a no-op', async () => {
    let tableInstance: ProbeTable | null = null

    function Harness() {
      const { table } = useDataTable<ProbeRow>({
        data: rowsA,
        columns: probeColumns,
        getRowId: (row) => String(row.id),
      })
      tableInstance = table
      return (
        <DataTablePage
          table={table}
          columns={probeColumns}
          toolbarProps={null}
        />
      )
    }

    render(<Harness />)
    expect(screen.getByText('alpha')).toBeInTheDocument()

    const paginationBefore = requireTable(tableInstance).getState().pagination

    // Same sequence the autoResetPageIndex microtask runs: reset the page
    // index to its current value (TanStack always emits a fresh
    // `{ ...old, pageIndex }` object even when nothing changes).
    await act(async () => {
      requireTable(tableInstance).setPageIndex(0)
      await new Promise((resolve) => setTimeout(resolve, 20))
    })

    // The shallow-equal bail-out must preserve the previous reference so
    // React's eager state bail-out skips the re-render entirely.
    expect(requireTable(tableInstance).getState().pagination).toBe(
      paginationBefore
    )
  })
})
