import '@testing-library/jest-dom/vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { AccountsPage } from '../components/accounts-page'
import type { Account } from '../types'

// Pins the fresh-site → accounts-page race (E-line handover): right after
// creating a site the snapshot is still fetching, and the Add account
// button used to be pinned disabled by `sites.length === 0` against the
// not-yet-loaded snapshot. "Still loading" and "genuinely empty library"
// are distinct states and only the latter may disable the button.

const testState = vi.hoisted(() => ({
  accountsQuery: {
    data: undefined as
      | {
          generatedAt: string
          accounts: Array<Partial<Account>>
          sites: Array<{ id: number; name: string; url: string }>
        }
      | undefined,
    isLoading: true,
    isFetching: true,
    error: null as Error | null,
  },
  formOpen: false,
}))

vi.mock('@/components/common/use-probe-history', () => ({
  useProbeHistory: () => ({ data: undefined }),
}))
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => vi.fn(),
  useSearch: () => ({ page: 1, pageSize: 20 }),
}))

vi.mock('@/components/data-table', () => ({
  DataTableBulkActions: () => null,
  DataTablePage: () => null,
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
  AccountFormDialog: (props: { open: boolean }) => {
    testState.formOpen = props.open
    return null
  },
}))

vi.mock('../components/accounts-columns', () => ({
  useAccountsColumns: () => [],
}))

afterEach(() => cleanup())

beforeEach(() => {
  testState.accountsQuery = {
    data: undefined,
    isLoading: true,
    isFetching: true,
    error: null,
  }
  testState.formOpen = false
})

const ADD_BUTTON_NAME = 'Add account'

describe('AccountsPage Add account button while the snapshot loads', () => {
  it('stays enabled while the snapshot is still loading', () => {
    render(<AccountsPage />)

    expect(
      screen.getByRole('button', { name: ADD_BUTTON_NAME })
    ).not.toBeDisabled()
  })

  it('opens the create dialog when clicked during loading', () => {
    render(<AccountsPage />)

    fireEvent.click(screen.getByRole('button', { name: ADD_BUTTON_NAME }))

    expect(testState.formOpen).toBe(true)
  })

  it('is disabled once the snapshot loaded with an empty site library', () => {
    testState.accountsQuery = {
      data: { generatedAt: '', accounts: [], sites: [] },
      isLoading: false,
      isFetching: false,
      error: null,
    }

    render(<AccountsPage />)

    expect(screen.getByRole('button', { name: ADD_BUTTON_NAME })).toBeDisabled()
  })

  it('is enabled once the snapshot loaded with at least one site', () => {
    testState.accountsQuery = {
      data: {
        generatedAt: '',
        accounts: [],
        sites: [{ id: 1, name: 'Primary', url: 'https://primary.example' }],
      },
      isLoading: false,
      isFetching: false,
      error: null,
    }

    render(<AccountsPage />)

    expect(
      screen.getByRole('button', { name: ADD_BUTTON_NAME })
    ).not.toBeDisabled()
  })
})
