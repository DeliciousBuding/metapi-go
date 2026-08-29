// Regression test for the table page's background-refetch indicator. While
// `isFetching`, the container may dim slightly but must never carry
// `pointer-events-none` — rows stay rendered (placeholderData) and every
// click must still land (issue #889).
import '@testing-library/jest-dom/vitest'
import {
  getCoreRowModel,
  useReactTable,
  type ColumnDef,
} from '@tanstack/react-table'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { DataTablePage } from '../data-table-page'

type ProbeRow = { id: number; name: string }

const probeRows: ProbeRow[] = [
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

function renderDataTablePage(options: { isFetching?: boolean }) {
  function Harness() {
    const table = useReactTable({
      data: probeRows,
      columns: probeColumns,
      getCoreRowModel: getCoreRowModel(),
    })
    return (
      <DataTablePage
        table={table}
        columns={probeColumns}
        isFetching={options.isFetching}
        toolbarProps={null}
        showPagination={false}
      />
    )
  }
  return render(<Harness />)
}

describe('DataTablePage background refetch indicator', () => {
  it('never blocks pointer events while refetching', () => {
    const { container } = renderDataTablePage({ isFetching: true })

    // Rows stay visible through the refetch.
    expect(screen.getByText('alpha')).toBeInTheDocument()
    expect(screen.getByText('beta')).toBeInTheDocument()

    // No element in the table layout may swallow clicks while refetching.
    expect(container.querySelector('.pointer-events-none')).toBeNull()
    // The subtle dim indicator remains so a refetch is still perceptible.
    expect(container.querySelector('.opacity-80')).not.toBeNull()
  })

  it('renders at full opacity when idle', () => {
    const { container } = renderDataTablePage({ isFetching: false })

    expect(screen.getByText('alpha')).toBeInTheDocument()
    expect(container.querySelector('.pointer-events-none')).toBeNull()
    expect(container.querySelector('.opacity-80')).toBeNull()
  })
})

describe('DataTablePage error contract (S7)', () => {
  function renderWithError(options: {
    placement?: 'replace' | 'inline'
    onRetry?: () => void
  }) {
    function Harness() {
      const table = useReactTable({
        data: probeRows,
        columns: probeColumns,
        getCoreRowModel: getCoreRowModel(),
      })
      return (
        <DataTablePage
          table={table}
          columns={probeColumns}
          error={new Error('boom')}
          errorMessageKey='sites.page.loadError'
          onErrorRetry={options.onRetry}
          errorPlacement={options.placement}
          toolbarProps={null}
          showPagination={false}
        />
      )
    }
    return render(<Harness />)
  }

  it('replace placement swaps the whole table region for the banner', () => {
    renderWithError({})

    expect(screen.getByRole('alert')).toHaveTextContent('boom')
    // Stale rows must not read as current data.
    expect(screen.queryByText('alpha')).not.toBeInTheDocument()
  })

  it('inline placement keeps the rows visible under the banner', () => {
    renderWithError({ placement: 'inline' })

    expect(screen.getByRole('alert')).toHaveTextContent('boom')
    expect(screen.getByText('alpha')).toBeInTheDocument()
  })

  it('banner Retry invokes onErrorRetry', () => {
    const onRetry = vi.fn()
    renderWithError({ onRetry })

    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(onRetry).toHaveBeenCalledTimes(1)
  })
})
