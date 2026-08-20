// Behavior tests for the checkin page empty-state CTA: with accounts
// present the CTA reuses the header's manual check-in flow (same dialog,
// same mutation); without accounts there is nothing to check in, so the
// CTA links to the accounts page instead of rendering a disabled button.

import '@testing-library/jest-dom/vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { CheckinPage } from '../checkin-page'

const testState = vi.hoisted(() => ({
  navigate: vi.fn(),
  accounts: [] as Array<{ id: number; username: string }>,
  manualDialogOpen: false,
}))

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => testState.navigate,
}))

vi.mock('@/components/data-table', () => ({
  // Render only the empty-state action slot (testid-scoped) so the test
  // can assert the CTA without colliding with the page header's own
  // "Manual check-in" button.
  DataTablePage: (props: { emptyAction?: ReactNode }) => (
    <div data-testid='empty-action-slot'>{props.emptyAction}</div>
  ),
  useDataTable: () => ({ table: {} }),
  useUrlTableState: () => ({
    globalFilter: '',
    onGlobalFilterChange: vi.fn(),
    pagination: { pageIndex: 0, pageSize: 20 },
    onPaginationChange: vi.fn(),
    sorting: [],
    onSortingChange: vi.fn(),
    columnFilters: [],
    onColumnFiltersChange: vi.fn(),
    filters: {
      status: '',
      reason: '',
      site: '',
      accountId: '',
      from: '',
      to: '',
    },
    updateUrlState: vi.fn(),
  }),
}))

vi.mock('@/components/common/query-error-banner', () => ({
  QueryErrorBanner: () => null,
}))

vi.mock('@/features/accounts', () => ({
  useAccounts: () => ({ data: { accounts: testState.accounts } }),
}))

vi.mock('@/features/sites/api', () => ({
  useSites: () => ({ data: [] }),
}))

vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() },
}))

vi.mock('../../api', () => ({
  useCheckinLogs: () => ({
    data: { items: [], total: 0 },
    isLoading: false,
    isFetching: false,
    error: null,
    refetch: vi.fn(),
  }),
  useManualCheckin: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useCheckinAccount: () => ({ mutateAsync: vi.fn(), isPending: false }),
}))

vi.mock('../checkin-columns', () => ({
  useCheckinColumns: () => [],
}))

vi.mock('../checkin-detail-sheet', () => ({
  CheckinDetailSheet: () => null,
}))

vi.mock('../manual-checkin-dialog', () => ({
  ManualCheckinDialog: (props: { open: boolean }) => {
    testState.manualDialogOpen = props.open
    return null
  },
}))

beforeEach(() => {
  testState.navigate.mockReset()
  testState.accounts = []
  testState.manualDialogOpen = false
})

afterEach(() => cleanup())

describe('CheckinPage empty state', () => {
  it('offers the manual check-in flow when accounts exist', () => {
    testState.accounts = [{ id: 1, username: 'acc-1' }]

    render(<CheckinPage />)

    // Scope to the empty-state slot: the page header renders its own
    // "Manual check-in" button as well.
    const emptySlot = screen.getByTestId('empty-action-slot')
    const cta = within(emptySlot).getByRole('button', {
      name: /Manual check-in/,
    })
    expect(cta).toBeInTheDocument()
    expect(testState.manualDialogOpen).toBe(false)

    fireEvent.click(cta)
    expect(testState.manualDialogOpen).toBe(true)
    expect(testState.navigate).not.toHaveBeenCalled()
  })

  it('falls back to a manage-accounts CTA when no accounts exist', () => {
    testState.accounts = []

    render(<CheckinPage />)

    const emptySlot = screen.getByTestId('empty-action-slot')
    expect(
      within(emptySlot).queryByRole('button', { name: /Manual check-in/ })
    ).not.toBeInTheDocument()

    const cta = within(emptySlot).getByRole('button', {
      name: /Manage accounts/,
    })
    fireEvent.click(cta)
    expect(testState.navigate).toHaveBeenCalledTimes(1)
    expect(testState.navigate).toHaveBeenCalledWith({ to: '/accounts' })
  })
})
