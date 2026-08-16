import '@testing-library/jest-dom/vitest'
import { cleanup, render, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { AccountsPage } from '../components/accounts-page'

const testState = vi.hoisted(() => ({
  search: { page: 1, pageSize: 20, create: true, siteId: 7 },
  snapshot: {
    generatedAt: '',
    accounts: [],
    sites: [
      {
        id: 7,
        name: 'Primary site',
        url: 'https://primary.example',
        platform: 'openai',
        status: 'active',
      },
    ],
  },
  navigate: vi.fn(),
  formOpenCount: 0,
  formSiteIds: [] as Array<number | undefined>,
  previousFormOpen: undefined as boolean | undefined,
}))

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => testState.navigate,
  useSearch: () => testState.search,
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
  useAccounts: () => ({
    data: testState.snapshot,
    isLoading: false,
    isFetching: false,
    error: null,
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
  AccountFormDialog: (props: { open: boolean; initialSiteId?: number }) => {
    if (props.open && testState.previousFormOpen !== true) {
      testState.formOpenCount += 1
      testState.formSiteIds.push(props.initialSiteId)
    }
    testState.previousFormOpen = props.open
    return null
  },
}))

vi.mock('../components/accounts-columns', () => ({
  useAccountsColumns: () => [],
}))

afterEach(() => cleanup())

beforeEach(() => {
  testState.navigate.mockReset()
  testState.formOpenCount = 0
  testState.formSiteIds.length = 0
  testState.previousFormOpen = undefined
})

describe('AccountsPage site create deep link', () => {
  it('opens once with the referenced site and clears transient parameters', async () => {
    const { rerender } = render(<AccountsPage />)

    await waitFor(() => {
      expect(testState.formOpenCount).toBe(1)
    })

    rerender(<AccountsPage />)

    await waitFor(() => {
      expect(testState.formOpenCount).toBe(1)
    })
    expect(testState.formSiteIds).toEqual([7])
    expect(testState.navigate).toHaveBeenCalledTimes(1)
    expect(testState.navigate).toHaveBeenCalledWith(
      expect.objectContaining({
        to: '/accounts',
        replace: true,
        search: expect.objectContaining({
          siteId: undefined,
          create: undefined,
        }),
      })
    )
  })
})
