// metapi-go/data-table — the toolbar's Reset contract.
//
// Reset has to undo two different kinds of state: what the table holds (global
// filter, column filters) and what only the toolbar holds (a search draft typed
// but not yet through the debounce). The second is the easy one to break — the
// table never saw that text, so clearing table state alone leaves it on screen.
// Both are pinned here, together with the visibility rule that decides when
// Reset is offered at all.
import '@testing-library/jest-dom/vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { DataTableToolbar } from '../toolbar/toolbar'

type TableState = {
  globalFilter: string
  columnFilters: { id: string; value: unknown[] }[]
}

function makeTable(state: TableState) {
  return {
    getState: () => state,
    getColumn: () => undefined,
    getAllColumns: () => [],
    setGlobalFilter: vi.fn((value: string) => {
      state.globalFilter = value
    }),
    resetColumnFilters: vi.fn(() => {
      state.columnFilters = []
    }),
  }
}

afterEach(() => {
  cleanup()
  vi.useRealTimers()
})

describe('DataTableToolbar reset', () => {
  it('is hidden while nothing is filtered', () => {
    render(
      <DataTableToolbar
        table={makeTable({ globalFilter: '', columnFilters: [] }) as never}
        searchPlaceholder='Search…'
      />
    )

    expect(screen.queryByRole('button', { name: /Reset/ })).toBeNull()
  })

  it('clears the table filters and calls onReset', () => {
    const table = makeTable({
      globalFilter: 'openai',
      columnFilters: [{ id: 'status', value: ['active'] }],
    })
    const onReset = vi.fn()
    const view = render(
      <DataTableToolbar
        table={table as never}
        searchPlaceholder='Search…'
        onReset={onReset}
      />
    )

    fireEvent.click(screen.getByRole('button', { name: /Reset/ }))

    expect(table.setGlobalFilter).toHaveBeenCalledWith('')
    expect(table.resetColumnFilters).toHaveBeenCalledOnce()
    expect(onReset).toHaveBeenCalledOnce()

    // In production the table state change re-renders the page; the mock table
    // has no subscribers, so the re-render is driven explicitly.
    view.rerender(
      <DataTableToolbar
        table={table as never}
        searchPlaceholder='Search…'
        onReset={onReset}
      />
    )
    expect(screen.getByPlaceholderText('Search…')).toHaveValue('')
  })

  it('drops a search draft the debounce has not committed yet', () => {
    vi.useFakeTimers()
    const table = makeTable({ globalFilter: '', columnFilters: [] })
    render(
      <DataTableToolbar
        table={table as never}
        searchPlaceholder='Search…'
        searchDebounceMs={500}
        // A page-owned filter (proxy logs keeps status + date range in the URL)
        // is what makes Reset reachable while the table itself is clean.
        hasAdditionalFilters
      />
    )

    const input = screen.getByPlaceholderText('Search…')
    fireEvent.change(input, { target: { value: 'openai' } })
    expect(input).toHaveValue('openai')

    fireEvent.click(screen.getByRole('button', { name: /Reset/ }))

    expect(input).toHaveValue('')
    expect(table.setGlobalFilter).not.toHaveBeenCalled()
  })
})
