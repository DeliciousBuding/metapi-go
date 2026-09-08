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
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import axe from 'axe-core'
import {
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from 'vitest'

import i18n from '@/i18n/config'

import type { RebuildRoutesResult } from '../../api'
import {
  ROUTE_REBUILD_STORAGE_KEY,
  routeRebuildTaskQueryKey,
} from '../../lib/route-rebuild-reference'
import type { RouteRebuildTask } from '../../lib/use-route-rebuild-task'
import { RoutesPage } from '../routes-page'

const boundary = vi.hoisted(() => ({
  rebuild: vi.fn(),
  getTask: vi.fn(),
  routes: vi.fn(),
  candidates: vi.fn(),
  channels: vi.fn(),
  accounts: vi.fn(),
  sites: vi.fn(),
  success: vi.fn(),
  info: vi.fn(),
  warning: vi.fn(),
  error: vi.fn(),
}))
vi.mock('@/lib/api', () => ({
  api: {
    rebuildRoutes: boundary.rebuild,
    getTask: boundary.getTask,
    getRoutesSummary: boundary.routes,
    getModelTokenCandidates: boundary.candidates,
    getChannels: boundary.channels,
    getAccountsSnapshot: boundary.accounts,
    getSites: boundary.sites,
  },
}))
vi.mock('@/lib/toast', () => ({
  toast: {
    success: boundary.success,
    info: boundary.info,
    warning: boundary.warning,
    error: boundary.error,
  },
}))

const clients: QueryClient[] = []
const completeResult: RebuildRoutesResult = {
  success: true,
  queued: false,
  status: 'completed',
  routesCreated: 2,
  routesConsidered: 5,
  channelsInserted: 7,
  channelsRemoved: 1,
  channelsKept: 3,
  unsafeModelsSkipped: 0,
  changed: true,
  modelRefresh: { total: 4, success: 4, failed: 0, notProcessed: 0 },
}
function task(
  status: RouteRebuildTask['status'],
  result?: RebuildRoutesResult,
  id = 'rebuild-1'
) {
  return {
    success: true,
    task: {
      id,
      type: 'routes-rebuild',
      status,
      result: result ?? null,
      error: status === 'failed' ? 'route rebuild failed' : null,
    },
  }
}
function queued(reused = false, id = 'rebuild-1') {
  return {
    success: true,
    queued: true,
    reused,
    jobId: id,
    taskId: id,
    status: 'pending',
  }
}
function createTestRouter() {
  const rootRoute = createRootRoute({ component: Outlet })
  const authenticated = createRoute({
    getParentRoute: () => rootRoute,
    id: '_authenticated',
    component: Outlet,
  })
  const routesRoute = createRoute({
    getParentRoute: () => authenticated,
    path: 'token-routes',
    component: RoutesPage,
  })
  const awayRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: '/',
    component: () => <p>Other page</p>,
  })
  return createRouter({
    routeTree: rootRoute.addChildren([
      authenticated.addChildren([routesRoute]),
      awayRoute,
    ]),
    history: createMemoryHistory({ initialEntries: ['/token-routes'] }),
  })
}
async function renderPage(queryClient?: QueryClient) {
  const client =
    queryClient ??
    new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    })
  const router = createTestRouter()
  clients.push(client)
  const view = render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  await screen.findByRole('heading', { name: i18n.t('tokenRoutes.page.title') })
  await waitFor(() => expect(boundary.routes).toHaveBeenCalled())
  return { ...view, client, router }
}
async function startRebuild() {
  fireEvent.click(screen.getAllByRole('button', { name: 'Auto-rebuild' })[0])
  const confirmation = await screen.findByRole('alertdialog')
  fireEvent.click(
    within(confirmation).getByRole('button', { name: 'Auto-rebuild' })
  )
}
async function pollTask() {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(2000)
  })
}

