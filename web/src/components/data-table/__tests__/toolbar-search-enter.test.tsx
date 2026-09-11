// metapi-go/data-table — when the toolbar's search draft reaches the table.
//
// Two timing rules, both invisible in the DOM:
//
// - Enter commits immediately (#889). With debounced filtering the draft would
//   otherwise sit for the whole delay after the user has said "search now".
// - An IME composition never commits mid-flight. Half-typed pinyin is not a
//   filter, so the draft is held until compositionend and only then debounced.

import '@testing-library/jest-dom/vitest'
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
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

describe('DataTableToolbar search composition', () => {
  it('holds the draft while the IME is composing, then commits it', () => {
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
    fireEvent.compositionStart(input)
    fireEvent.change(input, { target: { value: 'ni' } })
    expect(input).toHaveValue('ni')

    // The debounce elapses mid-composition and must still not commit.
    act(() => {
      vi.advanceTimersByTime(600)
    })
    expect(setGlobalFilter).not.toHaveBeenCalled()

    fireEvent.compositionEnd(input, { target: { value: '你好' } })
    act(() => {
      vi.advanceTimersByTime(600)
    })
    expect(setGlobalFilter).toHaveBeenCalledWith('你好')
  })
})
