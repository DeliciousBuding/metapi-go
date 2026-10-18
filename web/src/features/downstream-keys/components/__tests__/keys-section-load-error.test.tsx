// Pins the keys list's load-failure branch. The section is rendered by the
// standalone /downstream-keys route, so its failure copy must describe the key
// list: while it shared the settings chrome it also inherited
// `settings.common.loadFailed` ("Failed to load settings. The form is disabled
// to avoid overwriting your configuration."), which describes a settings form
// this page does not have. `SectionError` now takes the copy from its caller.
//
// Second half mirrors the redirects section's regression test: a failed load
// must not fall through to the empty-list branch and read as "no keys".

import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import type { ReactElement, ReactNode } from 'react'
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

// The keys table's row actions link to the key-filtered logs page; the real
// TanStack Router Link needs a router context these section-level tests do
// not mount, so render a plain anchor whose href stays assertable.
vi.mock('@tanstack/react-router', () => ({
  Link: ({
    to,
    search,
    children,
    ...rest
  }: {
    to: string
    search?: Record<string, unknown>
    children: ReactNode
  } & Record<string, unknown>) => {
    const query = search
      ? '?' + new URLSearchParams(search as Record<string, string>).toString()
      : ''
    return (
      <a href={to + query} {...rest}>
        {children}
      </a>
    )
  },
}))

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

describe('KeysSection load error', () => {
  it('explains the migrated token and stops showing the notice after it is deleted', async () => {
    mockGetKeys.mockResolvedValue({
      migratedKeyId: 71,
      items: [
        {
          id: 71,
          name: 'Migrated access token',
          enabled: false,
          supportedModels: ['*'],
        },
      ],
    })
    const view = renderKeysSection()
    expect(
      await screen.findByText('Your root key has moved to API keys')
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Review migrated key' })
    ).toBeInTheDocument()
    view.unmount()
    mockGetKeys.mockResolvedValue({ migratedKeyId: 71, items: [] })
    renderKeysSection()
    await screen.findByText(/No API keys yet/)
    expect(
      screen.queryByText('Your root key has moved to API keys')
    ).not.toBeInTheDocument()
  })

  it('reports the key list failure, not the settings form failure', async () => {
    mockGetKeys.mockRejectedValue(new Error('boom'))

    renderKeysSection()

    await screen.findByText('Failed to load API keys.')
    expect(screen.getByRole('button', { name: 'Retry' })).toBeInTheDocument()
    // The settings copy this section inherited must not come back: there is no
    // settings form on this page to disable.
    expect(
      screen.queryByText(/Failed to load settings/)
    ).not.toBeInTheDocument()
    // A failed load must not masquerade as an empty list.
    expect(screen.queryByText(/No API keys yet/)).not.toBeInTheDocument()
  })

  it('recovers to the list after Retry', async () => {
    mockGetKeys.mockRejectedValueOnce(new Error('boom'))
    mockGetKeys.mockResolvedValue({
      items: [
        {
          id: 1,
          name: 'prod key',
          keyMasked: 'sk-…abc',
          enabled: true,
          supportedModels: [],
        },
      ],
    })

    renderKeysSection()

    fireEvent.click(await screen.findByRole('button', { name: 'Retry' }))

    await waitFor(() => {
      expect(screen.getByText('prod key')).toBeInTheDocument()
    })
  })
})
