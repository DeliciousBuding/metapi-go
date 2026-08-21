// Behavior tests for the per-row pending state + error-toast wiring on the
// OAuth connections table. Mirrors the accounts-columns test style: the cell
// component (`OAuthRowActions`) is rendered in isolation with mocked actions
// so the assertions stay about user-visible behavior (accessible name, click
// payload, per-row disabled state, spinner). A second describe block tests
// the page-level error-toast wiring (page callback → mutation onError →
// toast.error with the account display name) by capturing the actions the
// page hands to `useOAuthColumns` and driving the mock mutation's onError.
import '@testing-library/jest-dom/vitest'
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

import type { OAuthClient } from '../../types'
import { OAuthRowActions, type OAuthColumnActions } from '../oauth-columns'
import { OAuthPage } from '../oauth-page'

// ---------------------------------------------------------------------------
// Shared jsdom stubs (Base UI dropdown positioning needs both)
// ---------------------------------------------------------------------------

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

afterEach(() => cleanup())

// ---------------------------------------------------------------------------
// Test fixtures
// ---------------------------------------------------------------------------

const baseClient = {
  accountId: 42,
  siteId: 1,
  provider: 'openai',
  username: 'smoke-user',
  email: null,
  accountKey: null,
  modelCount: 3,
  modelsPreview: ['gpt-4o', 'gpt-4o-mini', 'o1'],
  status: 'healthy' as const,
} as unknown as OAuthClient

function buildActions(): OAuthColumnActions {
  return {
    onViewDetails: vi.fn(),
    onRefreshQuota: vi.fn(),
    onRebind: vi.fn(),
    onDelete: vi.fn(),
  }
}

// ---------------------------------------------------------------------------
// Cell-level: OAuthRowActions per-row pending behavior
// ---------------------------------------------------------------------------

