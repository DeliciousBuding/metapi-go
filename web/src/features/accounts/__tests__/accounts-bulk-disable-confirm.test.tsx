// Regression test: the bulk "Disable" action fired immediately on click; it
// must now open a confirmation dialog first (audit #1029 batch B) — matching
// the destructive bulk-delete guard already in place.
import '@testing-library/jest-dom/vitest'
import type { ReactNode } from 'react'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { AccountsPage } from '../components/accounts-page'

const testState = vi.hoisted(() => ({
  navigate: vi.fn(),
  batchMutateAsync: vi.fn(),
}))

vi.mock('@/components/common/use-probe-history', () => ({
  useProbeHistory: () => ({ data: undefined }),
}))
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => testState.navigate,
  useSearch: () => ({ page: 1, pageSize: 20 }),
}))

vi.mock('@/components/data-table', () => ({
  // Render the bulk action children so the Disable button is clickable.
  DataTableBulkActions: (props: { children?: ReactNode }) => (
    <div>{props.children}</div>
  ),
  DataTablePage: (props: { bulkActions?: ReactNode }) => (
    <div>{props.bulkActions}</div>
  ),
  useUrlTableState: () => ({
    globalFilter: '',
    onGlobalFilterChange: vi.fn(),
    columnFilters: [],
    onColumnFiltersChange: vi.fn(),
    pagination: { pageIndex: 0, pageSize: 20 },
    onPaginationChange: vi.fn(),
    ensurePageInRange: vi.fn(),
  }),
  useDataTable: () => ({
    table: {
      // One selected account so the bulk action proceeds.
      getFilteredSelectedRowModel: () => ({ rows: [{ original: { id: 5 } }] }),
      resetRowSelection: vi.fn(),
    },
  }),
}))

vi.mock('@/features/import', () => ({
  ImportWizardDialog: () => null,
}))

vi.mock('../api', () => ({
  useAccounts: () => ({
    data: { accounts: [], sites: [] },
    error: null,
    isLoading: false,
    isFetching: false,
  }),
  useBatchUpdateAccounts: () => ({
    mutateAsync: testState.batchMutateAsync,
    isPending: false,
  }),
  useDeleteAccount: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useRefreshAccount: () => ({ mutate: vi.fn(), isPending: false }),
  useToggleAccountCheckin: () => ({ mutate: vi.fn(), isPending: false }),
  useToggleAccountPin: () => ({ mutate: vi.fn(), isPending: false }),
  useToggleAccountStatus: () => ({ mutate: vi.fn(), isPending: false }),
}))

vi.mock('../components/account-detail-sheet', () => ({
  AccountDetailSheet: () => null,
}))

vi.mock('../components/account-form-dialog', () => ({
  AccountFormDialog: () => null,
}))

vi.mock('../components/accounts-columns', () => ({
  useAccountsColumns: () => [],
}))

beforeAll(() => {
  // base-ui AlertDialog queries matchMedia under jsdom.
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
  testState.navigate.mockReset()
  testState.batchMutateAsync.mockReset()
})

afterEach(() => cleanup())

// The dialog action shares its label with the trigger; the action button is
// the last matching button in the tree once the dialog is open.
function clickLastButtonNamed(name: string) {
  const buttons = screen.getAllByRole('button', { name })
  const button = buttons.at(-1)
  if (!button) {
    throw new Error(`Button "${name}" not found`)
  }
  fireEvent.click(button)
}

describe('AccountsPage bulk disable confirmation', () => {
  it('opens a confirmation before bulk disabling', () => {
    render(<AccountsPage />)

    fireEvent.click(screen.getByRole('button', { name: 'Disable' }))

    expect(screen.getByText('Disable selected accounts?')).toBeInTheDocument()
    expect(testState.batchMutateAsync).not.toHaveBeenCalled()
  })

  it('bulk disables after confirming', async () => {
    render(<AccountsPage />)

    fireEvent.click(screen.getByRole('button', { name: 'Disable' }))
    clickLastButtonNamed('Disable')

    await waitFor(() => {
      expect(testState.batchMutateAsync).toHaveBeenCalledTimes(1)
    })
    expect(testState.batchMutateAsync).toHaveBeenCalledWith({
      ids: [5],
      action: 'disable',
    })
  })

  it('does not bulk disable when the confirmation is cancelled', () => {
    render(<AccountsPage />)

    fireEvent.click(screen.getByRole('button', { name: 'Disable' }))
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))

    expect(testState.batchMutateAsync).not.toHaveBeenCalled()
  })
})
