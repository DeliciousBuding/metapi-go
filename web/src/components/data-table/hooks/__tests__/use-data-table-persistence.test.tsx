// metapi-go/data-table — the persisted column preferences.
//
// Column visibility and sizing are the only table state that outlives the page:
// both go to localStorage under a per-page key. That makes them the one place
// where user-editable data flows *into* table state, so the contract is pinned
// here: hydrate from storage, write changes back, survive junk instead of
// throwing on it, and re-read when the storage key changes under a mounted hook
// (the settings pages reuse one table component across sections).
import type { ColumnDef } from '@tanstack/react-table'
import { act, cleanup, render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { useDataTable } from '@/components/data-table'

type ProbeRow = { id: number; name: string }
type ProbeTable = ReturnType<typeof useDataTable<ProbeRow>>['table']
type ProbeOptions = {
  columnVisibilityStorageKey?: string
  columnSizingStorageKey?: string
  initialColumnVisibility?: Record<string, boolean>
}

const VISIBILITY_KEY = 'test.probe.columnVisibility'
const OTHER_VISIBILITY_KEY = 'test.probe.columnVisibility.other'
const SIZING_KEY = 'test.probe.columnSizing'

const rows: ProbeRow[] = [
  { id: 1, name: 'alpha' },
  { id: 2, name: 'beta' },
]

const columns: ColumnDef<ProbeRow, unknown>[] = [
  { accessorKey: 'name', header: 'Name' },
  { id: 'owner', header: 'Owner' },
]

function requireTable(table: ProbeTable | null): ProbeTable {
  if (table === null) throw new Error('table instance was not initialized')
  return table
}

function stored(key: string): unknown {
  const raw = window.localStorage.getItem(key)
  return raw === null ? null : JSON.parse(raw)
}

function renderTable(options: ProbeOptions) {
  let tableInstance: ProbeTable | null = null

  function Harness(props: { options: ProbeOptions }) {
    const { table } = useDataTable<ProbeRow>({
      data: rows,
      columns,
      enableColumnResizing: true,
      ...props.options,
    })
    tableInstance = table
    return <span>harness</span>
  }

  const view = render(<Harness options={options} />)

  return {
    table: () => requireTable(tableInstance),
    rerenderWith: (next: ProbeOptions) =>
      view.rerender(<Harness options={next} />),
  }
}

beforeEach(() => {
  window.localStorage.clear()
})

afterEach(() => {
  cleanup()
  vi.useRealTimers()
})

describe('useDataTable column visibility persistence', () => {
  it('hides the columns storage says are hidden', () => {
    window.localStorage.setItem(
      VISIBILITY_KEY,
      JSON.stringify({ owner: false, name: true })
    )

    const { table } = renderTable({
      columnVisibilityStorageKey: VISIBILITY_KEY,
    })

    expect(table().getColumn('owner')?.getIsVisible()).toBe(false)
    expect(table().getColumn('name')?.getIsVisible()).toBe(true)
  })

  it('writes a visibility change back to storage', () => {
    const { table } = renderTable({
      columnVisibilityStorageKey: VISIBILITY_KEY,
    })

    act(() => {
      table().setColumnVisibility({ owner: false })
    })

    expect(stored(VISIBILITY_KEY)).toEqual({ owner: false })
  })

  it('keeps everything visible when storage holds junk', () => {
    for (const junk of ['{not json', '[1,2]', '"owner"', 'null']) {
      window.localStorage.setItem(VISIBILITY_KEY, junk)

      const { table } = renderTable({
        columnVisibilityStorageKey: VISIBILITY_KEY,
      })

      expect(table().getColumn('owner')?.getIsVisible()).toBe(true)
      cleanup()
    }
  })

  it('drops stored entries that are not booleans', () => {
    window.localStorage.setItem(
      VISIBILITY_KEY,
      JSON.stringify({ owner: 'no', name: false })
    )

    const { table } = renderTable({
      columnVisibilityStorageKey: VISIBILITY_KEY,
    })

    // `name` is a real preference; `owner`'s string is not, and defaults win.
    expect(table().getState().columnVisibility).toEqual({ name: false })
  })

  it('re-reads when the storage key changes under a mounted table', () => {
    window.localStorage.setItem(
      VISIBILITY_KEY,
      JSON.stringify({ owner: false })
    )
    window.localStorage.setItem(
      OTHER_VISIBILITY_KEY,
      JSON.stringify({ name: false })
    )

    const { table, rerenderWith } = renderTable({
      columnVisibilityStorageKey: VISIBILITY_KEY,
    })
    expect(table().getColumn('owner')?.getIsVisible()).toBe(false)

    rerenderWith({ columnVisibilityStorageKey: OTHER_VISIBILITY_KEY })

    expect(table().getState().columnVisibility).toEqual({ name: false })
    // Re-hydrating is not a user change: the old key keeps what it had.
    expect(stored(VISIBILITY_KEY)).toEqual({ owner: false })
  })
})

describe('useDataTable column sizing persistence', () => {
  it('debounces the write, because a resize drag changes it per pointer move', () => {
    vi.useFakeTimers()
    const { table } = renderTable({ columnSizingStorageKey: SIZING_KEY })

    act(() => {
      table().setColumnSizing({ name: 240 })
    })
    expect(window.localStorage.getItem(SIZING_KEY)).toBeNull()

    act(() => {
      vi.advanceTimersByTime(250)
    })
    expect(stored(SIZING_KEY)).toEqual({ name: 240 })
  })

  it('drops stored widths that are not positive numbers', () => {
    window.localStorage.setItem(
      SIZING_KEY,
      JSON.stringify({ name: -5, owner: '180', id: 0, size: 120 })
    )

    const { table } = renderTable({ columnSizingStorageKey: SIZING_KEY })

    expect(table().getState().columnSizing).toEqual({ size: 120 })
  })
})
