import '@testing-library/jest-dom/vitest'
import { cleanup, render, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { AccountsPage } from '../components/accounts-page'
import type { Account } from '../types'

const DEFAULT_SEARCH = { page: 1, pageSize: 20, create: true, siteId: 7 }
const DEFAULT_SNAPSHOT = {
  generatedAt: '',
  accounts: [] as Array<Partial<Account>>,
  sites: [
    {
      id: 7,
      name: 'Primary site',
      url: 'https://primary.example',
      platform: 'openai',
      status: 'active',
    },
  ],
}

const testState = vi.hoisted(() => ({
  search: { page: 1, pageSize: 20, create: true, siteId: 7 } as Record<
    string,
    unknown
  >,
  snapshot: {
    generatedAt: '',
    accounts: [] as Array<Partial<Account>>,
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
  detailSheetProps: null as {
    account: Account | null
    open: boolean
  } | null,
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
  AccountDetailSheet: (props: { account: Account | null; open: boolean }) => {
    testState.detailSheetProps = props
    return null
  },
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
  testState.search = { ...DEFAULT_SEARCH }
  testState.snapshot = {
    ...DEFAULT_SNAPSHOT,
    accounts: [...DEFAULT_SNAPSHOT.accounts],
    sites: [...DEFAULT_SNAPSHOT.sites],
  }
  testState.navigate.mockReset()
  testState.formOpenCount = 0
  testState.formSiteIds.length = 0
  testState.previousFormOpen = undefined
  testState.detailSheetProps = null
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

function makeAccount(id: number): Account {
  return {
    id,
    siteId: 7,
    username: `account-${id}`,
    status: 'active',
  } as unknown as Account
}

// Dashboard attention items link `/accounts?accountId=N` for expired /
// low-balance accounts; the page opens the detail sheet once and strips
// the param. Mirrors the channels page's channelId drilldown tests.
describe('AccountsPage account attention deep link', () => {
  it('opens the detail sheet for the referenced account and clears the parameter', async () => {
    testState.search = { page: 1, pageSize: 20, accountId: 3 }
    testState.snapshot.accounts = [makeAccount(3), makeAccount(4)]
    const { rerender } = render(<AccountsPage />)

    await waitFor(() => {
      expect(testState.detailSheetProps?.open).toBe(true)
    })

    rerender(<AccountsPage />)

    await waitFor(() => {
      expect(testState.detailSheetProps?.open).toBe(true)
    })
    expect(testState.detailSheetProps?.account?.id).toBe(3)
    expect(testState.navigate).toHaveBeenCalledTimes(1)
    expect(testState.navigate).toHaveBeenCalledWith(
      expect.objectContaining({
        to: '/accounts',
        replace: true,
        search: expect.objectContaining({ accountId: undefined }),
      })
    )
  })

  it('clears a stale account id silently without opening the sheet', async () => {
    testState.search = { page: 1, pageSize: 20, accountId: 99 }
    testState.snapshot.accounts = [makeAccount(3)]

    render(<AccountsPage />)

    await waitFor(() => {
      expect(testState.navigate).toHaveBeenCalled()
    })
    expect(testState.detailSheetProps?.open).toBe(false)
    expect(testState.detailSheetProps?.account).toBeNull()
  })

  it('does not navigate when no account deep link is present', async () => {
    testState.search = { page: 1, pageSize: 20 }
    testState.snapshot.accounts = [makeAccount(3)]
    testState.snapshot.sites = []

    render(<AccountsPage />)

    await waitFor(() => {
      expect(testState.detailSheetProps).not.toBeNull()
    })
    expect(testState.navigate).not.toHaveBeenCalled()
    expect(testState.detailSheetProps?.open).toBe(false)
  })
})
