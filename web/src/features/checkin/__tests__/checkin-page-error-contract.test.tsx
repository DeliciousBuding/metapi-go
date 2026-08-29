// Pins the unified list-page error contract on the checkin page (W19-T1
// P2-o): a failed load REPLACES the filters + table with the
// QueryErrorBanner instead of stacking over them, and its Retry re-fetches.
// A stale cache must never read as current data.

import '@testing-library/jest-dom/vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { CheckinPage } from '../components/checkin-page'

const testState = vi.hoisted(() => ({
  logsQuery: {
    data: { items: [], total: 0 } as { items: unknown[]; total: number },
    isLoading: false,
    isFetching: false,
    error: null as Error | null,
    refetch: vi.fn(),
  },
}))

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => vi.fn(),
}))

// Render a probe in place of the table so presence/absence is assertable.
vi.mock('@/components/data-table', () => ({
  DataTablePage: () => <div data-testid='checkin-table' />,
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
  useAccounts: () => ({
    data: { generatedAt: '', accounts: [] },
    isLoading: false,
  }),
}))

vi.mock('@/features/sites/api', () => ({
  useSites: () => ({ data: [] }),
}))

vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() },
}))

vi.mock('../api', () => ({
  useCheckinLogs: () => testState.logsQuery,
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
  ManualCheckinDialog: () => null,
}))

beforeEach(() => {
  testState.logsQuery = {
    data: { items: [], total: 0 },
    isLoading: false,
    isFetching: false,
    error: null,
    refetch: vi.fn(),
  }
})

afterEach(() => cleanup())

describe('CheckinPage error contract', () => {
  it('replaces the filters and table with the error banner on failure', () => {
    testState.logsQuery.error = new Error('boom')
    render(<CheckinPage />)

    const alert = screen.getByRole('alert')
    expect(alert).toHaveTextContent('Failed to load check-in records: boom')
    // The table is hidden, not stacked under the banner.
    expect(screen.queryByTestId('checkin-table')).not.toBeInTheDocument()
    // The date-range filter row is part of the replaced region.
    expect(
      screen.queryByLabelText('Start time', { exact: false })
    ).not.toBeInTheDocument()
  })

  it('banner Retry re-fetches the logs query', () => {
    testState.logsQuery.error = new Error('boom')
    render(<CheckinPage />)

    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(testState.logsQuery.refetch).toHaveBeenCalledTimes(1)
  })

  it('renders the table without a banner when the load succeeds', () => {
    render(<CheckinPage />)

    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(screen.getByTestId('checkin-table')).toBeInTheDocument()
  })
})