describe('OAuthRowActions per-row pending', () => {
  it('renders the dropdown trigger with a localized aria-label', () => {
    render(
      <OAuthRowActions
        client={baseClient}
        actions={buildActions()}
        pendingAccountId={null}
      />
    )

    expect(
      screen.getByRole('button', { name: 'Row actions' })
    ).toBeInTheDocument()
  })

  it('calls onViewDetails with the client when the view-details item is clicked', async () => {
    const actions = buildActions()
    render(
      <OAuthRowActions
        client={baseClient}
        actions={actions}
        pendingAccountId={null}
      />
    )

    fireEvent.click(screen.getByRole('button', { name: 'Row actions' }))
    const viewItem = await screen.findByRole('menuitem', {
      name: 'View details',
    })
    fireEvent.click(viewItem)

    expect(actions.onViewDetails).toHaveBeenCalledTimes(1)
    expect(actions.onViewDetails).toHaveBeenCalledWith(baseClient)
  })

  it('keeps view-details clickable while the row has a mutation in flight', async () => {
    render(
      <OAuthRowActions
        client={baseClient}
        actions={buildActions()}
        pendingAccountId={42}
      />
    )

    fireEvent.click(screen.getByRole('button', { name: 'Row actions' }))
    const viewItem = await screen.findByRole('menuitem', {
      name: 'View details',
    })

    // Opening a read-only panel cannot conflict with an in-flight request.
    expect(viewItem).not.toHaveAttribute('data-disabled')
  })

  it('calls onRefreshQuota with the client when the refresh item is clicked', async () => {
    const actions = buildActions()
    render(
      <OAuthRowActions
        client={baseClient}
        actions={actions}
        pendingAccountId={null}
      />
    )

    fireEvent.click(screen.getByRole('button', { name: 'Row actions' }))
    const refreshItem = await screen.findByRole('menuitem', {
      name: 'Refresh quota',
    })
    fireEvent.click(refreshItem)

    expect(actions.onRefreshQuota).toHaveBeenCalledTimes(1)
    expect(actions.onRefreshQuota).toHaveBeenCalledWith(baseClient)
  })

  it('calls onRebind with the client when the rebind item is clicked', async () => {
    const actions = buildActions()
    render(
      <OAuthRowActions
        client={baseClient}
        actions={actions}
        pendingAccountId={null}
      />
    )

    fireEvent.click(screen.getByRole('button', { name: 'Row actions' }))
    const rebindItem = await screen.findByRole('menuitem', {
      name: 'Rebind',
    })
    fireEvent.click(rebindItem)

    expect(actions.onRebind).toHaveBeenCalledTimes(1)
    expect(actions.onRebind).toHaveBeenCalledWith(baseClient)
  })

  it('shows a spinner on the trigger and disables refresh/rebind items for the pending row', async () => {
    render(
      <OAuthRowActions
        client={baseClient}
        actions={buildActions()}
        pendingAccountId={42}
      />
    )

    const trigger = screen.getByRole('button', { name: 'Row actions' })
    // The trigger swaps MoreHorizontal for a spinning Spinner.
    expect(trigger.querySelector('svg.animate-spin')).toBeInTheDocument()

    fireEvent.click(trigger)
    const refreshItem = await screen.findByRole('menuitem', {
      name: 'Refresh quota',
    })
    const rebindItem = screen.getByRole('menuitem', { name: 'Rebind' })

    // Both refresh + rebind are disabled for the row whose accountId matches
    // pendingAccountId (Base UI sets data-disabled + pointer-events:none).
    expect(refreshItem).toHaveAttribute('data-disabled')
    expect(rebindItem).toHaveAttribute('data-disabled')
  })

  it('leaves refresh/rebind enabled and shows no spinner for a non-pending row (no global lock)', async () => {
    render(
      <OAuthRowActions
        client={baseClient}
        actions={buildActions()}
        pendingAccountId={999}
      />
    )

    const trigger = screen.getByRole('button', { name: 'Row actions' })
    // No spinner — a different row is pending, not this one.
    expect(trigger.querySelector('svg.animate-spin')).not.toBeInTheDocument()

    fireEvent.click(trigger)
    const refreshItem = await screen.findByRole('menuitem', {
      name: 'Refresh quota',
    })
    const rebindItem = screen.getByRole('menuitem', { name: 'Rebind' })

    expect(refreshItem).not.toHaveAttribute('data-disabled')
    expect(rebindItem).not.toHaveAttribute('data-disabled')
  })

  it('keeps the delete item enabled while the row is pending', async () => {
    render(
      <OAuthRowActions
        client={baseClient}
        actions={buildActions()}
        pendingAccountId={42}
      />
    )

    fireEvent.click(screen.getByRole('button', { name: 'Row actions' }))
    const deleteItem = await screen.findByRole('menuitem', {
      name: 'Delete',
    })

    expect(deleteItem).not.toHaveAttribute('data-disabled')
  })
})

// ---------------------------------------------------------------------------
// Page-level: refresh/rebind failure toast wiring
// ---------------------------------------------------------------------------

// The hoisted state object captures the actions the page hands to
// `useOAuthColumns` and the mutation callbacks the page passes to
// `refreshQuota.mutate` / `rebindConnection.mutate`. This lets the test drive
// the real page callback wiring (onRefreshQuota → mutate → onError →
// toast.error) without rendering the full data-table + dropdown stack.
const pageState = vi.hoisted(() => ({
  capturedActions: null as OAuthColumnActions | null,
  refreshOnError: null as ((error: Error) => void) | null,
  rebindOnError: null as ((error: Error) => void) | null,
  toastError: vi.fn(),
  toastSuccess: vi.fn(),
}))

vi.mock('@/lib/toast', () => ({
  toast: {
    success: pageState.toastSuccess,
    error: pageState.toastError,
  },
}))

vi.mock('../../api', () => ({
  useOAuthConnections: () => ({
    data: [],
    isLoading: false,
    isFetching: false,
    error: null,
  }),
  useDeleteOAuthConnection: () => ({
    mutateAsync: vi.fn(),
    isPending: false,
  }),
  useRefreshOAuthQuota: () => ({
    mutate: (
      _accountId: number,
      options?: {
        onSuccess?: () => void
        onError?: (error: Error) => void
      }
    ) => {
      pageState.refreshOnError = options?.onError ?? null
    },
    isPending: false,
    variables: undefined,
  }),
  useRebindOAuthConnection: () => ({
    mutate: (
      _accountId: number,
      options?: {
        onSuccess?: () => void
        onError?: (error: Error) => void
      }
    ) => {
      pageState.rebindOnError = options?.onError ?? null
    },
    isPending: false,
    variables: undefined,
  }),
}))

