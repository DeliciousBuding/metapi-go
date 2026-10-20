import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
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
import { api } from '@/lib/api'

import {
  buildGatewayRequestsHref,
  GatewayRequestsPage,
  readGatewayRequestsSearch,
  ProxyLogsWorkspace,
} from '../components/gateway-requests-page'
import { ProxyLogsAutoRefreshToggle } from '../components/proxy-logs-auto-refresh-toggle'
import { proxyLogsSearchSchema } from '../lib/proxy-logs-schema'
import { useProxyLogsAutoRefresh } from '../lib/use-proxy-logs-auto-refresh'
import { gatewayRequestFixture } from './gateway-request-fixture'

beforeAll(() => {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: vi.fn((query: string) => ({
      matches: false,
      media: query,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    })),
  })
  class ResizeObserverStub {
    observe() {}
    unobserve() {}
    disconnect() {}
  }
  Object.defineProperty(globalThis, 'ResizeObserver', {
    writable: true,
    value: ResizeObserverStub,
  })
})

beforeEach(() => {
  localStorage.removeItem('metapi-go:proxy-logs:auto-refresh')
  vi.spyOn(api, 'getProxyLogsQuery').mockResolvedValue({
    items: [],
    total: 0,
    page: 1,
    pageSize: 20,
  })
  vi.spyOn(api, 'getProxyLogsMeta').mockResolvedValue({
    clientOptions: [],
    sites: [],
    summary: {
      totalCount: 0,
      successCount: 0,
      failedCount: 0,
      totalCost: 0,
      totalTokensAll: 0,
    },
  })
  vi.spyOn(api, 'getGatewayRequests').mockResolvedValue({
    items: [
      gatewayRequestFixture({
        terminalStatus: 'output_interrupted',
        httpStatus: 200,
      }),
    ],
    total: 1,
    page: 1,
    pageSize: 20,
    summary: {
      totalCount: 1,
      completedCount: 0,
      failedCount: 1,
      cancelledCount: 0,
      rejectedCount: 0,
    },
    coverageStartedAt: '2026-10-16T00:00:00Z',
  })
  vi.spyOn(api, 'getGatewayRequest').mockResolvedValue(
    gatewayRequestFixture({ terminalStatus: 'output_interrupted' })
  )
  vi.spyOn(api, 'getProxyLogDetail').mockResolvedValue({
    ...gatewayRequestFixture().attempts[0],
    status: 'success',
    modelRequested: 'synthetic-model',
    modelActual: 'synthetic-upstream-model',
    latencyMs: 115,
    retryCount: 0,
  })
})
afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

function renderPage(entry = '/proxy-logs', workspace = false) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const root = createRootRoute({
    component: () => (
      <QueryClientProvider client={client}>
        <Outlet />
      </QueryClientProvider>
    ),
  })
  const route = createRoute({
    getParentRoute: () => root,
    path: '/proxy-logs',
    validateSearch: proxyLogsSearchSchema,
    component: workspace ? ProxyLogsWorkspace : GatewayRequestsPage,
  })
  const router = createRouter({
    routeTree: root.addChildren([route]),
    history: createMemoryHistory({ initialEntries: [entry] }),
  })
  render(<RouterProvider router={router} />)
  return { router, client }
}

