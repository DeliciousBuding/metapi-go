// Pins the checkin page's fetch-window behavior for the manual check-in
// entry points. While the accounts snapshot is still fetching,
// `accountOptions` is `[]`, and pinning the header button's `disabled` and
// the empty-state CTA branch to that raw array made the button gray for the
// whole fetch window and flashed "Manage accounts" before snapping back.
// Same fix pattern as the accounts page's Add account button: only the
// LOADED-AND-EMPTY state disables/redirects.

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

import { CheckinPage } from '../components/checkin-page'

const MANUAL_CHECKIN_NAME = 'Manual check-in'
const MANAGE_ACCOUNTS_NAME = 'Manage accounts'

const testState = vi.hoisted(() => ({
  accountsQuery: {
    data: undefined as
      | {
          generatedAt: string
          accounts: Array<{ id: number; username: string }>
        }
      | undefined,
    isLoading: true,
  },
  manualOpen: false,
}))

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => vi.fn(),
}))

// Surface the empty-state CTA so the branch can be asserted directly; the
// table itself is irrelevant to this regression.
vi.mock('@/components/data-table', () => ({
  DataTablePage: (props: { emptyAction?: ReactNode }) => (
    <div data-testid='checkin-empty-action'>{props.emptyAction}</div>
  ),
  useDataTable: () => ({ table: {} }),
  useUrlTableState: () => ({
    globalFilter: '',
    onGlobalFilterChange: vi.fn(),
    pagination: { pageIndex: 0, pageSize: 20 },
    onPaginationChange: vi.fn(),
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

vi.mock('@/features/accounts', () => ({
  useAccounts: () => testState.accountsQuery,
}))

vi.mock('@/features/sites/api', () => ({
  useSites: () => ({ data: [] }),
}))

vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() },
}))

vi.mock('../api', () => ({
  useCheckinLogs: () => ({
    data: { items: [], total: 0 },
    isLoading: false,
    isFetching: false,
    error: null,
    refetch: vi.fn(),
  }),
  useManualCheckin: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useCheckinAccount: () => ({
    mutateAsync: vi.fn(),
    isPending: false,
    variables: null,
  }),
}))

vi.mock('../components/checkin-columns', () => ({
  useCheckinColumns: () => [],
}))

vi.mock('../components/checkin-detail-sheet', () => ({
  CheckinDetailSheet: () => null,
}))

vi.mock('../components/manual-checkin-dialog', () => ({
  ManualCheckinDialog: (props: { open: boolean }) => {
    testState.manualOpen = props.open
    return null
  },
}))

afterEach(() => cleanup())

beforeEach(() => {
  testState.accountsQuery = { data: undefined, isLoading: true }
  testState.manualOpen = false
})

describe('CheckinPage manual check-in while the accounts snapshot loads', () => {
  it('keeps the header button enabled and clickable during loading', () => {
    render(<CheckinPage />)

    const buttons = screen.getAllByRole('button', {
      name: MANUAL_CHECKIN_NAME,
    })
    for (const button of buttons) {
      expect(button).not.toBeDisabled()
    }

    // Clicking the header button opens the manual check-in dialog even
    // though the account list has not landed yet.
    fireEvent.click(buttons[0])
    expect(testState.manualOpen).toBe(true)
  })

  it('keeps the empty-state CTA on manual check-in during loading (no Manage accounts flash)', () => {
    render(<CheckinPage />)

    const emptyAction = screen.getByTestId('checkin-empty-action')
    expect(
      within(emptyAction).getByRole('button', { name: MANUAL_CHECKIN_NAME })
    ).toBeInTheDocument()
    expect(
      within(emptyAction).queryByRole('button', { name: MANAGE_ACCOUNTS_NAME })
    ).toBeNull()
  })
})

describe('CheckinPage manual check-in once the snapshot has loaded', () => {
  it('disables the header button for a genuinely empty account library', () => {
    testState.accountsQuery = {
      data: { generatedAt: '', accounts: [] },
      isLoading: false,
    }

    render(<CheckinPage />)

    expect(
      screen.getByRole('button', { name: MANUAL_CHECKIN_NAME })
    ).toBeDisabled()
  })

  it('points the empty-state CTA to the accounts page when the library is empty', () => {
    testState.accountsQuery = {
      data: { generatedAt: '', accounts: [] },
      isLoading: false,
    }

    render(<CheckinPage />)

    const emptyAction = screen.getByTestId('checkin-empty-action')
    expect(
      within(emptyAction).getByRole('button', { name: MANAGE_ACCOUNTS_NAME })
    ).toBeInTheDocument()
    expect(
      within(emptyAction).queryByRole('button', { name: MANUAL_CHECKIN_NAME })
    ).toBeNull()
  })

  it('enables the header button once accounts exist', () => {
    testState.accountsQuery = {
      data: {
        generatedAt: '',
        accounts: [{ id: 1, username: 'primary-account' }],
      },
      isLoading: false,
    }

    render(<CheckinPage />)

    const buttons = screen.getAllByRole('button', {
      name: MANUAL_CHECKIN_NAME,
    })
    for (const button of buttons) {
      expect(button).not.toBeDisabled()
    }
  })
})
