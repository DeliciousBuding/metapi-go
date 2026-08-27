// Behavior test for the channels empty-state CTA (2026-08-18
// multi-perspective review: channels-page empty state had no next step).
// Channels are materialised from accounts + routes, so the CTA navigates
// to /accounts. The shared DataTablePage renders the empty state; this test
// exercises the page-level wiring with a real table instance.

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
}))

vi.mock('@/components/common/use-probe-history', () => ({
  useProbeHistory: () => ({ data: undefined }),
}))
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => testState.navigate,
  useSearch: () => ({}),
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
    isLoading: false,
    isFetching: false,
    error: null,
  }),
}))

vi.mock('../components/channel-detail-sheet', () => ({
  ChannelDetailSheet: () => null,
}))

function makeChannel(overrides: Partial<ChannelRow>): ChannelRow {
  return {
    id: 1,
    routeId: 1,
    name: 'probe-channel',
    site: { id: 1, name: 'Probe site' },
    type: 'openai',
    status: 'enabled',
    models: 'gpt-*',
    priority: 1,
    weight: 1,
    responseMs: null,
    cooldownUntil: null,
    enabled: true,
    manualOverride: false,
    ...overrides,
  } as ChannelRow
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
  testState.navigate.mockReset()
})

afterEach(() => cleanup())

describe('ChannelsPage empty state', () => {
  it('offers a manage-accounts CTA when no channels exist', async () => {
    render(<ChannelsPage />)

    const cta = await screen.findByRole('button', {
      name: /Manage accounts/,
    })
    expect(cta).toBeInTheDocument()
    expect(screen.getByText('No channels')).toBeInTheDocument()
  })

  it('navigates to the accounts page from the empty-state CTA', async () => {
    render(<ChannelsPage />)

    fireEvent.click(
      await screen.findByRole('button', { name: /Manage accounts/ })
    )

    expect(testState.navigate).toHaveBeenCalledWith({ to: '/accounts' })
  })

  it('hides the CTA once channels are materialised', async () => {
    testState.channels = [makeChannel({})]

    render(<ChannelsPage />)

    await screen.findByText('probe-channel')
    expect(
      screen.queryByRole('button', { name: /Manage accounts/ })
    ).not.toBeInTheDocument()
  })
})