beforeAll(() => {
  vi.stubGlobal(
    'ResizeObserver',
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    }
  )
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: vi.fn((query: string) => ({
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
beforeEach(async () => {
  await i18n.changeLanguage('en')
  window.localStorage.clear()
  for (const mock of Object.values(boundary)) mock.mockReset()
  boundary.routes.mockResolvedValue([])
  boundary.candidates.mockResolvedValue({ models: {} })
  boundary.channels.mockResolvedValue({ items: [] })
  boundary.accounts.mockResolvedValue({ accounts: [], sites: [] })
  boundary.sites.mockResolvedValue([])
  boundary.rebuild.mockResolvedValue(queued())
  boundary.getTask.mockResolvedValue(task('running'))
  vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval'] })
})
afterEach(() => {
  cleanup()
  for (const client of clients.splice(0)) client.clear()
  vi.useRealTimers()
})

describe('RoutesPage background rebuild', () => {
  it('tracks an accepted task through pending and running before showing the server result', async () => {
    boundary.getTask
      .mockResolvedValueOnce(task('pending'))
      .mockResolvedValue(task('running'))
    const { client } = await renderPage()
    const invalidate = vi.spyOn(client, 'invalidateQueries')
    await startRebuild()

    await screen.findByRole('heading', { name: 'Rebuild queued' })
    expect(boundary.rebuild).toHaveBeenCalledWith(true, false)
    expect(
      JSON.parse(
        window.localStorage.getItem(ROUTE_REBUILD_STORAGE_KEY) ?? 'null'
      )
    ).toEqual({ taskId: 'rebuild-1', refreshModels: true })
    expect(invalidate).not.toHaveBeenCalledWith({ queryKey: ['routes'] })
    for (const button of screen.getAllByRole('button', {
      name: 'Auto-rebuild',
    })) {
      expect(button).toBeDisabled()
      fireEvent.click(button)
    }
    expect(boundary.rebuild).toHaveBeenCalledTimes(1)
    await pollTask()
    await screen.findByRole('heading', { name: 'Rebuild running' })
    boundary.getTask.mockResolvedValue(task('succeeded', completeResult))
    await pollTask()

    await screen.findByRole('heading', { name: 'Rebuild completed' })
    expect(
      screen.getByText('2 routes created · 7 channels added · 1 removed')
    ).toBeVisible()
    const details = screen.getByRole('button', { name: 'View details' })
    expect(details).toHaveAttribute('aria-expanded', 'false')
    expect(screen.getByText('Routes created')).not.toBeVisible()
    fireEvent.click(details)
    expect(screen.getByText('Routes created')).toBeVisible()
    expect(
      screen.getByText('Routes created').nextElementSibling
    ).toHaveTextContent('2')
    expect(
      screen.getByText('Channels added').nextElementSibling
    ).toHaveTextContent('7')
    for (const queryKey of [
      ['routes'],
      ['channels'],
      ['accounts'],
      ['models'],
      ['account-models'],
    ]) {
      expect(invalidate).toHaveBeenCalledWith({ queryKey })
    }
    for (const button of screen.getAllByRole('button', {
      name: 'Auto-rebuild',
    })) {
      expect(button).toBeEnabled()
    }
    const observations = boundary.getTask.mock.calls.length
    await pollTask()
    expect(boundary.getTask).toHaveBeenCalledTimes(observations)
  })

  it('shows a server-confirmed failure after running and can retry with the original parameters', async () => {
    window.localStorage.setItem(
      ROUTE_REBUILD_STORAGE_KEY,
      JSON.stringify({ taskId: 'rebuild-1', refreshModels: false })
    )
    await renderPage()
    await screen.findByRole('heading', { name: 'Rebuild running' })
    boundary.getTask.mockResolvedValue(task('failed'))
    await pollTask()

    const failure = await screen.findByRole('heading', {
      name: 'Rebuild failed',
    })
    expect(failure.closest('[role="alert"]')).toHaveTextContent(
      'route rebuild failed'
    )
    expect(
      screen.queryByRole('heading', { name: 'Rebuild completed' })
    ).not.toBeInTheDocument()
    expect(boundary.success).not.toHaveBeenCalled()
    const calls = boundary.getTask.mock.calls.length
    await pollTask()
    expect(boundary.getTask).toHaveBeenCalledTimes(calls)

    boundary.rebuild.mockResolvedValue(queued(false, 'rebuild-2'))
    boundary.getTask.mockResolvedValue(task('running', undefined, 'rebuild-2'))
    fireEvent.click(
      screen.getByRole('button', { name: 'Recover or restart rebuild' })
    )
    await screen.findByRole('heading', { name: 'Rebuild running' })
    expect(boundary.rebuild).toHaveBeenCalledWith(false, false)
    expect(screen.getByText('Task: rebuild-2')).toBeInTheDocument()
  })

  it.each([
    {
      name: 'failed account model scans, including empty model availability',
      override: {
        modelRefresh: { total: 4, success: 3, failed: 1, notProcessed: 0 },
      },
      copy: '1 failed; 0 not processed',
    },
    {
      name: 'unprocessed account models',
      override: {
        modelRefresh: { total: 4, success: 3, failed: 0, notProcessed: 1 },
      },
      copy: '0 failed; 1 not processed',
    },
    {
      name: 'unsafe model names',
      override: { unsafeModelsSkipped: 3 },
      copy: '0 failed; 0 not processed',
    },
  ])(
    'reports $name as warnings instead of full success',
    async ({ override, copy }) => {
      await renderPage()
      await startRebuild()
      await screen.findByRole('heading', { name: 'Rebuild running' })
      boundary.getTask.mockResolvedValue(
        task('succeeded', { ...completeResult, ...override })
      )
      await pollTask()

      const heading = await screen.findByRole('heading', {
        name: 'Rebuild finished with warnings',
      })
      expect(heading.closest('[role="alert"]')).toBeInTheDocument()
      expect(screen.getByText(/Account model refresh:/)).toHaveTextContent(copy)
      expect(
        screen.getByText('Unsafe models skipped').nextElementSibling
      ).toHaveTextContent(String(override.unsafeModelsSkipped ?? 0))
      expect(
        screen.queryByRole('heading', { name: 'Rebuild completed' })
      ).not.toBeInTheDocument()
      expect(boundary.success).not.toHaveBeenCalled()
    }
  )

  it('rechecks the retained task after actual navigation away and back without relaunching', async () => {
    const { router } = await renderPage()
    await startRebuild()
    await screen.findByRole('heading', { name: 'Rebuild running' })
    await act(async () => {
      await router.navigate({ to: '/' })
    })
    await screen.findByText('Other page')
    const previousReads = boundary.getTask.mock.calls.length
    boundary.getTask.mockResolvedValue(task('succeeded', completeResult))

    await act(async () => {
      await router.navigate({ to: '/token-routes' })
    })

    await screen.findByRole('heading', { name: 'Rebuild completed' })
    expect(boundary.getTask.mock.calls.length).toBeGreaterThan(previousReads)
    expect(boundary.rebuild).toHaveBeenCalledTimes(1)
  })

  it('restores only the stored reference after a reload with a fresh QueryClient', async () => {
    const first = await renderPage()
    await startRebuild()
    await screen.findByRole('heading', { name: 'Rebuild running' })
    first.unmount()
    first.client.clear()
    const previousReads = boundary.getTask.mock.calls.length
    boundary.getTask.mockResolvedValue(task('succeeded', completeResult))

    await renderPage()

    await screen.findByRole('heading', { name: 'Rebuild completed' })
    expect(boundary.getTask.mock.calls.length).toBeGreaterThan(previousReads)
    expect(boundary.getTask).toHaveBeenLastCalledWith(
      'rebuild-1',
      expect.objectContaining({ skipErrorHandler: true })
    )
    expect(boundary.rebuild).toHaveBeenCalledTimes(1)
    expect(
      JSON.parse(
        window.localStorage.getItem(ROUTE_REBUILD_STORAGE_KEY) ?? 'null'
      )
    ).toEqual({ taskId: 'rebuild-1', refreshModels: true })
  })

  it('does not display a cached terminal result until the server re-verifies it', async () => {
    window.localStorage.setItem(
      ROUTE_REBUILD_STORAGE_KEY,
      JSON.stringify({ taskId: 'rebuild-1', refreshModels: true })
    )
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    client.setQueryData(
      routeRebuildTaskQueryKey('rebuild-1'),
      task('succeeded', completeResult).task
    )
    let rejectRead!: (reason: Error) => void
    boundary.getTask.mockReturnValue(
      new Promise((_resolve, reject) => {
        rejectRead = reject
      })
    )
    const invalidate = vi.spyOn(client, 'invalidateQueries')
    await renderPage(client)

    await screen.findByRole('heading', { name: 'Checking rebuild task' })
    expect(
      screen.queryByRole('heading', { name: 'Rebuild completed' })
    ).not.toBeInTheDocument()
    await act(async () => {
      rejectRead(new Error('offline'))
    })
    await screen.findByRole('heading', {
      name: 'Task status is temporarily unavailable',
    })
    expect(invalidate).not.toHaveBeenCalledWith({ queryKey: ['routes'] })
    expect(
      screen.queryByRole('button', { name: 'Dismiss result' })
    ).not.toBeInTheDocument()
    expect(window.localStorage.getItem(ROUTE_REBUILD_STORAGE_KEY)).toContain(
      'rebuild-1'
    )
  })

  it('keeps a running task on network errors and retries observation, not the rebuild', async () => {
    const { client } = await renderPage()
    await startRebuild()
    await screen.findByRole('heading', { name: 'Rebuild running' })
    const invalidate = vi.spyOn(client, 'invalidateQueries')
    boundary.getTask.mockRejectedValue(new Error('network offline'))
    await pollTask()

    await screen.findByRole('heading', {
      name: 'Task status is temporarily unavailable',
    })
    expect(
      screen.queryByRole('heading', { name: 'Rebuild failed' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('heading', { name: 'Rebuild completed' })
    ).not.toBeInTheDocument()
    expect(invalidate).not.toHaveBeenCalledWith({ queryKey: ['routes'] })
    expect(window.localStorage.getItem(ROUTE_REBUILD_STORAGE_KEY)).toContain(
      'rebuild-1'
    )
    const reads = boundary.getTask.mock.calls.length
    await pollTask()
    expect(boundary.getTask).toHaveBeenCalledTimes(reads)
    for (const button of screen.getAllByRole('button', {
      name: 'Auto-rebuild',
    })) {
      expect(button).toBeDisabled()
    }

    boundary.getTask.mockResolvedValue(task('running'))
    const retry = screen.getByRole('button', { name: 'Retry status check' })
    act(() => retry.focus())
    expect(retry).toHaveFocus()
    fireEvent.click(retry)
    await screen.findByRole('heading', { name: 'Rebuild running' })
    expect(boundary.rebuild).toHaveBeenCalledTimes(1)
    expect(boundary.success).not.toHaveBeenCalled()
  })

  it('recovers a failed status query when backend deduplication returns the same task ID', async () => {
    window.localStorage.setItem(
      ROUTE_REBUILD_STORAGE_KEY,
      JSON.stringify({ taskId: 'rebuild-1', refreshModels: false })
    )
    boundary.getTask.mockRejectedValue(new Error('not found or disconnected'))
    boundary.rebuild.mockResolvedValue(queued(true))
    await renderPage()
    await screen.findByRole('heading', {
      name: 'Task status is temporarily unavailable',
    })
    boundary.getTask.mockResolvedValue(task('running'))

    fireEvent.click(
      screen.getByRole('button', { name: 'Recover or restart rebuild' })
    )

    await screen.findByRole('heading', { name: 'Rebuild running' })
    expect(boundary.rebuild).toHaveBeenCalledWith(false, false)
    expect(boundary.info).toHaveBeenCalledWith(
      'Reconnected to an existing rebuild task.'
    )
    expect(window.localStorage.getItem(ROUTE_REBUILD_STORAGE_KEY)).toContain(
      'rebuild-1'
    )
  })

  it('treats a lost launch response as unknown and can recover the already-running task', async () => {
    boundary.rebuild
      .mockRejectedValueOnce(new Error('response lost'))
      .mockResolvedValue(queued(true))
    await renderPage()
    await startRebuild()

    await screen.findByRole('heading', {
      name: 'Rebuild request could not be confirmed',
    })
    expect(boundary.getTask).not.toHaveBeenCalled()
    expect(
      screen.queryByRole('heading', { name: 'Rebuild failed' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('heading', { name: 'Rebuild completed' })
    ).not.toBeInTheDocument()
    expect(boundary.success).not.toHaveBeenCalled()
    fireEvent.click(
      screen.getByRole('button', { name: 'Retry / recover request' })
    )

    await screen.findByRole('heading', { name: 'Rebuild running' })
    expect(boundary.rebuild.mock.calls).toEqual([
      [true, false],
      [true, false],
    ])
  })

  it('does not consume another task type or a missing result as successful rebuild evidence', async () => {
    window.localStorage.setItem(
      ROUTE_REBUILD_STORAGE_KEY,
      JSON.stringify({ taskId: 'rebuild-1', refreshModels: true })
    )
    boundary.getTask.mockResolvedValue({
      success: true,
      task: {
        ...task('succeeded', completeResult).task,
        type: 'database-migration',
      },
    })
    await renderPage()
    await screen.findByRole('heading', {
      name: 'Task status is temporarily unavailable',
    })
    expect(
      screen.queryByRole('heading', { name: 'Rebuild completed' })
    ).not.toBeInTheDocument()

    boundary.getTask.mockResolvedValue(task('succeeded'))
    fireEvent.click(screen.getByRole('button', { name: 'Retry status check' }))
    await waitFor(() => expect(boundary.getTask).toHaveBeenCalledTimes(2))
    expect(
      screen.queryByRole('heading', { name: 'Rebuild completed' })
    ).not.toBeInTheDocument()
    expect(boundary.success).not.toHaveBeenCalled()
  })

  it('dismisses a verified terminal result and clears its reload reference', async () => {
    window.localStorage.setItem(
      ROUTE_REBUILD_STORAGE_KEY,
      JSON.stringify({ taskId: 'rebuild-1', refreshModels: true })
    )
    boundary.getTask.mockResolvedValue(task('succeeded', completeResult))
    await renderPage()
    await screen.findByRole('heading', { name: 'Rebuild completed' })

    fireEvent.click(screen.getByRole('button', { name: 'Dismiss result' }))

    expect(window.localStorage.getItem(ROUTE_REBUILD_STORAGE_KEY)).toBeNull()
    expect(
      screen.queryByRole('heading', { name: 'Rebuild completed' })
    ).not.toBeInTheDocument()
  })

  it('keeps a launch accepted after navigation and observes it on return', async () => {
    let acceptLaunch!: (value: ReturnType<typeof queued>) => void
    boundary.rebuild.mockReturnValue(
      new Promise((resolve) => {
        acceptLaunch = resolve
      })
    )
    const { router } = await renderPage()
    await startRebuild()
    await screen.findByRole('heading', {
      name: 'Requesting a background rebuild',
    })
    await act(async () => {
      await router.navigate({ to: '/' })
    })
    await screen.findByText('Other page')
    await act(async () => {
      acceptLaunch(queued())
    })
    await waitFor(() =>
      expect(window.localStorage.getItem(ROUTE_REBUILD_STORAGE_KEY)).toContain(
        'rebuild-1'
      )
    )

    await act(async () => {
      await router.navigate({ to: '/token-routes' })
    })

    await screen.findByRole('heading', { name: 'Rebuild running' })
    expect(boundary.rebuild).toHaveBeenCalledTimes(1)
  })

  it.each([
    {
      language: 'en',
      label: 'Automatic route creation settings',
      description: /when automatic creation is enabled/,
      oldClaim: 'it does not create routes',
    },
    {
      language: 'zhCN',
      label: '模型路由自动创建设置',
      description: /开启自动创建后/,
      oldClaim: '不会创建路由',
    },
  ])(
    'links clearly to Scheduling with accurate optional-mode guidance in $language',
    async ({ language, label, description, oldClaim }) => {
      await i18n.changeLanguage(language)
      await renderPage()

      const settings = screen.getByRole('link', { name: label })
      expect(settings).toHaveAttribute(
        'href',
        '/settings/operations/scheduling'
      )
      act(() => settings.focus())
      expect(settings).toHaveFocus()
      expect(await screen.findByText(description)).toBeInTheDocument()
      expect(screen.queryByText(new RegExp(oldClaim))).not.toBeInTheDocument()
    }
  )

  it('does not substitute zeroes for incomplete task result counts', async () => {
    window.localStorage.setItem(
      ROUTE_REBUILD_STORAGE_KEY,
      JSON.stringify({ taskId: 'rebuild-1', refreshModels: true })
    )
    boundary.getTask.mockResolvedValue(task('succeeded', { success: true }))
    await renderPage()

    await screen.findByRole('heading', {
      name: 'Task status is temporarily unavailable',
    })
    expect(
      screen.queryByRole('heading', { name: 'Rebuild completed' })
    ).not.toBeInTheDocument()
    expect(screen.queryByText('Routes created')).not.toBeInTheDocument()
  })

  it('keeps the mobile rebuild menu disabled during background work and exposes accessible recovery actions', async () => {
    window.localStorage.setItem(
      ROUTE_REBUILD_STORAGE_KEY,
      JSON.stringify({ taskId: 'rebuild-1', refreshModels: true })
    )
    await renderPage()
    await screen.findByRole('heading', { name: 'Rebuild running' })
    fireEvent.click(screen.getByRole('button', { name: 'More actions' }))
    expect(
      await screen.findByRole('menuitem', { name: 'Auto-rebuild' })
    ).toHaveAttribute('aria-disabled', 'true')
    fireEvent.keyDown(screen.getByRole('menu'), { key: 'Escape' })
    await waitFor(() =>
      expect(screen.queryByRole('menu')).not.toBeInTheDocument()
    )
    boundary.getTask.mockRejectedValue(new Error('offline'))
    await pollTask()

    const recovery = await screen.findByRole('region', {
      name: 'Task status is temporarily unavailable',
    })
    const result = await axe.run(recovery, {
      rules: { 'color-contrast': { enabled: false } },
    })
    expect(result.violations).toEqual([])
    const retry = within(recovery).getByRole('button', {
      name: 'Retry status check',
    })
    act(() => retry.focus())
    expect(retry).toHaveFocus()
  })
})
