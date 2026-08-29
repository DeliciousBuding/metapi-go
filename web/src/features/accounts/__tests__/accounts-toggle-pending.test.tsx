// Behavior test for the AccountsPage → columns pending-id wiring (Wave 11
// feedback loops). Each toggle mutation derives its OWN per-row pending id
// from `isPending` + `variables` so pin / check-in / status spinners never
// cross-talk. Captures the positional args handed to the columns hook via a
// mock, mirroring accounts-toggle-feedback.test.tsx's seam strategy.
import '@testing-library/jest-dom/vitest'
import { cleanup, render, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { AccountsPage } from '../components/accounts-page'
import type { AccountRowActions } from '../types'

type PendingArgs = {
  pendingStatusId: number | null
  pendingCheckinId: number | null
  pendingPinId: number | null
}

const testState = vi.hoisted(() => ({
  captured: null as PendingArgs | null,
  pin: {
    isPending: false,
    variables: undefined as { id: number; isPinned: boolean } | undefined,
  },
  checkin: {
    isPending: false,
    variables: undefined as { id: number; checkinEnabled: boolean } | undefined,
  },
  status: {
    isPending: false,
    variables: undefined as { id: number; status: string } | undefined,
  },
}))

vi.mock('@/components/common/use-probe-history', () => ({
  useProbeHistory: () => ({ data: undefined }),
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
    success: vi.fn(),
    error: vi.fn(),
    warning: vi.fn(),
    info: vi.fn(),
  },
}))

vi.mock('../api', () => ({
  useAccountsPage: () => ({
    data: { generatedAt: '', accounts: [], sites: [] },
    isLoading: false,
    isFetching: false,
    error: null,
  }),
  useBatchUpdateAccounts: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useDeleteAccount: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useRefreshAccount: () => ({ mutate: vi.fn(), isPending: false }),
  useToggleAccountCheckin: () => ({
    mutate: vi.fn(),
    isPending: testState.checkin.isPending,
    variables: testState.checkin.variables,
  }),
  useToggleAccountPin: () => ({
    mutate: vi.fn(),
    isPending: testState.pin.isPending,
    variables: testState.pin.variables,
  }),
  useToggleAccountStatus: () => ({
    mutate: vi.fn(),
    isPending: testState.status.isPending,
    variables: testState.status.variables,
  }),
}))

vi.mock('../components/account-detail-sheet', () => ({
  AccountDetailSheet: () => null,
}))

vi.mock('../components/account-form-dialog', () => ({
  AccountFormDialog: () => null,
}))

vi.mock('../components/accounts-columns', () => ({
  useAccountsColumns: (
    _actions: AccountRowActions,
    pendingStatusId: number | null,
    pendingCheckinId: number | null,
    pendingPinId: number | null
  ) => {
    testState.captured = {
      pendingStatusId,
      pendingCheckinId,
      pendingPinId,
    }
    return []
  },
}))

beforeEach(() => {
  testState.captured = null
  testState.pin = { isPending: false, variables: undefined }
  testState.checkin = { isPending: false, variables: undefined }
  testState.status = { isPending: false, variables: undefined }
})

afterEach(() => cleanup())

describe('AccountsPage toggle pending wiring', () => {
  it('derives pendingPinId from the in-flight pin mutation only', async () => {
    testState.pin = { isPending: true, variables: { id: 9, isPinned: true } }

    render(<AccountsPage />)

    await waitFor(() => expect(testState.captured).not.toBeNull())
    expect(testState.captured).toEqual({
      pendingStatusId: null,
      pendingCheckinId: null,
      pendingPinId: 9,
    })
  })

  it('derives pendingCheckinId from the in-flight check-in mutation only', async () => {
    testState.checkin = {
      isPending: true,
      variables: { id: 21, checkinEnabled: true },
    }

    render(<AccountsPage />)

    await waitFor(() => expect(testState.captured).not.toBeNull())
    expect(testState.captured).toEqual({
      pendingStatusId: null,
      pendingCheckinId: 21,
      pendingPinId: null,
    })
  })

  it('keeps concurrent pin and check-in toggles on their own ids', async () => {
    testState.pin = { isPending: true, variables: { id: 9, isPinned: false } }
    testState.checkin = {
      isPending: true,
      variables: { id: 21, checkinEnabled: false },
    }

    render(<AccountsPage />)

    await waitFor(() => expect(testState.captured).not.toBeNull())
    expect(testState.captured).toEqual({
      pendingStatusId: null,
      pendingCheckinId: 21,
      pendingPinId: 9,
    })
  })

  it('reports no pending ids when every toggle is idle', async () => {
    render(<AccountsPage />)

    await waitFor(() => expect(testState.captured).not.toBeNull())
    expect(testState.captured).toEqual({
      pendingStatusId: null,
      pendingCheckinId: null,
      pendingPinId: null,
    })
  })
})
