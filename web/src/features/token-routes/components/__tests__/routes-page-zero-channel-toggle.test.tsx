// Focused tests for the routes page "show zero-channel models" view toggle.
// The toggle lives in the data-table toolbar's `viewToggle` slot (the view
// control cluster next to View Options) — never below the table. Toggling
// it merges zero-channel placeholder rows into the table data (asserted via
// the real placeholder builder), and the choice is persisted to
// localStorage (proxy-logs auto-refresh pattern) so the operator's view
// survives route remounts and reloads.

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
import type { MissingTokenModelsByName } from '@/lib/helpers/routeMissingTokenHints'

import type { RouteSummaryRow } from '../../types'
import { RoutesPage } from '../routes-page'

const STORAGE_KEY = 'metapi-go:token-routes:show-zero-channel'

type ZeroChannelCandidates = {
  modelsWithoutToken: MissingTokenModelsByName
  modelsMissingTokenGroups: MissingTokenModelsByName
}

const testState = vi.hoisted(() => ({
  routes: [] as RouteSummaryRow[],
  candidates: undefined as ZeroChannelCandidates | undefined,
  toolbarProps: null as { viewToggle?: ReactNode } | null,
  tableData: null as RouteSummaryRow[] | null,
}))

vi.mock('@/components/data-table', () => ({
  DataTableBulkActions: () => null,
  DataTablePage: (props: { toolbarProps?: { viewToggle?: ReactNode } }) => {
    testState.toolbarProps = props.toolbarProps ?? null
    return null
  },
  useUrlTableState: () => ({
    globalFilter: '',
    onGlobalFilterChange: vi.fn(),
    columnFilters: [],
    onColumnFiltersChange: vi.fn(),
    pagination: { pageIndex: 0, pageSize: 20 },
    onPaginationChange: vi.fn(),
    filters: { enabled: '', accountId: '', siteId: '', routeId: '' },
  }),
  useDataTable: (options: { data: RouteSummaryRow[] }) => {
    testState.tableData = options.data
    return {
      table: {
        getFilteredSelectedRowModel: () => ({ rows: [] }),
        resetRowSelection: vi.fn(),
      },
    }
  },
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

vi.mock('../../api', async () => {
  const { buildZeroChannelPlaceholderRoutes } =
    await import('@/lib/helpers/zeroChannelRoutes')
  return {
    useRoutes: () => ({
      data: testState.routes,
      isLoading: false,
      isFetching: false,
      error: null,
      refetch: vi.fn().mockResolvedValue({}),
    }),
    useModelTokenCandidates: () => ({ data: testState.candidates }),
    useDeleteRoute: () => ({ mutateAsync: vi.fn(), isPending: false }),
    useUpdateRoute: () => ({ mutate: vi.fn(), isPending: false }),
    useClearRouteCooldown: () => ({ mutate: vi.fn(), isPending: false }),
    useRebuildRoutes: () => ({ mutate: vi.fn(), isPending: false }),
    useRefreshRouteDecisions: () => ({ mutate: vi.fn(), isPending: false }),
    useBatchUpdateRoutes: () => ({ mutateAsync: vi.fn(), isPending: false }),
    // Real merge semantics (the real placeholder builder) so the tests
    // assert the rows the table actually receives, not a mocked passthrough.
    useZeroChannelRoutes: (
      routes: RouteSummaryRow[] | undefined,
      candidates: ZeroChannelCandidates | undefined,
      showZeroChannel: boolean
    ) => {
      const base = routes ?? []
      if (!showZeroChannel || !candidates) return base
      return [
        ...base,
        ...buildZeroChannelPlaceholderRoutes(
          base,
          candidates.modelsWithoutToken,
          candidates.modelsMissingTokenGroups
        ),
      ]
    },
  }
})

// The page's row-delete path uses the shared undo helper; tests stub it so
// no QueryClientProvider is required.
vi.mock('@/lib/undoable-delete', () => ({
  useUndoableDelete: () => vi.fn(),
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

const ZERO_CANDIDATES: ZeroChannelCandidates = {
  modelsWithoutToken: {
    'gpt-zero': [
      {
        accountId: 9,
        username: 'bob',
        siteId: 3,
        siteName: 'Prod Site',
      },
    ],
  },
  modelsMissingTokenGroups: {},
}

function makeRoute(id: number): RouteSummaryRow {
  return {
    id,
    routeMode: 'pattern',
    modelPattern: `model-${id}`,
    enabled: true,
  } as unknown as RouteSummaryRow
}

/** Render the toolbar `viewToggle` slot captured from DataTablePage. */
function renderToolbarToggle() {
  const toggle = testState.toolbarProps?.viewToggle
  expect(toggle).toBeTruthy()
  return render(<div>{toggle}</div>)
}

beforeEach(() => {
  testState.routes = []
  testState.candidates = undefined
  testState.toolbarProps = null
  testState.tableData = null
  window.localStorage.clear()
})

afterEach(() => cleanup())

describe('RoutesPage zero-channel toggle placement', () => {
  it('renders the switch in the toolbar viewToggle slot, not below the table', () => {
    testState.routes = [makeRoute(1)]

    render(<RoutesPage />)

    // The page's own DOM must not contain the switch (e.g. below the
    // table) — it exists only inside the toolbar slot handed to the table.
    expect(screen.queryByRole('switch')).not.toBeInTheDocument()

    const toggle = testState.toolbarProps?.viewToggle
    expect(toggle).toBeTruthy()
    const { container } = render(<div>{toggle}</div>)
    expect(within(container).getByRole('switch')).toBeInTheDocument()
    expect(
      within(container).getByText(/Show zero-channel models/)
    ).toBeInTheDocument()
  })
})

describe('RoutesPage zero-channel toggle behavior', () => {
  it('defaults to off, and toggling on merges zero-channel rows into the table', () => {
    testState.routes = [makeRoute(1)]
    testState.candidates = ZERO_CANDIDATES

    render(<RoutesPage />)

    // Default off: the table receives only the real routes.
    expect(testState.tableData).toHaveLength(1)
    const initial = renderToolbarToggle()
    expect(within(initial.container).getByRole('switch')).toHaveAttribute(
      'aria-checked',
      'false'
    )

    fireEvent.click(within(initial.container).getByRole('switch'))

    // The table now receives the merged zero-channel placeholder rows and
    // the re-rendered toolbar slot reflects the checked state.
    expect(testState.tableData).toHaveLength(2)
    expect(
      testState.tableData?.some((row) => row.modelPattern === 'gpt-zero')
    ).toBe(true)
    const updated = renderToolbarToggle()
    expect(within(updated.container).getByRole('switch')).toHaveAttribute(
      'aria-checked',
      'true'
    )

    // The choice is persisted for future mounts.
    expect(window.localStorage.getItem(STORAGE_KEY)).toBe('true')
  })

  it('restores a persisted on-state across remounts', () => {
    window.localStorage.setItem(STORAGE_KEY, 'true')
    testState.routes = [makeRoute(1)]
    testState.candidates = ZERO_CANDIDATES

    render(<RoutesPage />)

    const view = renderToolbarToggle()
    expect(within(view.container).getByRole('switch')).toHaveAttribute(
      'aria-checked',
      'true'
    )
    // The restored preference immediately applies to the table rows.
    expect(testState.tableData).toHaveLength(2)
  })

  it('keeps the toggle on across unmount/remount after the operator enables it', () => {
    testState.routes = [makeRoute(1)]
    testState.candidates = ZERO_CANDIDATES

    const first = render(<RoutesPage />)
    const firstToggle = renderToolbarToggle()
    fireEvent.click(within(firstToggle.container).getByRole('switch'))
    expect(testState.tableData).toHaveLength(2)

    // Simulate navigating away and back (full remount).
    first.unmount()
    cleanup()

    render(<RoutesPage />)
    const restored = renderToolbarToggle()
    expect(within(restored.container).getByRole('switch')).toHaveAttribute(
      'aria-checked',
      'true'
    )
    expect(testState.tableData).toHaveLength(2)
  })
})
