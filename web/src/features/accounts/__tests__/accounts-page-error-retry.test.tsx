// Regression tests for the accounts page load-error state: the shared
// QueryErrorBanner + Retry button, wired to refetch, in the same shape the
// sites page pins (audit gap-7: accounts/channels load errors had no Retry).
import '@testing-library/jest-dom/vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { AccountsPage } from '../components/accounts-page'

const testState = vi.hoisted(() => ({
  search: { page: 1, pageSize: 20 } as Record<string, unknown>,
  navigate: vi.fn(),
  accountsQuery: {
    data: undefined as { accounts: unknown[]; sites: unknown[] } | undefined,
    error: null as Error | null,
    isLoading: false,
    isFetching: false,
    refetch: vi.fn().mockResolvedValue({}),
  },
}))

vi.mock('@/components/common/use-probe-history', () => ({
  useProbeHistory: () => ({ data: undefined }),
}))
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => testState.navigate,
  useSearch: () => testState.search,
}))

vi.mock('@/components/data-table', () => ({
  DataTableBulkActions: () => null,
  DataTablePage: () => <div data-testid='data-table-page' />,
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
      getFilteredSelectedRowModel: () => ({ rows: [] }),
      resetRowSelection: vi.fn(),
    },
  }),
}))

vi.mock('@/features/import', () => ({
  ImportWizardDialog: () => null,
}))

vi.mock('../api', () => ({
  useAccountsPage: () => testState.accountsQuery,
  useBatchUpdateAccounts: () => ({ mutateAsync: vi.fn(), isPending: false }),
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

beforeEach(() => {
  testState.navigate.mockReset()
  testState.accountsQuery.refetch.mockReset()
  testState.accountsQuery.refetch.mockResolvedValue({})
  testState.accountsQuery.data = {
    accounts: [],
    sites: [{ id: 1, name: 'Primary site' }],
  }
  testState.accountsQuery.error = null
  testState.accountsQuery.isLoading = false
  testState.accountsQuery.isFetching = false
})

afterEach(() => cleanup())

describe('AccountsPage load error state', () => {
  it('renders the error banner with a Retry button when the snapshot fails', () => {
    testState.accountsQuery.error = new Error('boom')

    render(<AccountsPage />)

    expect(
      screen.getByText(/Failed to load accounts: boom/)
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Retry' })).toBeInTheDocument()
  })

  it('renders no error banner or Retry button when the snapshot loads', () => {
    testState.accountsQuery.error = null

    render(<AccountsPage />)

    expect(
      screen.queryByText(/Failed to load accounts/)
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Retry' })
    ).not.toBeInTheDocument()
  })

  it('calls refetch when the Retry button is clicked', () => {
    testState.accountsQuery.error = new Error('boom')

    render(<AccountsPage />)

    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))

    expect(testState.accountsQuery.refetch).toHaveBeenCalledTimes(1)
  })

  it('suppresses the table (and its empty-state CTA) while the load error is present', () => {
    testState.accountsQuery.error = new Error('boom')

    render(<AccountsPage />)

    expect(screen.queryByTestId('data-table-page')).not.toBeInTheDocument()
  })

  it('renders the table when the snapshot loads without error', () => {
    testState.accountsQuery.error = null

    render(<AccountsPage />)

    expect(screen.getByTestId('data-table-page')).toBeInTheDocument()
  })
})
