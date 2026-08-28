// Regression tests for destructive-adjacent route actions: the header
// "Auto-rebuild" action and the bulk "Disable" action both fired immediately;
// they must now open a confirmation dialog first (audit #1029 batch B).
import '@testing-library/jest-dom/vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { RoutesPage } from '../routes-page'

const testState = vi.hoisted(() => ({
  rebuildMutate: vi.fn(),
  batchMutateAsync: vi.fn(),
}))

vi.mock('@/components/data-table', () => ({
  // Render the bulk action children so the Disable button is clickable.
  DataTableBulkActions: (props: { children?: ReactNode }) => (
    <div>{props.children}</div>
  ),
  DataTablePage: (props: { bulkActions?: ReactNode }) => (
    <div>{props.bulkActions}</div>
  ),
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
      // One selected route with a real id so the bulk action proceeds.
      getFilteredSelectedRowModel: () => ({
        rows: [{ original: { id: 7 } }],
      }),
      resetRowSelection: vi.fn(),
    },
  }),
}))

vi.mock('@tanstack/react-router', () => ({
  useSearch: () => ({}),
  useNavigate: () => vi.fn(),
}))

vi.mock('@/features/accounts/api', () => ({
  useAccounts: () => ({ data: undefined }),
}))
vi.mock('@/features/sites/api', () => ({
  useSites: () => ({ data: undefined }),
}))
vi.mock('@/features/channels/api', () => ({
  useChannels: () => ({ data: [] }),
}))

vi.mock('../../api', () => ({
  useRoutes: () => ({ data: [], error: null, isLoading: false, isFetching: false }),
  useModelTokenCandidates: () => ({ data: undefined }),
  useDeleteRoute: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useUpdateRoute: () => ({ mutate: vi.fn(), isPending: false }),
  useClearRouteCooldown: () => ({ mutate: vi.fn(), isPending: false }),
  useRebuildRoutes: () => ({ mutate: testState.rebuildMutate, isPending: false }),
  useRefreshRouteDecisions: () => ({ mutate: vi.fn(), isPending: false }),
  useBatchUpdateRoutes: () => ({
    mutateAsync: testState.batchMutateAsync,
    isPending: false,
  }),
  useZeroChannelRoutes: (routes: unknown[]) => routes ?? [],
}))

vi.mock('../routes-columns', () => ({
  useRoutesColumns: () => [],
}))

vi.mock('../route-form-dialog', () => ({
  RouteFormDialog: () => null,
}))

vi.mock('../route-detail-sheet', () => ({
  RouteDetailSheet: () => null,
}))

beforeAll(() => {
  // base-ui AlertDialog queries matchMedia under jsdom.
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

beforeEach(() => {
  testState.rebuildMutate.mockReset()
  testState.batchMutateAsync.mockReset()
})

afterEach(() => cleanup())

// The dialog action shares its label with the trigger; the action button is
// the last matching button in the tree once the dialog is open.
function clickLastButtonNamed(name: string) {
  const buttons = screen.getAllByRole('button', { name })
  const button = buttons.at(-1)
  if (!button) {
    throw new Error(`Button "${name}" not found`)
  }
  fireEvent.click(button)
}

describe('RoutesPage rebuild confirmation', () => {
  it('opens a confirmation before rebuilding routes', () => {
    render(<RoutesPage />)

    clickLastButtonNamed('Auto-rebuild')

    expect(screen.getByText('Rebuild routes?')).toBeInTheDocument()
    expect(testState.rebuildMutate).not.toHaveBeenCalled()
  })

  it('rebuilds after confirming', async () => {
    render(<RoutesPage />)

    clickLastButtonNamed('Auto-rebuild')
    clickLastButtonNamed('Auto-rebuild')

    await waitFor(() => {
      expect(testState.rebuildMutate).toHaveBeenCalledTimes(1)
    })
    expect(testState.rebuildMutate).toHaveBeenCalledWith({ refreshModels: true })
  })

  it('does not rebuild when the confirmation is cancelled', () => {
    render(<RoutesPage />)

    clickLastButtonNamed('Auto-rebuild')
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))

    expect(testState.rebuildMutate).not.toHaveBeenCalled()
  })
})

describe('RoutesPage bulk disable confirmation', () => {
  it('opens a confirmation before bulk disabling', () => {
    render(<RoutesPage />)

    fireEvent.click(screen.getByRole('button', { name: 'Disable' }))

    expect(screen.getByText('Disable selected routes?')).toBeInTheDocument()
    expect(testState.batchMutateAsync).not.toHaveBeenCalled()
  })

  it('bulk disables after confirming', async () => {
    render(<RoutesPage />)

    fireEvent.click(screen.getByRole('button', { name: 'Disable' }))
    clickLastButtonNamed('Disable')

    await waitFor(() => {
      expect(testState.batchMutateAsync).toHaveBeenCalledTimes(1)
    })
    expect(testState.batchMutateAsync).toHaveBeenCalledWith({
      ids: [7],
      action: 'disable',
    })
  })
})
