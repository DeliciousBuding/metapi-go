// Page-level wiring for the channels error banner (P1-4 closure): the
// banner counts runtime-failing channels (cooldown / breaker_open — never
// manually_disabled, which is operator intent) from the loaded list, its
// filter action writes the shareable `?status=` facet, and an error-only
// URL scope flips it into the clearable indicator.
import '@testing-library/jest-dom/vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import {
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from 'vitest'

import '@/i18n/config'

import { ChannelsPage } from '../components/channels-page'
import type { ChannelRow } from '../types'

const testState = vi.hoisted(() => ({
  channels: [] as ChannelRow[],
  navigate: vi.fn(),
  search: {} as Record<string, unknown>,
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
  useLocation: () => ({ href: '/channels', searchStr: '' }),
  Link: ({ children }: { children?: unknown }) => children,
}))

vi.mock('@/components/data-table', async (importOriginal) => {
  const actual =
    await importOriginal<typeof import('@/components/data-table')>()
  return {
    ...actual,
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
  }
})

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

function makeChannel(overrides: Partial<ChannelRow>): ChannelRow {
  return {
    id: 1,
    routeId: 1,
    name: 'probe-channel',
    site: { id: 1, name: 'Probe site' },
    type: 'account',
    status: 'enabled',
    models: 'gpt-*',
    priority: 1,
    weight: 1,
    responseMs: null,
    cooldownUntil: null,
    cooldownReasonCode: null,
    cooldownReason: null,
    cooldownReasonAt: null,
    enabled: true,
    manualOverride: false,
    ...overrides,
  }
}

beforeAll(() => {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: vi.fn().mockImplementation((query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  })

  class ResizeObserverStub {
    observe() {}
    unobserve() {}
    disconnect() {}
  }
  Object.defineProperty(globalThis, 'ResizeObserver', {
    writable: true,
    value: ResizeObserverStub,
  })
})

beforeEach(() => {
  testState.channels = []
  testState.search = {}
  testState.navigate.mockReset()
})

afterEach(() => cleanup())

describe('ChannelsPage error banner filter loop', () => {
  it('counts runtime failures and writes the status facet on filter', () => {
    testState.channels = [
      makeChannel({ id: 1, status: 'cooldown' }),
      makeChannel({ id: 2, status: 'breaker_open' }),
      makeChannel({ id: 3, status: 'enabled' }),
      // Operator intent, not a failure: excluded from the count.
      makeChannel({ id: 4, status: 'manually_disabled' }),
    ]
    render(<ChannelsPage />)

    expect(
      screen.getByText('2 channels are failing (cooldown or circuit breaker).')
    ).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Show failing' }))
    expect(testState.navigate).toHaveBeenCalledTimes(1)
    const call = testState.navigate.mock.calls[0][0] as {
      to: string
      search: Record<string, unknown>
    }
    expect(call.to).toBe('/channels')
    expect(call.search.status).toBe('cooldown,breaker_open')
    expect(call.search.page).toBe(0)
  })

  it('shows the clearable indicator when the URL already scopes to errors', () => {
    testState.channels = [
      makeChannel({ id: 1, status: 'cooldown' }),
      makeChannel({ id: 2, status: 'enabled' }),
    ]
    testState.search = { status: 'cooldown,breaker_open' }
    render(<ChannelsPage />)

    expect(
      screen.getByText('Showing failing channels only.')
    ).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Show all' }))
    const call = testState.navigate.mock.calls[0][0] as {
      search: Record<string, unknown>
    }
    expect(call.search.status).toBeUndefined()
  })

  it('renders no banner when every channel is healthy', () => {
    testState.channels = [makeChannel({ id: 1, status: 'enabled' })]
    render(<ChannelsPage />)
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })
})
