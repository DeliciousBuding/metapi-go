// Regression tests for the channels page load-error state: the shared
// QueryErrorBanner + Retry button wiring (audit gap-7: the accounts/channels
// load errors had no Retry — sites shipped first, the two list pages now
// share the same pattern).
import '@testing-library/jest-dom/vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { ChannelsPage } from '../components/channels-page'
import type { ChannelRow } from '../types'

const testState = vi.hoisted(() => ({
  search: {} as Record<string, unknown>,
  channels: [] as ChannelRow[],
  navigate: vi.fn(),
  refetch: vi.fn().mockResolvedValue({}),
  error: null as Error | null,
  isLoading: false,
  isFetching: false,
}))

vi.mock('@/components/common/use-probe-history', () => ({
  useProbeHistory: () => ({ data: undefined }),
}))
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => testState.navigate,
  useSearch: () => testState.search,
}))

vi.mock('@/components/data-table', () => ({
  DataTablePage: () => <div data-testid='data-table-page' />,
  encodeSorting: () => '',
  useUrlTableState: () => ({
    globalFilter: '',
    onGlobalFilterChange: vi.fn(),
    columnFilters: [],
    onColumnFiltersChange: vi.fn(),
    filters: { status: '' },
    pagination: { pageIndex: 0, pageSize: 20 },
    onPaginationChange: vi.fn(),
    sorting: [],
    onSortingChange: vi.fn(),
    ensurePageInRange: vi.fn(),
  }),
  useDataTable: () => ({ table: {} }),
}))

vi.mock('../api', () => ({
  useChannels: () => ({
    data: testState.channels,
    isLoading: testState.isLoading,
    isFetching: testState.isFetching,
    error: testState.error,
    refetch: testState.refetch,
  }),
  useChannelsPage: () => ({
    data: {
      items: testState.channels,
      total: testState.channels.length,
    },
    isLoading: testState.isLoading,
    isFetching: testState.isFetching,
    error: testState.error,
    refetch: testState.refetch,
  }),
  useChannelsErrorSummary: () => ({
    data: {
      total: testState.channels.length,
      errorCount: testState.channels.filter(
        (channel) =>
          channel.status === 'cooldown' || channel.status === 'breaker_open'
      ).length,
      byStatus: {
        enabled: 0,
        cooldown: 0,
        breaker_open: 0,
        manually_disabled: 0,
      },
    },
    isLoading: false,
    isFetching: false,
    error: null,
    refetch: vi.fn(),
  }),
}))

vi.mock('../components/channel-detail-sheet', () => ({
  ChannelDetailSheet: () => null,
}))

vi.mock('../components/cooldown-reason-dialog', () => ({
  CooldownReasonDialog: () => null,
}))

vi.mock('../components/channels-columns', () => ({
  useChannelsColumns: () => [],
  CHANNELS_STATUS_FILTER_OPTIONS: [],
}))

beforeEach(() => {
  testState.search = {}
  testState.channels = []
  testState.navigate.mockReset()
  testState.refetch.mockReset()
  testState.refetch.mockResolvedValue({})
  testState.error = null
  testState.isLoading = false
  testState.isFetching = false
})

afterEach(() => cleanup())

describe('ChannelsPage load error state', () => {
  it('renders the error banner with a Retry button when the list fails', () => {
    testState.error = new Error('boom')

    render(<ChannelsPage />)

    expect(
      screen.getByText(/Failed to load channels: boom/)
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Retry' })).toBeInTheDocument()
  })

  it('renders no error banner or Retry button when the list loads', () => {
    testState.error = null

    render(<ChannelsPage />)

    expect(
      screen.queryByText(/Failed to load channels/)
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Retry' })
    ).not.toBeInTheDocument()
  })

  it('calls refetch when the Retry button is clicked', () => {
    testState.error = new Error('boom')

    render(<ChannelsPage />)

    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))

    expect(testState.refetch).toHaveBeenCalledTimes(1)
  })

  it('suppresses the table (and its empty-state CTA) while the load error is present', () => {
    testState.error = new Error('boom')

    render(<ChannelsPage />)

    expect(screen.queryByTestId('data-table-page')).not.toBeInTheDocument()
  })

  it('renders the table when the list loads without error', () => {
    testState.error = null

    render(<ChannelsPage />)

    expect(screen.getByTestId('data-table-page')).toBeInTheDocument()
  })
})
