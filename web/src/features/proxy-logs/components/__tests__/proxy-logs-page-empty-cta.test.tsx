// Regression test: the proxy-logs empty state was a dead end — users staring
// at "no logs" had no path forward. It must offer a "View routes" CTA that
// navigates to the token-routes page (audit #1029 batch B).
import '@testing-library/jest-dom/vitest'
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from '@tanstack/react-router'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import type { ReactNode } from 'react'
import {
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from 'vitest'

import '@/i18n/config'

import { ProxyLogsPage } from '../proxy-logs-page'

const testState = vi.hoisted(() => ({
  metaQuery: {
    data: undefined,
    error: null as Error | null,
    isFetching: false,
    refetch: vi.fn(),
  },
}))

vi.mock('@/components/data-table', () => ({
  DataTablePage: (props: { emptyAction?: ReactNode }) => (
    <div>{props.emptyAction}</div>
  ),
  useUrlTableState: () => ({
    globalFilter: '',
    onGlobalFilterChange: vi.fn(),
    columnFilters: [],
    onColumnFiltersChange: vi.fn(),
    pagination: { pageIndex: 0, pageSize: 20 },
    onPaginationChange: vi.fn(),
    filters: {
      status: '',
      siteId: '',
      channelId: '',
      client: '',
      from: '',
      to: '',
      latencyMin: '',
      latencyMax: '',
    },
  }),
  useDataTable: () => ({ table: {} }),
}))

vi.mock('@/lib/api', () => ({
  api: { getProxyLogsQuery: vi.fn() },
}))

vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() },
}))

vi.mock('../../api', () => ({
  useProxyLogs: () => ({
    data: { items: [], total: 0 },
    isLoading: false,
    isFetching: false,
    refetch: vi.fn(),
  }),
  useProxyLogsMeta: () => testState.metaQuery,
}))

vi.mock('../../lib/use-proxy-logs-auto-refresh', () => ({
  useProxyLogsAutoRefresh: () => ({
    intervalMs: false,
    setIntervalMs: vi.fn(),
  }),
}))

vi.mock('../proxy-logs-header-actions', () => ({
  ProxyLogsHeaderActions: () => null,
}))

vi.mock('../proxy-log-detail-sheet', () => ({
  ProxyLogDetailSheet: () => null,
}))

vi.mock('../proxy-logs-auto-refresh-toggle', () => ({
  ProxyLogsAutoRefreshToggle: () => null,
}))

vi.mock('../proxy-logs-columns', () => ({
  useProxyLogsColumns: () => [],
}))

beforeAll(() => {
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
  testState.metaQuery.data = undefined
  testState.metaQuery.error = null
  testState.metaQuery.isFetching = false
  testState.metaQuery.refetch.mockReset()
})

afterEach(() => cleanup())

function renderPage() {
  const root = createRootRoute({ component: Outlet })
  const logs = createRoute({
    getParentRoute: () => root,
    path: '/proxy-logs',
    component: ProxyLogsPage,
  })
  const routes = createRoute({
    getParentRoute: () => root,
    path: '/token-routes',
    component: () => null,
  })
  const router = createRouter({
    routeTree: root.addChildren([logs, routes]),
    history: createMemoryHistory({
      initialEntries: ['/proxy-logs?view=attempts'],
    }),
  })
  render(<RouterProvider router={router} />)
  return router
}

describe('ProxyLogsPage empty-state CTA', () => {
  it('offers a View routes CTA that navigates to /token-routes', async () => {
    const router = renderPage()

    const cta = await screen.findByRole('button', { name: 'View routes' })
    expect(cta).toBeInTheDocument()

    fireEvent.click(cta)
    await waitFor(() =>
      expect(router.state.location.pathname).toBe('/token-routes')
    )
  })
})

describe('ProxyLogsPage meta error banner', () => {
  it('surfaces a meta query failure with a Retry action', async () => {
    testState.metaQuery.error = new Error('meta boom')
    renderPage()

    expect(
      await screen.findByText(/Failed to load log summary: meta boom/)
    ).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(testState.metaQuery.refetch).toHaveBeenCalledTimes(1)
  })
})
