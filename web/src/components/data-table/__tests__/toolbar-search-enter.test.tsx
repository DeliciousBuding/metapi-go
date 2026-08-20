// Behavior test for the toolbar search Enter shortcut (#889): with debounced
// filtering the draft only commits after the delay, but pressing Enter must
// commit immediately so "type → Enter" behaves like an explicit search.

import '@testing-library/jest-dom/vitest'
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { DataTableToolbar } from '../toolbar/toolbar'

function makeTable(
  globalFilter: string,
  setGlobalFilter: (value: string) => void
) {
  return {
    getState: () => ({ columnFilters: [], globalFilter }),
    getColumn: () => undefined,
    getAllColumns: () => [],
    setGlobalFilter,
    resetColumnFilters: vi.fn(),
  }
}

afterEach(() => {
  cleanup()
  vi.useRealTimers()
})

describe('DataTableToolbar search Enter commit', () => {
  it('commits the draft on Enter without waiting out the debounce', () => {
    vi.useFakeTimers()
    const setGlobalFilter = vi.fn()
    render(
      <DataTableToolbar
        table={makeTable('', setGlobalFilter) as never}
        searchPlaceholder='Search…'
        searchDebounceMs={500}
      />
    )

    const input = screen.getByPlaceholderText('Search…')
    fireEvent.change(input, { target: { value: 'openai' } })

    // Debounce has not elapsed — nothing committed yet.
    expect(setGlobalFilter).not.toHaveBeenCalled()

    fireEvent.keyDown(input, { key: 'Enter' })
    expect(setGlobalFilter).toHaveBeenCalledWith('openai')
  })

  it('plain typing still waits for the debounce', () => {
    vi.useFakeTimers()
    const setGlobalFilter = vi.fn()
    render(
      <DataTableToolbar
        table={makeTable('', setGlobalFilter) as never}
        searchPlaceholder='Search…'
        searchDebounceMs={500}
      />
    )

    const input = screen.getByPlaceholderText('Search…')
    fireEvent.change(input, { target: { value: 'gemini' } })
    expect(setGlobalFilter).not.toHaveBeenCalled()

    // Let the debounce elapse; act() flushes the debounce state update and
    // the commit effect that follows it.
    act(() => {
      vi.advanceTimersByTime(600)
    })
    expect(setGlobalFilter).toHaveBeenCalledWith('gemini')
  })
})
