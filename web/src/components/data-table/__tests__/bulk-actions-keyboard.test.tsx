// metapi-go/data-table — the bulk-actions toolbar's keyboard and Escape contract.
//
// The bar is a `role="toolbar"`, which is a promise to the keyboard: one Tab stop
// in, then arrows to move, Home/End to jump, and Escape to leave (here, by
// clearing the selection). None of that is observable in the DOM, so it is pinned
// here — including the one case that is easy to get wrong: an Escape pressed
// inside an open dropdown belongs to the dropdown, and must not also throw away
// the user's selection.
import '@testing-library/jest-dom/vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { DataTableBulkActions } from '../toolbar/bulk-actions'

function makeTable(selectedCount: number) {
  return {
    getFilteredSelectedRowModel: () => ({
      rows: Array.from({ length: selectedCount }, () => ({})),
    }),
    resetRowSelection: vi.fn(),
  }
}

/** Renders the bar with three actions; returns the table stub. */
function renderToolbar(children?: React.ReactNode) {
  const table = makeTable(2)
  render(
    <DataTableBulkActions table={table as never} entityName='Site'>
      {children ?? (
        <>
          <button type='button'>Action 1</button>
          <button type='button'>Action 2</button>
          <button type='button'>Action 3</button>
        </>
      )}
    </DataTableBulkActions>
  )
  return table
}

afterEach(() => cleanup())

describe('DataTableBulkActions roving focus', () => {
  it('moves focus with the arrows and wraps at both ends', () => {
    renderToolbar()
    const toolbar = screen.getByRole('toolbar')
    // [0] is the clear-selection button the bar renders itself.
    const buttons = screen.getAllByRole('button')

    toolbar.focus()
    fireEvent.keyDown(toolbar, { key: 'ArrowRight' })
    expect(buttons[0]).toHaveFocus()

    fireEvent.keyDown(toolbar, { key: 'ArrowRight' })
    expect(buttons[1]).toHaveFocus()

    fireEvent.keyDown(toolbar, { key: 'ArrowLeft' })
    expect(buttons[0]).toHaveFocus()

    // Wraps to the last rather than leaving the toolbar.
    fireEvent.keyDown(toolbar, { key: 'ArrowLeft' })
    expect(buttons.at(-1)).toHaveFocus()

    fireEvent.keyDown(toolbar, { key: 'Home' })
    expect(buttons[0]).toHaveFocus()

    fireEvent.keyDown(toolbar, { key: 'End' })
    expect(buttons.at(-1)).toHaveFocus()
  })
})

describe('DataTableBulkActions Escape', () => {
  it('clears the selection when pressed on the toolbar', () => {
    const table = renderToolbar()
    const toolbar = screen.getByRole('toolbar')

    toolbar.focus()
    fireEvent.keyDown(toolbar, { key: 'Escape' })

    expect(table.resetRowSelection).toHaveBeenCalledTimes(1)
  })

  it('leaves the selection alone when the keystroke belongs to an open dropdown', () => {
    const table = renderToolbar(
      <div data-slot='dropdown-menu-content'>
        <button type='button'>Delete</button>
      </div>
    )
    const toolbar = screen.getByRole('toolbar')
    const menuItem = screen.getByRole('button', { name: 'Delete' })

    menuItem.focus()
    fireEvent.keyDown(toolbar, { key: 'Escape' })

    expect(table.resetRowSelection).not.toHaveBeenCalled()
  })
})
