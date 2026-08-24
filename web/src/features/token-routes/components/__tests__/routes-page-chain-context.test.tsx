// Focused tests for the routes page chain-context banner. Onboarding deep
// links (`/token-routes?accountId=N`, `?siteId=M`) render a context banner
// above the table. The banner must resolve the ids to human-readable names
// from the loader-prefetched `useAccounts()` / `useSites()` list-query
// cache snapshots (the route loader prefetches both under the same query
// keys, so resolution is a cache hit — no extra request), and fall back to
// `#ID` only when the id genuinely is not in the cache.

import '@testing-library/jest-dom/vitest'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { RoutesPage } from '../routes-page'

type AccountEntry = { id: number; username: string }
type SiteEntry = { id: number; name: string }

const testState = vi.hoisted(() => ({
  accountId: '',
  siteId: '',
  accountsSnapshot: undefined as { accounts: AccountEntry[] } | undefined,
  sitesList: undefined as SiteEntry[] | undefined,
}))

vi.mock('@/components/data-table', () => ({
  DataTableBulkActions: () => null,
  DataTablePage: () => null,
  useUrlTableState: () => ({
    globalFilter: '',
    onGlobalFilterChange: vi.fn(),
    columnFilters: [],
    onColumnFiltersChange: vi.fn(),
    pagination: { pageIndex: 0, pageSize: 20 },
    onPaginationChange: vi.fn(),
    filters: {
      enabled: '',
      accountId: testState.accountId,
      siteId: testState.siteId,
      routeId: '',
    },
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

vi.mock('@/features/accounts/api', () => ({
  useAccounts: () => ({ data: testState.accountsSnapshot }),
}))
vi.mock('@/features/sites/api', () => ({
  useSites: () => ({ data: testState.sitesList }),
}))
vi.mock('@/features/channels/api', () => ({
  useChannels: () => ({ data: [] }),
}))

vi.mock('../../api', () => ({
  useRoutes: () => ({
    data: [],
    isLoading: false,
    isFetching: false,
    error: null,
    refetch: vi.fn().mockResolvedValue({}),
  }),
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
  RouteFormDialog: () => null,
}))

vi.mock('../route-detail-sheet', () => ({
  RouteDetailSheet: () => null,
}))

beforeEach(() => {
  testState.accountId = ''
  testState.siteId = ''
  testState.accountsSnapshot = undefined
  testState.sitesList = undefined
})

afterEach(() => cleanup())

function getBanner(): HTMLElement {
  // The banner's sentence is built from direct text nodes of one div, so a
  // regex on the element text pins the banner (and nothing else on the
  // page).
  return screen.getByText(/Configuring routes for/)
}

describe('RoutesPage chain-context banner name resolution', () => {
  it('resolves deep-link ids to the cached account and site names', () => {
    testState.accountId = '7'
    testState.siteId = '3'
    testState.accountsSnapshot = {
      accounts: [{ id: 7, username: 'alice' }],
    }
    testState.sitesList = [{ id: 3, name: 'Prod Site' }]

    render(<RoutesPage />)

    const banner = getBanner()
    expect(banner).toHaveTextContent('account "alice"')
    expect(banner).toHaveTextContent('site "Prod Site"')
    // Names replace the raw ids — no bare `#ID` survives in the banner.
    expect(banner).not.toHaveTextContent('#7')
    expect(banner).not.toHaveTextContent('#3')
  })

  it('falls back to #ID when the ids are absent from the cached snapshots', () => {
    testState.accountId = '7'
    testState.siteId = '3'
    testState.accountsSnapshot = { accounts: [] }
    testState.sitesList = []

    render(<RoutesPage />)

    const banner = getBanner()
    expect(banner).toHaveTextContent('account "#7"')
    expect(banner).toHaveTextContent('site "#3"')
  })

  it('falls back to #ID while the cache snapshots are still loading', () => {
    testState.accountId = '7'
    testState.siteId = '3'
    // useAccounts()/useSites() have not resolved yet (data undefined).
    render(<RoutesPage />)

    const banner = getBanner()
    expect(banner).toHaveTextContent('account "#7"')
    expect(banner).toHaveTextContent('site "#3"')
  })

  it('resolves each id independently when only one is cached', () => {
    testState.accountId = '7'
    testState.siteId = '3'
    testState.accountsSnapshot = {
      accounts: [{ id: 7, username: 'alice' }],
    }
    testState.sitesList = [{ id: 99, name: 'Other Site' }]

    render(<RoutesPage />)

    const banner = getBanner()
    expect(banner).toHaveTextContent('account "alice"')
    expect(banner).toHaveTextContent('site "#3"')
    expect(banner).not.toHaveTextContent('Other Site')
  })

  it('renders no banner without a chain-context deep link', () => {
    testState.accountsSnapshot = {
      accounts: [{ id: 7, username: 'alice' }],
    }
    testState.sitesList = [{ id: 3, name: 'Prod Site' }]

    render(<RoutesPage />)

    expect(screen.queryByText(/Configuring routes for/)).not.toBeInTheDocument()
  })
})
