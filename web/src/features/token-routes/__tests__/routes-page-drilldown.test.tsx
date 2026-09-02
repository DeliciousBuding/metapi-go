// Behavior tests for the token-routes one-shot drilldowns:
// - proxy-log detail -> `/token-routes?routeId=N`: the page opens the route
//   detail sheet for the referenced route and strips the param, without
//   disturbing the persistent accountId/siteId chain context.
// - channel detail sheet -> `/token-routes?edit=N`: the page opens the edit
//   dialog (same state as the row edit action) for the referenced route and
//   strips the param.
// A stale id strips silently in both flows.

import '@testing-library/jest-dom/vitest'
import { act, cleanup, render, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { RoutesPage } from '../components/routes-page'
import type { RouteSummaryRow } from '../types'

const testState = vi.hoisted(() => ({
  routeIdParam: '',
  routerSearch: {} as Record<string, unknown>,
  routes: [] as RouteSummaryRow[],
  routesLoading: false,
  navigate: vi.fn(),
  detailSheetProps: null as {
    route: RouteSummaryRow | null
    open: boolean
    onEdit?: (route: RouteSummaryRow) => void
  } | null,
  formDialogProps: null as {
    route: RouteSummaryRow | null
    open: boolean
    mode: 'create' | 'edit'
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

vi.mock('@/features/channels/api', () => ({
  useChannels: () => ({ data: [] }),
}))

vi.mock('../api', () => ({
  useRoutes: () => ({
    data: testState.routes,
    isLoading: testState.routesLoading,
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
// The page's row-delete path uses the shared undo helper; tests stub it so
// no QueryClientProvider is required.
vi.mock('@/lib/undoable-delete', () => ({
  useUndoableDelete: () => vi.fn(),
}))
// The step 3 → 4 handoff strip mounts whenever routes exist, which the
// drilldown cases arrange. It owns its own downstream-keys query and has its
// own suite (routes-key-next-step.test.tsx) — stubbed here so this file stays
// about the routeId/edit deep links.
vi.mock('../components/routes-key-next-step', () => ({
  RoutesKeyNextStep: () => null,
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
  RouteFormDialog: (props: {
    route: RouteSummaryRow | null
    open: boolean
    mode: 'create' | 'edit'
  }) => {
    testState.formDialogProps = props
    return null
  },
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
  testState.routesLoading = false
  testState.navigate.mockReset()
  testState.detailSheetProps = null
  testState.formDialogProps = null
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

describe('RoutesPage edit drilldown', () => {
  it('opens the edit dialog for the referenced route and strips the param', async () => {
    testState.routerSearch = { edit: 5, accountId: 7 }
    testState.routes = [makeRoute(5), makeRoute(6)]

    render(<RoutesPage />)

    await waitFor(() => {
      expect(testState.formDialogProps?.open).toBe(true)
    })
    expect(testState.formDialogProps?.mode).toBe('edit')
    expect(testState.formDialogProps?.route?.id).toBe(5)
    // The detail sheet (the old `routeId` target) must stay closed — the
    // edit deep link opens the editor, not the read-only panel.
    expect(testState.detailSheetProps?.open).toBe(false)
    // Strip is a replace-navigation that keeps the chain context intact.
    expect(testState.navigate).toHaveBeenCalledWith(
      expect.objectContaining({
        to: '/token-routes',
        replace: true,
        search: expect.objectContaining({
          edit: undefined,
          accountId: 7,
        }),
      })
    )
  })

  it('strips a stale edit id without opening the dialog', async () => {
    testState.routerSearch = { edit: 99 }
    testState.routes = [makeRoute(5)]

    render(<RoutesPage />)

    await waitFor(() => {
      expect(testState.navigate).toHaveBeenCalled()
    })
    expect(testState.formDialogProps?.open).toBe(false)
    expect(testState.formDialogProps?.route).toBeNull()
    expect(testState.navigate).toHaveBeenCalledWith(
      expect.objectContaining({
        to: '/token-routes',
        replace: true,
        search: expect.objectContaining({ edit: undefined }),
      })
    )
  })

  it('waits for the list to resolve before consuming the edit param', async () => {
    testState.routerSearch = { edit: 5 }
    testState.routes = []
    testState.routesLoading = true

    const { rerender } = render(<RoutesPage />)

    // While loading, the param must NOT be consumed and no strip navigate
    // may happen yet.
    expect(testState.formDialogProps?.open).toBe(false)
    expect(testState.navigate).not.toHaveBeenCalled()

    // Once the list resolves with the referenced route, the dialog opens
    // and the param is stripped.
    testState.routesLoading = false
    testState.routes = [makeRoute(5)]
    rerender(<RoutesPage />)

    await waitFor(() => {
      expect(testState.formDialogProps?.open).toBe(true)
    })
    expect(testState.formDialogProps?.route?.id).toBe(5)
    expect(testState.navigate).toHaveBeenCalledWith(
      expect.objectContaining({
        to: '/token-routes',
        replace: true,
        search: expect.objectContaining({ edit: undefined }),
      })
    )
  })

  it('opens the dialog exactly once across remount-like re-renders', async () => {
    testState.routerSearch = { edit: 5 }
    testState.routes = [makeRoute(5)]

    const { rerender } = render(<RoutesPage />)

    await waitFor(() => {
      expect(testState.formDialogProps?.open).toBe(true)
    })

    // After consumption the page strips `edit`; a re-render with the
    // stripped search must not navigate again (consumed-ref guard).
    testState.routerSearch = {}
    rerender(<RoutesPage />)

    expect(testState.navigate).toHaveBeenCalledTimes(1)
  })
})

describe('RoutesPage detail sheet Edit action', () => {
  it('closes the sheet and opens the edit dialog when onEdit fires', async () => {
    testState.routeIdParam = '5'
    testState.routerSearch = { routeId: 5 }
    testState.routes = [makeRoute(5)]

    render(<RoutesPage />)

    await waitFor(() => {
      expect(testState.detailSheetProps?.open).toBe(true)
    })
    const onEdit = testState.detailSheetProps?.onEdit
    expect(onEdit).toBeTypeOf('function')

    act(() => onEdit?.(makeRoute(5)))

    await waitFor(() => {
      expect(testState.detailSheetProps?.open).toBe(false)
    })
    expect(testState.formDialogProps?.open).toBe(true)
    expect(testState.formDialogProps?.mode).toBe('edit')
    expect(testState.formDialogProps?.route?.id).toBe(5)
  })
})