describe('request list navigation and read states', () => {
  it('switches views with shared scope intact, discards incompatible filters and supports Back', async () => {
    const { router } = renderPage(
      '/proxy-logs?downstreamKeyId=7&from=2026-10-16T00%3A00&to=2026-10-17T00%3A00&pageSize=50&requestStatus=output_interrupted&q=synthetic',
      true
    )
    const attempts = await screen.findByRole('link', {
      name: 'Attempt records',
    })
    expect(
      screen.getByRole('link', { name: 'Request outcomes' })
    ).toHaveAttribute('aria-current', 'page')
    fireEvent.click(attempts)
    await waitFor(() => expect(api.getProxyLogsQuery).toHaveBeenCalled())
    expect(router.state.location.search).toMatchObject({
      view: 'attempts',
      downstreamKeyId: 7,
      pageSize: 50,
      from: '2026-10-16T00:00',
      to: '2026-10-17T00:00',
      requestStatus: 'all',
    })
    expect(router.state.location.search.q).toBeUndefined()
    expect(
      screen.getByRole('link', { name: 'Attempt records' })
    ).toHaveAttribute('aria-current', 'page')
    fireEvent.click(screen.getByRole('link', { name: 'Request outcomes' }))
    await waitFor(() =>
      expect(router.state.location.search.view).toBe('requests')
    )
    expect(router.state.location.search).toMatchObject({
      downstreamKeyId: 7,
      pageSize: 50,
    })
    router.history.back()
    await waitFor(() =>
      expect(
        screen.getByRole('link', { name: 'Attempt records' })
      ).toHaveAttribute('aria-current', 'page')
    )
  })

  it('keeps legacy attempt deep links honest and clears their filters when switching to requests', async () => {
    const { router } = renderPage(
      '/proxy-logs?siteId=3&status=failed&latencyMin=2000&downstreamKeyId=7',
      true
    )
    expect(
      await screen.findByRole('link', { name: 'Attempt records' })
    ).toHaveAttribute('aria-current', 'page')
    fireEvent.click(screen.getByRole('link', { name: 'Request outcomes' }))
    await waitFor(() => expect(api.getGatewayRequests).toHaveBeenCalled())
    expect(router.state.location.search).not.toHaveProperty('siteId')
    expect(router.state.location.search).not.toHaveProperty('latencyMin')
    expect(router.state.location.search).toMatchObject({
      view: 'requests',
      status: 'all',
      downstreamKeyId: 7,
    })
  })

  it('explains attempts on demand without permanently occupying a banner', async () => {
    renderPage('/proxy-logs?view=attempts', true)
    const help = await screen.findByRole('button', {
      name: 'About these records',
    })
    fireEvent.click(help)
    expect(
      await screen.findByRole('dialog', { name: 'About these records' })
    ).toHaveTextContent('Attempt records are not request outcomes')
  })

  it('changes the controlled refresh interval and restores the preference on remount', async () => {
    function RefreshPreference() {
      return <ProxyLogsAutoRefreshToggle {...useProxyLogsAutoRefresh()} />
    }
    const first = render(<RefreshPreference />)
    fireEvent.click(screen.getByRole('button', { name: 'Auto-refresh: Off' }))
    fireEvent.click(screen.getByRole('menuitem', { name: '15s' }))
    expect(
      screen.getByRole('button', { name: 'Auto-refresh: 15s' })
    ).toBeInTheDocument()
    expect(localStorage.getItem('metapi-go:proxy-logs:auto-refresh')).toBe(
      '15000'
    )
    first.unmount()
    render(<RefreshPreference />)
    fireEvent.click(screen.getByRole('button', { name: 'Auto-refresh: 15s' }))
    fireEvent.click(screen.getByRole('menuitem', { name: 'Off' }))
    expect(
      screen.getByRole('button', { name: 'Auto-refresh: Off' })
    ).toBeInTheDocument()
    expect(localStorage.getItem('metapi-go:proxy-logs:auto-refresh')).toBe(
      'false'
    )
  })

  it('keeps key, range and page in the URL while opening and closing the single detail sheet', async () => {
    const { router } = renderPage(
      '/proxy-logs?downstreamKeyId=7&from=2026-10-16T00%3A00&page=2'
    )
    const open = await screen.findByRole('button', {
      name: 'Open request details',
    })
    expect(screen.getByText('Output interrupted')).toBeInTheDocument()
    expect(screen.getByText('HTTP 200')).toBeInTheDocument()
    expect(api.getGatewayRequests).toHaveBeenCalledWith(
      expect.objectContaining({
        offset: 40,
        downstreamKeyId: 7,
        from: '2026-10-16T00:00',
      })
    )
    fireEvent.click(open)
    await waitFor(() =>
      expect(router.state.location.search).toMatchObject({
        requestId: 'synthetic-request-1',
        page: 2,
        downstreamKeyId: 7,
      })
    )
    expect(
      await screen.findByRole('dialog', { name: 'Request details' })
    ).toBeInTheDocument()
    fireEvent.click(screen.getAllByRole('button', { name: 'Close' })[0])
    await waitFor(() =>
      expect(router.state.location.search).not.toHaveProperty('requestId')
    )
    expect(router.state.location.search).toMatchObject({
      page: 2,
      downstreamKeyId: 7,
      from: '2026-10-16T00:00',
    })
    expect(api.getGatewayRequests).toHaveBeenCalledTimes(1)
  })
  it('shows retry on a read failure, not the successful empty-state action', async () => {
    vi.mocked(api.getGatewayRequests).mockRejectedValueOnce(
      new Error('synthetic request query failed')
    )
    renderPage()
    expect(
      await screen.findByText(/Could not load requests/)
    ).toBeInTheDocument()
    expect(
      screen.queryByText('No recorded requests in this view')
    ).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /Retry/ }))
    expect(await screen.findByText('Output interrupted')).toBeInTheDocument()
  })
  it('preserves latest URL state and defaults malformed navigation values without losing unknowns', () => {
    const href = buildGatewayRequestsHref(
      { pageIndex: 0, filters: { status: 'client_cancelled' } },
      '?q=synthetic&downstreamKeyId=7&from=2026-10-16T00%3A00&page=3&requestId=synthetic-request-1'
    )
    const parsed = readGatewayRequestsSearch(href.split('?')[1])
    expect(parsed).toMatchObject({
      q: 'synthetic',
      pageIndex: 0,
      filters: {
        downstreamKeyId: '7',
        status: 'client_cancelled',
        requestId: 'synthetic-request-1',
      },
    })
    expect(
      readGatewayRequestsSearch(
        '?page=-1&pageSize=broken&requestStatus=success&downstreamKeyId=0'
      )
    ).toMatchObject({
      pageIndex: 0,
      pageSize: 20,
      filters: { status: 'all', downstreamKeyId: '' },
    })
  })
})
