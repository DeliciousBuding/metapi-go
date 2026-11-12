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

import { AuditLogsPage } from '../components/audit-logs-page'
import { gatewayRequestFixture } from './gateway-request-fixture'

beforeAll(() => {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: vi.fn().mockImplementation((media: string) => ({
      matches: false,
      media,
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
  vi.spyOn(api, 'getAdminAuditLogs').mockResolvedValue({
    items: [
      {
        id: 1,
        actor: 'operator',
        method: 'PATCH',
        path: '/api/sites/1',
        status: 200,
        requestId: 'audit-1',
        remoteIp: '127.0.0.1',
        createdAt: '2026-11-12T00:00:00Z',
      },
    ],
    total: 1,
    limit: 20,
    offset: 0,
  })
  vi.spyOn(api, 'getGatewayRequests').mockResolvedValue({
    items: [
      gatewayRequestFixture({
        id: 'catalog-access',
        modelRequested: null,
        downstreamPath: '/v1/models',
      }),
    ],
    total: 1,
    page: 1,
    pageSize: 20,
    summary: {
      totalCount: 1,
      completedCount: 1,
      failedCount: 0,
      cancelledCount: 0,
      rejectedCount: 0,
    },
    coverageStartedAt: null,
  })
})
afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

function renderAudit(entry = '/audit-logs') {
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
    path: '/audit-logs',
    component: AuditLogsPage,
  })
  const router = createRouter({
    routeTree: root.addChildren([route]),
    history: createMemoryHistory({ initialEntries: [entry] }),
  })
  render(<RouterProvider router={router} />)
  return router
}

describe('audit workspace', () => {
  it('keeps model usage out of management operations and exposes API access separately', async () => {
    renderAudit()
    expect(await screen.findByText('/api/sites/1')).toBeInTheDocument()
    expect(api.getGatewayRequests).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('link', { name: 'API access' }))
    expect(await screen.findByText('/v1/models')).toBeInTheDocument()
    expect(api.getGatewayRequests).toHaveBeenCalledWith(
      expect.objectContaining({ kind: 'access' })
    )
    expect(screen.queryByText('Tokens')).not.toBeInTheDocument()
  })

  it('preserves audit filters and pagination during URL navigation', async () => {
    const router = renderAudit('/audit-logs?method=PATCH&q=sites&page=2')
    expect(await screen.findByText('/api/sites/1')).toBeInTheDocument()
    const params = vi.mocked(api.getAdminAuditLogs).mock.calls[0][0]
    expect(params?.get('method')).toBe('PATCH')
    expect(params?.get('path')).toBe('sites')
    expect(params?.get('offset')).toBe('40')
    await router.navigate({
      to: '/audit-logs',
      search: { method: 'POST', q: 'models', page: 0 },
    })
    await waitFor(() => expect(api.getAdminAuditLogs).toHaveBeenCalledTimes(2))
    const latest = vi.mocked(api.getAdminAuditLogs).mock.calls.at(-1)?.[0]
    expect(latest?.get('method')).toBe('POST')
    expect(latest?.get('path')).toBe('models')
    expect(latest?.get('offset')).toBe('0')
  })

  it('shows retry on invalid audit responses rather than treating them as an empty success', async () => {
    vi.mocked(api.getAdminAuditLogs).mockResolvedValueOnce({ items: null })
    renderAudit()
    expect(
      await screen.findByRole('button', { name: /Retry/ })
    ).toBeInTheDocument()
    expect(screen.queryByText('/api/sites/1')).not.toBeInTheDocument()
  })
})
