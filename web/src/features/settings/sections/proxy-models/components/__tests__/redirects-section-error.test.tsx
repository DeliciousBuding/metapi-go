// Regression test: a failed redirects load used to fall through to the
// empty-list branch, masquerading as "no redirects". The section must now
// render SettingsSectionError with a Retry action (audit #1029 batch B) —
// mirroring the allowlist section's error branch.
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

import { RedirectsSection } from '../redirects-section'

const { mockGetRedirects } = vi.hoisted(() => ({
  mockGetRedirects: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  api: {
    getModelRedirects: mockGetRedirects,
    deleteModelRedirect: vi.fn(),
    applyModelRedirects: vi.fn(),
    generateModelRedirects: vi.fn(),
    updateModelRedirect: vi.fn(),
  },
}))

vi.mock('@/lib/toast', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
    info: vi.fn(),
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
  mockGetRedirects.mockReset()
})

afterEach(() => cleanup())

function renderSection() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: 0 },
      mutations: { retry: false },
    },
  })
  return render(
    (
      <QueryClientProvider client={queryClient}>
        <RedirectsSection />
      </QueryClientProvider>
    ) as ReactElement
  )
}

describe('RedirectsSection load error', () => {
  it('renders the section error instead of a fake empty list', async () => {
    mockGetRedirects.mockRejectedValue(new Error('boom'))

    renderSection()

    await screen.findByText(/Failed to load settings/)
    expect(screen.getByRole('button', { name: 'Retry' })).toBeInTheDocument()
    // The empty-list copy must not masquerade as the failure state.
    expect(screen.queryByText(/No model redirects/)).not.toBeInTheDocument()
  })

  it('recovers to the list after Retry', async () => {
    mockGetRedirects.mockRejectedValueOnce(new Error('boom'))
    mockGetRedirects.mockResolvedValue({
      items: [
        {
          id: 7,
          accountId: 1,
          username: 'ops',
          canonical: 'gpt-5.5',
          actual: 'gpt-5.4',
          source: 'manual',
          createdAt: '2026-01-01T00:00:00Z',
          updatedAt: '2026-01-01T00:00:00Z',
        },
      ],
    })

    renderSection()

    fireEvent.click(await screen.findByRole('button', { name: 'Retry' }))

    await waitFor(() => {
      expect(screen.getByText('gpt-5.5')).toBeInTheDocument()
    })
  })
})
