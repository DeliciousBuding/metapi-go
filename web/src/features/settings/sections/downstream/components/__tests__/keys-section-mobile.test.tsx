// Pins the downstream-keys mobile contract. The section used to render a
// bare <Table> that horizontally scrolled the whole row set on phones,
// pushing Connect/Delete off-screen. It now participates in the shared
// DataTablePage contract: ≤640px (TABLE_MOBILE_MEDIA_QUERY) renders the
// MobileCardList driven by column meta (mobileTitle = name, mobileBadge =
// enable switch); above it the desktop table stays.

import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import type { ReactElement } from 'react'
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
import { TABLE_MOBILE_MEDIA_QUERY } from '@/lib/breakpoints'

import { KeysSection } from '../keys-section'

const { mockGetKeys } = vi.hoisted(() => ({
  mockGetKeys: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  api: {
    getDownstreamApiKeys: mockGetKeys,
    createDownstreamApiKey: vi.fn(),
    updateDownstreamApiKey: vi.fn(),
    deleteDownstreamApiKey: vi.fn(),
    getSites: () => Promise.resolve([]),
  },
}))

vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

// Configurable media query: flip `mobileMatches` to emulate the 375px
// viewport (mobile card list) or a desktop viewport (table).
let mobileMatches = false

beforeAll(() => {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: vi.fn().mockImplementation((query: string) => ({
      matches: query === TABLE_MOBILE_MEDIA_QUERY ? mobileMatches : false,
      media: query,
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  })
})

beforeEach(() => {
  mobileMatches = false
  mockGetKeys.mockReset()
  mockGetKeys.mockResolvedValue({
    items: [
      {
        id: 1,
        name: 'prod key',
        keyMasked: 'sk-…abc',
        groupName: 'default',
        enabled: true,
        usedRequests: 10,
        maxRequests: 100,
        usedCost: 2,
        maxCost: 50,
        supportedModels: [],
      },
      {
        id: 2,
        name: 'dev key',
        keyMasked: 'sk-…def',
        enabled: false,
        supportedModels: ['*'],
      },
    ],
  })
})

afterEach(() => cleanup())

function renderKeysSection() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: 0 },
      mutations: { retry: false },
    },
  })
  return render(
    (
      <QueryClientProvider client={queryClient}>
        <KeysSection />
      </QueryClientProvider>
    ) as ReactElement
  )
}

describe('KeysSection — mobile (375px) rendering', () => {
  beforeEach(() => {
    mobileMatches = true
  })

  it('renders the card list instead of the desktop table', async () => {
    renderKeysSection()

    await waitFor(() => {
      expect(screen.getByText('prod key')).toBeInTheDocument()
    })

    // The bare <Table> must be gone on mobile: no table element, no
    // column headers.
    expect(document.querySelector('table')).toBeNull()
    expect(screen.queryByRole('columnheader')).toBeNull()

    // Card content: name (mobileTitle), masked key, enable switch
    // (mobileBadge), and the Connect/Edit/Delete actions all reachable.
    expect(screen.getByText('sk-…abc')).toBeInTheDocument()
    expect(screen.getByText('dev key')).toBeInTheDocument()
    expect(
      screen.getByRole('switch', { name: 'Toggle enabled for prod key' })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('switch', { name: 'Toggle enabled for dev key' })
    ).toBeInTheDocument()
    expect(screen.getAllByRole('button', { name: 'Connect' })).toHaveLength(2)
    expect(screen.getAllByRole('button', { name: 'Delete' })).toHaveLength(2)
    expect(screen.getByText('No models authorized')).toBeInTheDocument()
    expect(screen.getByText('All models')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Edit prod key' })
    ).toBeInTheDocument()
  })
})

describe('KeysSection — desktop rendering', () => {
  it('keeps the desktop table above the mobile breakpoint', async () => {
    renderKeysSection()

    await waitFor(() => {
      expect(screen.getByText('prod key')).toBeInTheDocument()
    })

    // Desktop keeps the table with its column headers.
    expect(
      screen.getByRole('columnheader', { name: 'Group' })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('columnheader', { name: 'Models' })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('columnheader', { name: 'Usage' })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('switch', { name: 'Toggle enabled for prod key' })
    ).toBeInTheDocument()
    expect(screen.getAllByRole('button', { name: 'Connect' })).toHaveLength(2)
  })
})
