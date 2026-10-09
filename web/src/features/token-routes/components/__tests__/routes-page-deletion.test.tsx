import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeAll, beforeEach, expect, it, vi } from 'vitest'

import type { RouteRowActions, RouteSummaryRow } from '../../types'
import { RoutesPage } from '../routes-page'

const state = vi.hoisted(() => ({
  preview: vi.fn(),
  remove: vi.fn(),
  message: vi.fn(),
  error: vi.fn(),
  success: vi.fn(),
  actions: null as RouteRowActions | null,
  route: {
    id: 7,
    modelPattern: 'fixture-model',
    displayName: 'Fixture route',
    displayIcon: null,
    modelMapping: null,
    enabled: true,
    channelCount: 1,
    enabledChannelCount: 1,
    siteNames: [],
    decisionSnapshot: null,
    decisionRefreshedAt: null,
  } as RouteSummaryRow,
}))
vi.mock('@/lib/api', () => ({
  api: {
    previewUpstreamDeletion: state.preview,
    deleteUpstreamEntity: state.remove,
  },
}))
vi.mock('@/lib/toast', () => ({
  toast: {
    message: state.message,
    error: state.error,
    success: state.success,
    dismiss: vi.fn(),
  },
}))
vi.mock('@tanstack/react-router', () => ({
  Link: () => null,
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
vi.mock('../../lib/use-route-rebuild-task', () => ({
  useRouteRebuildTask: () => ({
    reference: null,
    task: undefined,
    isBusy: false,
  }),
}))
vi.mock('../../api', () => ({
  routeQueryKeys: { all: ['routes'], summary: () => ['routes', 'summary'] },
  useRoutes: () => ({
    data: [state.route],
    error: null,
    isLoading: false,
    isFetching: false,
  }),
  useModelTokenCandidates: () => ({ data: undefined }),
  useUpdateRoute: () => ({ mutate: vi.fn(), isPending: false }),
  useClearRouteCooldown: () => ({ mutate: vi.fn(), isPending: false }),
  useRebuildRoutes: () => ({ mutate: vi.fn(), isPending: false }),
  useRefreshRouteDecisions: () => ({ mutate: vi.fn(), isPending: false }),
  useZeroChannelRoutes: (routes: unknown[]) => routes,
}))
vi.mock('@/components/data-table', () => ({
  DataTableBulkActions: () => null,
  DataTablePage: () => (
    <button
      type='button'
      disabled={state.actions?.isDeletePending}
      onClick={() => state.actions?.onDelete(state.route)}
    >
      Delete fixture route
    </button>
  ),
  useUrlTableState: () => ({
    globalFilter: '',
    onGlobalFilterChange: vi.fn(),
    columnFilters: [],
    onColumnFiltersChange: vi.fn(),
    pagination: { pageIndex: 0, pageSize: 20 },
    onPaginationChange: vi.fn(),
    filters: { enabled: '', accountId: '', siteId: '', routeId: '' },
  }),
  useDataTable: () => ({
    table: {
      getFilteredSelectedRowModel: () => ({ rows: [] }),
      resetRowSelection: vi.fn(),
    },
  }),
}))
vi.mock('../routes-columns', () => ({
  useRoutesColumns: (actions: RouteRowActions) => {
    state.actions = actions
    return []
  },
}))
vi.mock('../route-form-dialog', () => ({ RouteFormDialog: () => null }))
vi.mock('../route-detail-sheet', () => ({ RouteDetailSheet: () => null }))
vi.mock('../routes-header-actions', () => ({ RoutesHeaderActions: () => null }))
vi.mock('../routes-key-next-step', () => ({ RoutesKeyNextStep: () => null }))

const leaf = {
  kind: 'route',
  id: 7,
  counts: { routes: 1 },
  affectedRouteIds: [7],
  revision: 'leaf-revision',
  requiresCascade: false,
}
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
  vi.useFakeTimers()
  vi.clearAllMocks()
  state.preview.mockReset().mockResolvedValue(leaf)
  state.remove.mockReset().mockResolvedValue({ ...leaf, success: true })
  state.message.mockReturnValue('undo-toast')
})
afterEach(() => {
  cleanup()
  vi.useRealTimers()
})
function mount() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  client.setQueryData(['routes', 'summary'], [state.route])
  render(
    <QueryClientProvider client={client}>
      <RoutesPage />
    </QueryClientProvider>
  )
  return client
}
async function start() {
  fireEvent.click(screen.getByRole('button', { name: 'Delete fixture route' }))
  await act(async () => {
    await vi.advanceTimersByTimeAsync(0)
  })
}
function undoOptions() {
  return state.message.mock.calls[0][1] as {
    action: { onClick: () => void }
    onAutoClose: () => void
  }
}

it('previews a native leaf, preserves undo, and never deletes when undone', async () => {
  const client = mount()
  await start()
  expect(state.preview).toHaveBeenCalledExactlyOnceWith('route', 7)
  expect(client.getQueryData(['routes', 'summary'])).toEqual([])
  expect(state.remove).not.toHaveBeenCalled()
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  act(() => {
    undoOptions().action.onClick()
  })
  expect(client.getQueryData(['routes', 'summary'])).toEqual([state.route])
  act(() => {
    undoOptions().onAutoClose()
  })
  expect(state.remove).not.toHaveBeenCalled()
})

it('commits a leaf only after the existing undo window closes, with its preview revision', async () => {
  mount()
  await start()
  await act(async () => {
    undoOptions().onAutoClose()
    await vi.advanceTimersByTimeAsync(0)
  })
  expect(state.remove).toHaveBeenCalledExactlyOnceWith('route', 7, {
    cascade: false,
    expectedRevision: 'leaf-revision',
  })
})

it('deletes a route linked to an imported group only after reviewing its closure', async () => {
  state.preview.mockResolvedValue({
    ...leaf,
    requiresCascade: true,
    revision: 'group-revision',
    counts: { routes: 1, groups: 1, members: 3 },
  })
  const client = mount()
  await start()
  expect(state.message).not.toHaveBeenCalled()
  expect(client.getQueryData(['routes', 'summary'])).toEqual([state.route])
  expect(screen.getByText('Groups deleted')).toBeInTheDocument()
  expect(state.remove).not.toHaveBeenCalled()
  fireEvent.change(screen.getByPlaceholderText('DELETE'), {
    target: { value: 'DELETE' },
  })
  for (let i = 0; i < 3; i++) {
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000)
    })
  }
  fireEvent.click(screen.getByRole('button', { name: 'Delete these records' }))
  await act(async () => {
    await vi.advanceTimersByTimeAsync(0)
  })
  expect(state.remove).toHaveBeenCalledExactlyOnceWith('route', 7, {
    cascade: true,
    expectedRevision: 'group-revision',
  })
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
})

it('restores the native row and opens a new review when dependencies appear during undo', async () => {
  state.remove.mockRejectedValue({
    isAxiosError: true,
    response: {
      status: 409,
      data: {
        error: 'changed',
        preview: {
          ...leaf,
          requiresCascade: true,
          revision: 'changed-revision',
          counts: { routes: 1, groups: 1 },
        },
      },
    },
  })
  const client = mount()
  await start()
  await act(async () => {
    undoOptions().onAutoClose()
    await vi.advanceTimersByTimeAsync(0)
  })
  expect(client.getQueryData(['routes', 'summary'])).toEqual([state.route])
  expect(screen.getByRole('alert')).toHaveTextContent('Related records changed')
  expect(state.remove).toHaveBeenCalledOnce()
})
