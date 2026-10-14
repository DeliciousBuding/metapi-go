// Behavior test for the downstream key list's restriction filter (#1430).
//
// Operators auditing least-privilege need to answer "which keys still have no
// restriction?" without reading every scope cell. The toolbar filter and the
// count line are fed by the same keyRestrictionDimensions owner as the row
// indicator, so these tests pin the wiring end to end: a wildcard-only model
// grant is an explicit unrestricted key, while any other configured dimension
// (here: model rules and a rate window) makes a key restricted.

import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
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
    getAccountsSnapshot: () =>
      Promise.resolve({ accounts: [], sites: [], generatedAt: '' }),
    getAccountTokens: () => Promise.resolve([]),
  },
}))

vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() },
}))

const keyItems = [
  // Explicit all-models grant and nothing else: the canonical
  // "unrestricted" key the filter exists to surface.
  {
    id: 1,
    name: 'wide-open key',
    keyMasked: 'sk-…a',
    enabled: true,
    supportedModels: ['*'],
  },
  {
    id: 2,
    name: 'model-scoped key',
    keyMasked: 'sk-…b',
    enabled: true,
    supportedModels: ['gpt-5.5'],
  },
  {
    id: 3,
    name: 'rate-capped key',
    keyMasked: 'sk-…c',
    enabled: true,
    supportedModels: [],
    maxRpm: 60,
  },
]

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
})

beforeEach(() => {
  mockGetKeys.mockReset()
  mockGetKeys.mockResolvedValue({ items: keyItems })
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

async function chooseRestriction(optionName: string) {
  fireEvent.click(screen.getByRole('combobox', { name: 'Restriction filter' }))
  const option = await screen.findByRole('option', { name: optionName })
  fireEvent.pointerDown(option)
  fireEvent.click(option)
}

function visibleKeyNames(): string[] {
  return keyItems
    .map((item) => item.name)
    .filter((name) => screen.queryByText(name) !== null)
}

describe('KeysSection restriction filter (#1430)', () => {
  it('lists every key by default and reports the restricted count', async () => {
    renderKeysSection()

    await screen.findByText('wide-open key')
    expect(screen.getByText('model-scoped key')).toBeInTheDocument()
    expect(screen.getByText('rate-capped key')).toBeInTheDocument()
    expect(screen.getByText('2 restricted of 3')).toBeInTheDocument()
    // The default filter option is "All keys".
    expect(
      screen.getByRole('combobox', { name: 'Restriction filter' })
    ).toHaveTextContent('All keys')
  })

  it('narrows the list to restricted keys and back without a refetch', async () => {
    renderKeysSection()
    await screen.findByText('wide-open key')

    await chooseRestriction('Restricted only')
    await waitFor(() => {
      expect(visibleKeyNames()).toEqual(['model-scoped key', 'rate-capped key'])
    })

    await chooseRestriction('Unrestricted only')
    await waitFor(() => {
      expect(visibleKeyNames()).toEqual(['wide-open key'])
    })

    await chooseRestriction('All keys')
    await waitFor(() => {
      expect(visibleKeyNames()).toEqual([
        'wide-open key',
        'model-scoped key',
        'rate-capped key',
      ])
    })

    // Client-side filtering: the list query ran exactly once.
    expect(mockGetKeys).toHaveBeenCalledTimes(1)
  })
})
