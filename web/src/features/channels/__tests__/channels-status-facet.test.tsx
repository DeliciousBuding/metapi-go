// Behavior test for the channels status facet (issue #887 residual: the
// /channels toolbar had only a search box, so failing channels — cooldown /
// breaker_open — could not be isolated without eyeballing the table).
// Channels are filtered client-side by the shared data table, so the facet is
// exercised end-to-end here: real columns, real toolbar, real filter row model.

import '@testing-library/jest-dom/vitest'
import type { ColumnFiltersState } from '@tanstack/react-table'
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

import { CHANNELS_STATUS_FILTER_OPTIONS } from '../components/channels-columns'
import { ChannelsPage } from '../components/channels-page'
import type { ChannelRow } from '../types'

const testState = vi.hoisted(() => ({
  channels: [] as ChannelRow[],
  columnFilters: [] as ColumnFiltersState,
  onColumnFiltersChange: vi.fn(),
  navigate: vi.fn(),
}))

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => testState.navigate,
  useSearch: () => ({}),
  useLocation: () => ({ href: '/channels', searchStr: '' }),
  Link: ({ children }: { children?: unknown }) => children,
}))

// Only the URL-sync layer is stubbed: the column filters are injected directly
// so the assertions cover the real facet -> filterFn -> row model path.
vi.mock('@/components/data-table', async (importOriginal) => {
  const actual =
    await importOriginal<typeof import('@/components/data-table')>()
  return {
    ...actual,
    useUrlTableState: () => ({
      globalFilter: '',
      onGlobalFilterChange: vi.fn(),
      columnFilters: testState.columnFilters,
      onColumnFiltersChange: testState.onColumnFiltersChange,
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
    type: 'account',
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

/** One channel per real routing status, so every facet value has a row. */
function seedOneChannelPerStatus(): ChannelRow[] {
  return [
    makeChannel({ id: 1, name: 'healthy-one', status: 'enabled' }),
    makeChannel({ id: 2, name: 'cooling-two', status: 'cooldown' }),
    makeChannel({ id: 3, name: 'tripped-three', status: 'breaker_open' }),
    makeChannel({ id: 4, name: 'off-four', status: 'manually_disabled' }),
  ]
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

  // jsdom does not implement scrollIntoView; cmdk calls it when the facet
  // popover mounts and moves its active item.
  Element.prototype.scrollIntoView = vi.fn()
})

beforeEach(() => {
  testState.channels = seedOneChannelPerStatus()
  testState.columnFilters = []
  testState.onColumnFiltersChange.mockReset()
  testState.navigate.mockReset()
})

afterEach(() => cleanup())

/**
 * Resolve the toolbar facet trigger. The sortable `status` column header is
 * also a button named "Status", so disambiguate on the popover trigger slot
 * rather than on the accessible name alone.
 */
async function findStatusFacetTrigger(): Promise<HTMLElement> {
  const candidates = await screen.findAllByRole('button', { name: /Status/ })
  const trigger = candidates.find(
    (button) => button.dataset.slot === 'popover-trigger'
  )
  if (!trigger) {
    throw new Error('status facet trigger is not rendered in the toolbar')
  }
  return trigger
}

async function openStatusFacet(): Promise<void> {
  fireEvent.click(await findStatusFacetTrigger())
}

describe('CHANNELS_STATUS_FILTER_OPTIONS', () => {
  it('exposes exactly the four routing statuses in precedence order', () => {
    expect(
      CHANNELS_STATUS_FILTER_OPTIONS.map((option) => option.value)
    ).toEqual(['enabled', 'cooldown', 'breaker_open', 'manually_disabled'])
  })
})

describe('ChannelsPage status facet', () => {
  it('renders a status facet trigger in the toolbar', async () => {
    render(<ChannelsPage />)

    expect(await findStatusFacetTrigger()).toBeInTheDocument()
  })

  it('offers every real routing status as an option', async () => {
    render(<ChannelsPage />)

    await openStatusFacet()

    for (const label of [
      'Enabled',
      'Cooldown',
      'Breaker open',
      'Manually disabled',
    ]) {
      expect(
        await screen.findByRole('option', { name: new RegExp(label) })
      ).toBeInTheDocument()
    }
  })

  it('propagates the picked status to the table column filter', async () => {
    render(<ChannelsPage />)

    await openStatusFacet()
    fireEvent.click(await screen.findByRole('option', { name: /Cooldown/ }))

    expect(testState.onColumnFiltersChange).toHaveBeenCalled()
    const update = testState.onColumnFiltersChange.mock.calls[0][0]
    const nextFilters: ColumnFiltersState =
      typeof update === 'function' ? update([]) : update
    expect(nextFilters).toEqual([{ id: 'status', value: ['cooldown'] }])
  })

  it('shows only the failing channels when the URL carries a status filter', async () => {
    // The shareable "why is this channel failing" view: ?status=cooldown,
    // breaker_open. The status column filterFn must narrow the rows to exactly
    // those two and drop the healthy / manually disabled ones.
    testState.columnFilters = [
      { id: 'status', value: ['cooldown', 'breaker_open'] },
    ]

    render(<ChannelsPage />)

    expect(await screen.findByText('cooling-two')).toBeInTheDocument()
    expect(screen.getByText('tripped-three')).toBeInTheDocument()
    expect(screen.queryByText('healthy-one')).not.toBeInTheDocument()
    expect(screen.queryByText('off-four')).not.toBeInTheDocument()
  })

  it('keeps every channel visible when no status is selected', async () => {
    render(<ChannelsPage />)

    expect(await screen.findByText('healthy-one')).toBeInTheDocument()
    expect(screen.getByText('cooling-two')).toBeInTheDocument()
    expect(screen.getByText('tripped-three')).toBeInTheDocument()
    expect(screen.getByText('off-four')).toBeInTheDocument()
  })
})
