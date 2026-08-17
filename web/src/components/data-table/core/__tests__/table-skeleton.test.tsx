// Behavior tests for the skeleton subsystem. Two co-located concerns:
//   (a) the base `<Skeleton>` uses the `.animate-shimmer` utility (the
//       dormant --skeleton-highlight token is now wired via the gradient
//       in index.css), replacing the legacy `animate-pulse` opacity flicker;
//   (b) `<TableSkeleton>` derives each bar's width from `column.getSize()`
//       (clamped to [32, 140] px) so the skeleton mirrors real column
//       proportions instead of cycling through a fixed percentage pool.
// Asserts only user-visible DOM (class list, inline style.width) — never
// internal component state.

import '@testing-library/jest-dom/vitest'
import { cleanup, render } from '@testing-library/react'
import type { Table } from '@tanstack/react-table'
import { afterEach, describe, expect, it } from 'vitest'

import { Skeleton } from '@/components/ui/skeleton'
import { TableSkeleton } from '../table-skeleton'

/**
 * Minimal stub of a TanStack `Table<TData>`. `<TableSkeleton>` only reads
 * `table.getVisibleLeafColumns()` (each column exposes `{ id, getSize() }`)
 * and `table.getState().pagination.pageSize`. Building a real `Table` via
 * `useReactTable` would couple these assertions to TanStack's column-sizing
 * defaults (which differ across `columnSizingMode`/`enableColumnResizing`),
 * so a hand-rolled stub keeps the test pointed at the contract the
 * component actually depends on.
 */
interface StubColumn {
  id: string
  size: number
}

interface StubTable {
  columns: StubColumn[]
  pageSize: number
}

function buildStubTable(columns: StubColumn[], pageSize = 10): StubTable {
  return { columns, pageSize }
}

// The stub only implements the slice of `Table<TData>` that `TableSkeleton`
// consumes; the rest of the TanStack interface is unused here, so cast
// through `unknown` rather than fleshing out ~40 unused methods.
function asTable(stub: StubTable): Table<unknown> {
  return {
    getVisibleLeafColumns: () =>
      stub.columns.map((column) => ({
        id: column.id,
        getSize: () => column.size,
      })),
    getState: () => ({ pagination: { pageSize: stub.pageSize } }),
  } as unknown as Table<unknown>
}

afterEach(() => cleanup())

describe('Skeleton (base)', () => {
  it('applies the shimmer utility (wires the dormant --skeleton-highlight token)', () => {
    const { container } = render(<Skeleton />)

    const skeleton = container.querySelector('[data-slot="skeleton"]')
    expect(skeleton).not.toBeNull()
    expect(skeleton).toHaveClass('animate-shimmer')
    expect(skeleton).not.toHaveClass('animate-pulse')
  })

  it('keeps rounded-md and composes with caller-supplied className', () => {
    const { container } = render(<Skeleton className='h-4 w-48' />)

    const skeleton = container.querySelector('[data-slot="skeleton"]')
    expect(skeleton).not.toBeNull()
    expect(skeleton).toHaveClass('rounded-md')
    expect(skeleton).toHaveClass('h-4')
    expect(skeleton).toHaveClass('w-48')
  })
})

describe('TableSkeleton', () => {
  it('derives each bar width from column.getSize() (clamped to [32, 140] px)', () => {
    // 200 → 200 * 0.6 = 120 (in range)
    // 100 → 100 * 0.6 = 60  (in range)
    // 50  → 50  * 0.6 = 30  → clamped UP to 32
    // 300 → 300 * 0.6 = 180 → clamped DOWN to 140
    const stub = buildStubTable([
      { id: 'name', size: 200 },
      { id: 'age', size: 100 },
      { id: 'tiny', size: 50 },
      { id: 'huge', size: 300 },
    ])

    // rowCount={1} keeps the assertion focused on width derivation —
    // one row × four columns yields exactly four bars to assert against,
    // without the row-count multiplier (pageSize default 10) inflating
    // the bar count and masking which column produced which width.
    const { container } = render(
      <TableSkeleton table={asTable(stub)} rowCount={1} />
    )

    const bars = container.querySelectorAll('[data-slot="skeleton"]')
    expect(bars).toHaveLength(4) // one row × four columns
    expect(bars[0]).toHaveStyle({ width: '120px' })
    expect(bars[1]).toHaveStyle({ width: '60px' })
    expect(bars[2]).toHaveStyle({ width: '32px' }) // clamped up
    expect(bars[3]).toHaveStyle({ width: '140px' }) // clamped down
  })

  it('preserves the select-column special-case (small square, no inline width)', () => {
    const stub = buildStubTable([
      { id: 'select', size: 40 },
      { id: 'name', size: 200 },
    ])

    const { container } = render(
      <TableSkeleton table={asTable(stub)} rowCount={1} />
    )

    const bars = container.querySelectorAll('[data-slot="skeleton"]')
    expect(bars).toHaveLength(2)

    const selectBar = bars[0]
    expect(selectBar).toHaveClass('size-4')
    // select column passes `style={undefined}`, so React omits the style
    // attribute entirely — no `width` should appear anywhere on the element.
    expect(selectBar.getAttribute('style') ?? '').not.toContain('width')

    // Non-select column still gets a width derived from its size.
    expect(bars[1]).toHaveStyle({ width: '120px' })
  })

  it('honors the rowCount prop (renders exactly N rows)', () => {
    const stub = buildStubTable([{ id: 'name', size: 100 }])

    const { container } = render(
      <TableSkeleton table={asTable(stub)} rowCount={3} />
    )

    const rows = container.querySelectorAll('tr')
    expect(rows).toHaveLength(3)
  })
})
