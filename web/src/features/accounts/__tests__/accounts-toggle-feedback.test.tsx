// Behavior test for account pin/check-in toggle feedback (2026-08-18
// multi-perspective review: dropdown-driven pin/check-in toggles were
// fire-and-forget with no confirmation). The page wires success toasts at
// the call site because only the row knows the account name. Captures the
// memoized row actions via the columns hook mock and drives the mutations
// through stubbed hooks that invoke the caller's onSuccess.

import '@testing-library/jest-dom/vitest'
import { cleanup, render, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { AccountsPage } from '../components/accounts-page'
import type { Account, AccountRowActions } from '../types'

const testState = vi.hoisted(() => ({
  rowActions: null as AccountRowActions | null,
  pinMutate: vi.fn(),
  checkinMutate: vi.fn(),
  toastSuccess: vi.fn(),
}))

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => vi.fn(),
  useSearch: () => ({}),
}))

vi.mock('@/components/data-table', () => ({
  DataTableBulkActions: () => null,
  DataTablePage: () => null,
  encodeSorting: () => '',
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

vi.mock('@/lib/toast', () => ({
  toast: {
    success: (...args: unknown[]) => testState.toastSuccess(...args),
    error: vi.fn(),
    warning: vi.fn(),
    info: vi.fn(),
  },
}))

vi.mock('../api', () => ({
  useAccounts: () => ({
    data: { generatedAt: '', accounts: [], sites: [] },
    isLoading: false,
    isFetching: false,
    error: null,
  }),
  useBatchUpdateAccounts: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useDeleteAccount: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useRefreshAccount: () => ({ mutate: vi.fn(), isPending: false }),
  useToggleAccountCheckin: () => ({
    mutate: testState.checkinMutate,
    isPending: false,
  }),
  useToggleAccountPin: () => ({
    mutate: testState.pinMutate,
    isPending: false,
  }),
  useToggleAccountStatus: () => ({
    mutate: vi.fn(),
    isPending: false,
    variables: undefined,
  }),
}))

vi.mock('../components/account-detail-sheet', () => ({
  AccountDetailSheet: () => null,
}))

vi.mock('../components/account-form-dialog', () => ({
  AccountFormDialog: () => null,
}))

vi.mock('../components/accounts-columns', () => ({
  useAccountsColumns: (actions: AccountRowActions) => {
    testState.rowActions = actions
    return []
  },
}))

function makeAccount(overrides: Partial<Account>): Account {
  return {
    id: 9,
    siteId: 1,
    username: 'alpha-account',
    credentialMode: 'session',
    status: 'active',
    isPinned: false,
    checkinEnabled: false,
    ...overrides,
  } as Account
}

beforeEach(() => {
  testState.rowActions = null
  testState.pinMutate.mockReset()
  testState.checkinMutate.mockReset()
  testState.toastSuccess.mockReset()
  // Default stubs: succeed synchronously and invoke the caller's callback.
  testState.pinMutate.mockImplementation(
    (_variables: unknown, options?: { onSuccess?: () => void }) => {
      options?.onSuccess?.()
    }
  )
  testState.checkinMutate.mockImplementation(
    (_variables: unknown, options?: { onSuccess?: () => void }) => {
      options?.onSuccess?.()
    }
  )
})

afterEach(() => cleanup())

describe('AccountsPage toggle feedback', () => {
  it('confirms a successful pin toggle with the account name', async () => {
    render(<AccountsPage />)

    await waitFor(() => expect(testState.rowActions).not.toBeNull())
    testState.rowActions?.onTogglePin(makeAccount({ isPinned: false }))

    expect(testState.pinMutate).toHaveBeenCalledWith(
      { id: 9, isPinned: true },
      expect.objectContaining({ onSuccess: expect.any(Function) })
    )
    expect(testState.toastSuccess).toHaveBeenCalledTimes(1)
    expect(testState.toastSuccess.mock.calls[0][0]).toContain('alpha-account')
  })

  it('confirms a successful check-in toggle with the account name', async () => {
    render(<AccountsPage />)

    await waitFor(() => expect(testState.rowActions).not.toBeNull())
    testState.rowActions?.onToggleCheckin(
      makeAccount({ checkinEnabled: false })
    )

    expect(testState.checkinMutate).toHaveBeenCalledWith(
      { id: 9, checkinEnabled: true },
      expect.objectContaining({ onSuccess: expect.any(Function) })
    )
    expect(testState.toastSuccess).toHaveBeenCalledTimes(1)
    expect(testState.toastSuccess.mock.calls[0][0]).toContain('alpha-account')
  })

  it('stays silent when the toggle mutation does not succeed', async () => {
    testState.pinMutate.mockImplementation(() => {
      // Network/business failure: TanStack Query would call onError instead.
    })

    render(<AccountsPage />)

    await waitFor(() => expect(testState.rowActions).not.toBeNull())
    testState.rowActions?.onTogglePin(makeAccount({}))

    expect(testState.toastSuccess).not.toHaveBeenCalled()
  })
})
