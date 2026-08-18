// Behavior tests for the token-routes one-shot drilldown (proxy-log detail
// -> `/token-routes?routeId=N`): the page opens the route detail sheet for
// the referenced route and strips the param, without disturbing the
// persistent accountId/siteId chain context. A stale id strips silently.

import '@testing-library/jest-dom/vitest'
import { cleanup, render, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { RoutesPage } from '../components/routes-page'
import type { RouteSummaryRow } from '../types'

const testState = vi.hoisted(() => ({
  routeIdParam: '',
  routerSearch: {} as Record<string, unknown>,
  routes: [] as RouteSummaryRow[],
  navigate: vi.fn(),
  detailSheetProps: null as {
    route: RouteSummaryRow | null
    open: boolean
  } | null,
}))

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => testState.navigate,
  useSearch: () => testState.routerSearch,
  Link: ({ children }: { children?: ReactNode }) => children,
}))

vi.mock('@/components/data-table', () => ({
  DataTablePage: () => null,
  DataTableBulkActions: () => null,
  useUrlTableState: () => ({
    globalFilter: '',
    onGlobalFilterChange: vi.fn(),
    pagination: { pageIndex: 0, pageSize: 20 },
    onPaginationChange: vi.fn(),
    columnFilters: [],
    onColumnFiltersChange: vi.fn(),
    filters: {
      enabled: '',
      accountId: '',
      siteId: '',
      routeId: testState.routeIdParam,
    },
  }),
  useDataTable: () => ({ table: {} }),
}))

vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() },
}))

vi.mock('@/features/accounts/api', () => ({
  useAccounts: () => ({ data: undefined }),
}))
vi.mock('@/features/sites/api', () => ({
  useSites: () => ({ data: undefined }),
}))

vi.mock('../api', () => ({
  useRoutes: () => ({
    data: testState.routes,
    isLoading: false,
    isFetching: false,
    error: null,
  }),
  useModelTokenCandidates: () => ({ data: undefined }),
  useDeleteRoute: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useUpdateRoute: () => ({ mutate: vi.fn(), isPending: false }),
  useClearRouteCooldown: () => ({ mutate: vi.fn(), isPending: false }),
  useRebuildRoutes: () => ({ mutate: vi.fn(), isPending: false }),
  useRefreshRouteDecisions: () => ({ mutate: vi.fn(), isPending: false }),
  useBatchUpdateRoutes: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useZeroChannelRoutes: (routes: RouteSummaryRow[]) => routes,
}))

vi.mock('../components/route-detail-sheet', () => ({
  RouteDetailSheet: (props: {
    route: RouteSummaryRow | null
    open: boolean
  }) => {
    testState.detailSheetProps = props
    return null
  },
}))

vi.mock('../components/route-form-dialog', () => ({
  RouteFormDialog: () => null,
}))

vi.mock('../components/routes-columns', () => ({
  useRoutesColumns: () => [],
}))

function makeRoute(id: number): RouteSummaryRow {
  return {
    id,
    routeMode: 'pattern',
    modelPattern: `model-${id}`,
    enabled: true,
  } as unknown as RouteSummaryRow
}

beforeEach(() => {
  testState.routeIdParam = ''
  testState.routerSearch = {}
  testState.routes = []
  testState.navigate.mockReset()
  testState.detailSheetProps = null
})

afterEach(() => cleanup())

describe('RoutesPage drilldown', () => {
  it('opens the detail sheet for the referenced route and strips the param', async () => {
    testState.routeIdParam = '5'
    testState.routerSearch = { routeId: 5, accountId: 7 }
    testState.routes = [makeRoute(5), makeRoute(6)]

    render(<RoutesPage />)

    await waitFor(() => {
      expect(testState.detailSheetProps?.open).toBe(true)
    })
    expect(testState.detailSheetProps?.route?.id).toBe(5)
    // Strip is a replace-navigation that keeps the chain context intact.
    expect(testState.navigate).toHaveBeenCalledWith(
      expect.objectContaining({
        to: '/token-routes',
        replace: true,
        search: expect.objectContaining({
          routeId: undefined,
          accountId: 7,
        }),
      })
    )
  })

  it('strips a stale route id without opening the sheet', async () => {
    testState.routeIdParam = '99'
    testState.routes = [makeRoute(5)]

    render(<RoutesPage />)

    await waitFor(() => {
      expect(testState.navigate).toHaveBeenCalled()
    })
    expect(testState.detailSheetProps?.open).toBe(false)
    expect(testState.detailSheetProps?.route).toBeNull()
  })

  it('does not navigate when no drilldown param is present', async () => {
    testState.routes = [makeRoute(5)]

    render(<RoutesPage />)

    await waitFor(() => {
      expect(testState.detailSheetProps).not.toBeNull()
    })
    expect(testState.navigate).not.toHaveBeenCalled()
    expect(testState.detailSheetProps?.open).toBe(false)
  })
})
