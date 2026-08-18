// Regression tests for the routes list page. Covers the two P2 polish
// behaviors: the error state renders a Retry button that calls refetch
// (mirroring the sites-page error pattern), and the first-run empty state
// surfaces an "Add route" CTA (so the user does not have to scan up to the
// header to find the primary action).
import '@testing-library/jest-dom/vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { RoutesPage } from '../routes-page'

const testState = vi.hoisted(() => ({
  routesQuery: {
    data: [] as unknown[],
    error: null as Error | null,
    isLoading: false,
    isFetching: false,
    refetch: vi.fn().mockResolvedValue({}),
  },
  // Sentinel so the error-state test can assert the table page (and its
  // empty-state CTA slot) is NOT rendered while the query is in an error
  // state.
  dataTableRendered: false,
  // Captured emptyAction node lets the empty-state test assert the CTA
  // content without mounting the full data-table stack.
  capturedEmptyAction: null as ReactNode,
  formOpenCount: 0,
  previousFormOpen: undefined as boolean | undefined,
}))

vi.mock('@/components/data-table', () => ({
  DataTableBulkActions: () => null,
  DataTablePage: (props: { emptyAction?: ReactNode }) => {
    testState.dataTableRendered = true
    testState.capturedEmptyAction = props.emptyAction ?? null
    return null
  },
  useUrlTableState: () => ({
    globalFilter: '',
    onGlobalFilterChange: vi.fn(),
    columnFilters: [],
    onColumnFiltersChange: vi.fn(),
    pagination: { pageIndex: 0, pageSize: 20 },
    onPaginationChange: vi.fn(),
    sorting: [],
    onSortingChange: vi.fn(),
    filters: { enabled: '', accountId: '', siteId: '', routeId: '' },
  }),
  useDataTable: () => ({
    table: {
      getFilteredSelectedRowModel: () => ({ rows: [] }),
      resetRowSelection: vi.fn(),
    },
  }),
}))

vi.mock('@tanstack/react-router', () => ({
  useSearch: () => ({}),
  useNavigate: () => vi.fn(),
}))

vi.mock('../../api', () => ({
  useRoutes: () => testState.routesQuery,
  useModelTokenCandidates: () => ({ data: undefined }),
  useDeleteRoute: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useUpdateRoute: () => ({ mutate: vi.fn(), isPending: false }),
  useClearRouteCooldown: () => ({ mutate: vi.fn(), isPending: false }),
  useRebuildRoutes: () => ({ mutate: vi.fn(), isPending: false }),
  useRefreshRouteDecisions: () => ({ mutate: vi.fn(), isPending: false }),
  useBatchUpdateRoutes: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useZeroChannelRoutes: (routes: unknown[]) => routes ?? [],
}))

vi.mock('../routes-columns', () => ({
  useRoutesColumns: () => [],
}))

vi.mock('../route-form-dialog', () => ({
  RouteFormDialog: (props: { open: boolean }) => {
    if (props.open && testState.previousFormOpen !== true) {
      testState.formOpenCount += 1
    }
    testState.previousFormOpen = props.open
    return null
  },
}))

vi.mock('../route-detail-sheet', () => ({
  RouteDetailSheet: () => null,
}))

afterEach(() => cleanup())

beforeEach(() => {
  testState.routesQuery.refetch.mockReset()
  testState.routesQuery.refetch.mockResolvedValue({})
  testState.routesQuery.data = []
  testState.routesQuery.error = null
  testState.routesQuery.isLoading = false
  testState.routesQuery.isFetching = false
  testState.dataTableRendered = false
  testState.capturedEmptyAction = null
  testState.formOpenCount = 0
  testState.previousFormOpen = undefined
})

describe('RoutesPage error state', () => {
  it('renders the error banner and Retry button, not the table/empty CTA', () => {
    testState.routesQuery.error = new Error('boom')
    render(<RoutesPage />)

    expect(screen.getByText(/Failed to load routes: boom/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Retry' })).toBeInTheDocument()

    // The table page (which owns the empty-state "Add route" CTA) must be
    // suppressed while the query is in an error state, and the captured CTA
    // slot must stay empty.
    expect(testState.dataTableRendered).toBe(false)
    expect(testState.capturedEmptyAction).toBeNull()
  })

  it('renders the table page (not the error banner) when there is no error', () => {
    render(<RoutesPage />)

    expect(testState.dataTableRendered).toBe(true)
    expect(screen.queryByText(/Failed to load routes/)).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Retry' })
    ).not.toBeInTheDocument()
  })

  it('calls refetch when the Retry button is clicked', () => {
    testState.routesQuery.error = new Error('boom')
    render(<RoutesPage />)

    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))

    expect(testState.routesQuery.refetch).toHaveBeenCalledTimes(1)
  })
})

describe('RoutesPage empty-state CTA', () => {
  it('wires an "Add route" primary CTA into the empty-state slot', () => {
    render(<RoutesPage />)

    expect(testState.dataTableRendered).toBe(true)
    const captured = testState.capturedEmptyAction
    expect(captured).not.toBeNull()
    // Render the captured emptyAction node in isolation so we can assert the
    // user-visible CTA buttons without mounting the full data-table stack.
    // Scope queries to this render's container so the page header's own
    // "Add route" / "Auto-rebuild" buttons don't collide with the CTA ones.
    const { container } = render(<div>{captured}</div>)
    expect(
      within(container).getByRole('button', { name: /Add route/ })
    ).toBeInTheDocument()
    expect(
      within(container).getByRole('button', { name: /Auto-rebuild/ })
    ).toBeInTheDocument()
  })
})
