// Page-level wiring test for the OAuth detail sheet (issue #887 S4).
//
// Covers the two seams the sheet's own unit tests cannot see:
//   1. the row action menu's "view details" callback actually opens the sheet
//      with the row it was given, and
//   2. the sheet footer reuses the PAGE's mutations (a second mutation
//      instance would toast twice and skip the page's cache invalidation).
//
// The data-table and the columns hook are stubbed so the test drives the page
// callbacks directly instead of rendering the whole table + dropdown stack;
// the real `OAuthDetailSheet` renders so the assertions stay user-visible.

import '@testing-library/jest-dom/vitest'
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
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
import type { OAuthColumnActions } from '../oauth-columns'
import { OAuthPage } from '../oauth-page'

const connection = {
  accountId: 42,
  siteId: 7,
  provider: 'codex',
  username: 'ops@example.com',
  email: null,
  accountKey: null,
  projectId: 'proj-9001',
  planType: 'pro',
  modelCount: 2,
  modelsPreview: ['gpt-5', 'gpt-5-mini'],
  status: 'healthy',
  routeChannelCount: 3,
  quota: {
    status: 'supported',
    source: 'official',
    windows: {
      fiveHour: { supported: true, used: 12, limit: 50, remaining: 38 },
      sevenDay: { supported: false },
    },
  },
} as unknown as OAuthClient

const pageState = vi.hoisted(() => ({
  capturedActions: null as OAuthColumnActions | null,
  refreshMutate: vi.fn(),
  rebindMutate: vi.fn(),
}))

vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

vi.mock('../../api', () => ({
  useOAuthConnections: () => ({
    data: { items: [connection], total: 1 },
    isLoading: false,
    isFetching: false,
    error: null,
  }),
  useDeleteOAuthConnection: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useRefreshOAuthQuota: () => ({
    mutate: pageState.refreshMutate,
    isPending: false,
    variables: undefined,
  }),
  useRebindOAuthConnection: () => ({
    mutate: pageState.rebindMutate,
    isPending: false,
    variables: undefined,
  }),
}))

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
  pageState.capturedActions = null
  pageState.refreshMutate.mockReset()
  pageState.rebindMutate.mockReset()
})

afterEach(() => cleanup())

/** Render the page and open the detail sheet through the row action. */
function openDetailSheet() {
  render(<OAuthPage />)
  const actions = pageState.capturedActions
  if (!actions) throw new Error('column actions were not captured')
  act(() => actions.onViewDetails(connection))
}

describe('OAuthPage detail sheet wiring', () => {
  it('keeps the sheet closed until the row action asks for it', () => {
    render(<OAuthPage />)

    expect(screen.queryByText('Overview')).not.toBeInTheDocument()
  })

  it('opens the sheet with the row the action was given', () => {
    openDetailSheet()

    expect(screen.getByText('Overview')).toBeInTheDocument()
    expect(
      screen.getByRole('heading', { name: 'ops@example.com' })
    ).toBeInTheDocument()
    expect(screen.getByText('proj-9001')).toBeInTheDocument()
    // The quota `remaining` value only exists in the sheet, not the list.
    expect(screen.getByText('38')).toBeInTheDocument()
  })

  it('routes the footer refresh-quota action into the page mutation', () => {
    openDetailSheet()

    fireEvent.click(screen.getByRole('button', { name: 'Refresh quota' }))

    expect(pageState.refreshMutate).toHaveBeenCalledTimes(1)
    expect(pageState.refreshMutate.mock.calls[0][0]).toBe(42)
  })

  it('routes the footer rebind action into the page mutation', () => {
    openDetailSheet()

    fireEvent.click(screen.getByRole('button', { name: 'Rebind' }))

    expect(pageState.rebindMutate).toHaveBeenCalledTimes(1)
    expect(pageState.rebindMutate.mock.calls[0][0]).toBe(42)
  })
})
