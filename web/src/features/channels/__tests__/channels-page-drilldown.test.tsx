// Behavior tests for the channels one-shot drilldown (proxy-log detail ->
// `/channels?channelId=N`): the page opens the detail sheet for the
// referenced channel and strips the param. A stale id is stripped without
// opening anything. Mirrors the accounts create/siteId consume pattern.

import '@testing-library/jest-dom/vitest'
import { cleanup, render, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { ChannelsPage } from '../components/channels-page'
import type { ChannelRow } from '../types'

const testState = vi.hoisted(() => ({
  search: {} as Record<string, unknown>,
  channels: [] as ChannelRow[],
  navigate: vi.fn(),
  detailSheetProps: null as {
    channel: ChannelRow | null
    open: boolean
  } | null,
  isLoading: false as boolean,
  isFetching: false as boolean,
  error: null as Error | null,
  refetch: vi.fn(),
}))

vi.mock('@/components/common/use-probe-history', () => ({
  useProbeHistory: () => ({ data: undefined }),
}))
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => testState.navigate,
  useSearch: () => testState.search,
}))

vi.mock('@/components/data-table', () => ({
  DataTablePage: () => null,
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
  ChannelDetailSheet: (props: {
    channel: ChannelRow | null
    open: boolean
  }) => {
    testState.detailSheetProps = props
    return null
  },
}))

vi.mock('../components/cooldown-reason-dialog', () => ({
  CooldownReasonDialog: () => null,
}))

vi.mock('../components/channels-columns', () => ({
  useChannelsColumns: () => [],
  CHANNELS_STATUS_FILTER_OPTIONS: [],
}))

function makeChannel(id: number): ChannelRow {
  return {
    id,
    routeId: 1,
    name: `channel-${id}`,
    status: 'enabled',
    enabled: true,
  } as unknown as ChannelRow
}

beforeEach(() => {
  testState.search = {}
  testState.channels = []
  testState.navigate.mockReset()
  testState.detailSheetProps = null
})

afterEach(() => cleanup())

describe('ChannelsPage drilldown', () => {
  it('opens the detail sheet for the referenced channel and strips the param', async () => {
    testState.search = { channelId: 3 }
    testState.channels = [makeChannel(3), makeChannel(4)]

    render(<ChannelsPage />)

    await waitFor(() => {
      expect(testState.detailSheetProps?.open).toBe(true)
    })
    expect(testState.detailSheetProps?.channel?.id).toBe(3)
    expect(testState.navigate).toHaveBeenCalledWith(
      expect.objectContaining({
        to: '/channels',
        replace: true,
        search: expect.objectContaining({ channelId: undefined }),
      })
    )
  })

  it('strips a stale channel id without opening the sheet', async () => {
    testState.search = { channelId: 99 }
    testState.channels = [makeChannel(3)]

    render(<ChannelsPage />)

    await waitFor(() => {
      expect(testState.navigate).toHaveBeenCalled()
    })
    expect(testState.detailSheetProps?.open).toBe(false)
    expect(testState.detailSheetProps?.channel).toBeNull()
  })

  it('does not navigate when no drilldown param is present', async () => {
    testState.channels = [makeChannel(3)]

    render(<ChannelsPage />)

    await waitFor(() => {
      expect(testState.detailSheetProps).not.toBeNull()
    })
    expect(testState.navigate).not.toHaveBeenCalled()
    expect(testState.detailSheetProps?.open).toBe(false)
  })
})