// Partial mock: override only `useOAuthColumns` so the page-level tests can
// capture the actions object the page wires (onRefreshQuota / onRebind /
// onError). The real `OAuthRowActions` export is preserved via
// `importOriginal` so the cell-level tests render the genuine component.
vi.mock('../oauth-columns', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../oauth-columns')>()
  return {
    ...actual,
    useOAuthColumns: (actions: OAuthColumnActions) => {
      pageState.capturedActions = actions
      return []
    },
  }
})

vi.mock('../oauth-start-dialog', () => ({
  OAuthStartDialog: () => null,
}))

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => vi.fn(),
  useSearch: () => ({}),
}))

vi.mock('@/components/data-table', () => ({
  DataTablePage: () => null,
  encodeSorting: () => '',
  useDataTable: () => ({ table: {} }),
  useUrlTableState: () => ({
    globalFilter: '',
    onGlobalFilterChange: vi.fn(),
    columnFilters: [],
    onColumnFiltersChange: vi.fn(),
    pagination: { pageIndex: 0, pageSize: 20 },
    onPaginationChange: vi.fn(),
    sorting: [],
    onSortingChange: vi.fn(),
    ensurePageInRange: vi.fn(),
  }),
}))

beforeEach(() => {
  pageState.capturedActions = null
  pageState.refreshOnError = null
  pageState.rebindOnError = null
  pageState.toastError.mockReset()
  pageState.toastSuccess.mockReset()
})

describe('OAuthPage refresh/rebind failure toast', () => {
  it('toasts with the account username when refresh quota fails', async () => {
    render(<OAuthPage />)

    await waitFor(() => expect(pageState.capturedActions).not.toBeNull())
    const actions = pageState.capturedActions
    if (!actions) throw new Error('actions not captured')

    actions.onRefreshQuota(baseClient)

    expect(pageState.refreshOnError).not.toBeNull()
    if (!pageState.refreshOnError) throw new Error('onError not captured')
    pageState.refreshOnError(new Error('upstream 503'))

    expect(pageState.toastError).toHaveBeenCalledTimes(1)
    const toasted = pageState.toastError.mock.calls[0][0] as string
    expect(toasted).toContain('smoke-user')
  })

  it('falls back to the account id when username/email/key are absent', async () => {
    const anonymousClient = {
      ...baseClient,
      username: null,
      email: null,
      accountKey: null,
    } as unknown as OAuthClient

    render(<OAuthPage />)

    await waitFor(() => expect(pageState.capturedActions).not.toBeNull())
    const actions = pageState.capturedActions
    if (!actions) throw new Error('actions not captured')

    actions.onRefreshQuota(anonymousClient)

    expect(pageState.refreshOnError).not.toBeNull()
    if (!pageState.refreshOnError) throw new Error('onError not captured')
    pageState.refreshOnError(new Error('upstream 503'))

    expect(pageState.toastError).toHaveBeenCalledTimes(1)
    const toasted = pageState.toastError.mock.calls[0][0] as string
    expect(toasted).toContain('42')
  })

  it('toasts with the account username when rebind fails', async () => {
    render(<OAuthPage />)

    await waitFor(() => expect(pageState.capturedActions).not.toBeNull())
    const actions = pageState.capturedActions
    if (!actions) throw new Error('actions not captured')

    actions.onRebind(baseClient)

    expect(pageState.rebindOnError).not.toBeNull()
    if (!pageState.rebindOnError) throw new Error('onError not captured')
    pageState.rebindOnError(new Error('rebind refused'))

    expect(pageState.toastError).toHaveBeenCalledTimes(1)
    const toasted = pageState.toastError.mock.calls[0][0] as string
    expect(toasted).toContain('smoke-user')
  })
})
