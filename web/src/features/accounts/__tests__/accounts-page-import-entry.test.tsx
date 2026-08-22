// Pins the import-wizard toolbar entry on the accounts page — mirror of the
// sites page regression: with a non-empty account list the empty-state CTA
// (the only former wizard entry) never renders, so the toolbar keeps a
// permanent Import button reusing the already-mounted setImportOpen.

import '@testing-library/jest-dom/vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { AccountsPage } from '../components/accounts-page'
import type { Account } from '../types'

const testState = vi.hoisted(() => ({
  importOpen: false,
}))

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => vi.fn(),
  useSearch: () => ({ page: 1, pageSize: 20 }),
}))

// Render only the toolbar preActions slot: the regression is about toolbar
// reachability with a NON-EMPTY list, where the empty-state CTA never shows.
vi.mock('@/components/data-table', () => ({
  DataTableBulkActions: () => null,
  DataTablePage: (props: {
    toolbarProps?: { preActions?: ReactNode } | null
  }) => <div>{props.toolbarProps?.preActions}</div>,
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
  ImportWizardDialog: (props: { open: boolean }) => {
    testState.importOpen = props.open
    return null
  },
}))

vi.mock('../api', () => ({
  // Non-empty library: exactly the state where the empty-state CTA disappears.
  useAccounts: () => ({
    data: {
      generatedAt: '',
      accounts: [
        { id: 3, siteId: 7, username: 'account-3', status: 'active' },
      ] as Array<Partial<Account>>,
      sites: [{ id: 7, name: 'Primary site', url: 'https://primary.example' }],
    },
    isLoading: false,
    isFetching: false,
    error: null,
    refetch: vi.fn(),
  }),
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

afterEach(() => cleanup())

beforeEach(() => {
  testState.importOpen = false
})

const IMPORT_BUTTON_NAME = 'Import sites'

describe('AccountsPage toolbar import entry', () => {
  it('renders an Import button in the toolbar when the list is non-empty', () => {
    render(<AccountsPage />)

    expect(
      screen.getByRole('button', { name: IMPORT_BUTTON_NAME })
    ).toBeInTheDocument()
  })

  it('opens the import wizard from the toolbar button', () => {
    render(<AccountsPage />)

    fireEvent.click(screen.getByRole('button', { name: IMPORT_BUTTON_NAME }))

    expect(testState.importOpen).toBe(true)
  })
})
