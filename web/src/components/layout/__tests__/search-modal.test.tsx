// metapi-go/layout — command palette tests.
// Covers the local navigation layer (quick entries + settings matching)
// and the filtered deep links for check-in / proxy-log entity results.

import '@testing-library/jest-dom/vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import {
  afterAll,
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from 'vitest'

import i18n from '@/i18n/config'

import { SearchModal } from '../search-modal'

const { navigateMock, searchMock } = vi.hoisted(() => ({
  navigateMock: vi.fn(),
  searchMock: vi.fn(),
}))

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    useNavigate: () => navigateMock,
  }
})

vi.mock('@/lib/api/search', () => ({
  searchApi: { search: searchMock },
}))

// The dialog primitive probes browser APIs jsdom does not implement; cmdk
// scrolls the selected item into view on every render.
beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn()

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

afterAll(() => {
  vi.restoreAllMocks()
})

const EMPTY_RESPONSE = {
  sites: [],
  accounts: [],
  accountTokens: [],
  checkinLogs: [],
  proxyLogs: [],
  models: [],
}

function renderModal() {
  return render(<SearchModal open onOpenChange={vi.fn()} />)
}

async function typeQuery(value: string) {
  const input = screen.getByPlaceholderText(
    'Search pages, settings, sites, accounts, logs…'
  )
  fireEvent.change(input, { target: { value } })
  // Wait for the ~250 ms debounce + mocked backend round-trip to settle.
  await vi.waitFor(() => {
    expect(searchMock).toHaveBeenCalledWith(value, expect.anything())
  })
}

beforeEach(async () => {
  navigateMock.mockReset()
  searchMock.mockReset().mockResolvedValue(EMPTY_RESPONSE)
  await i18n.changeLanguage('en')
})

afterEach(() => cleanup())

describe('search palette navigation layer', () => {
  it('shows page quick entries and the settings subareas on an empty query', () => {
    renderModal()

    expect(screen.getByText('Pages')).toBeInTheDocument()
    // Primary pages from the root sidebar data.
    expect(screen.getByText('Sites')).toBeInTheDocument()
    expect(screen.getByText('Observability')).toBeInTheDocument()
    // The 5 settings subareas as quick entries under the Settings heading
    // (the heading itself also reads "Settings"; the /settings page entry
    // adds a third occurrence).
    expect(screen.getAllByText('Settings').length).toBeGreaterThanOrEqual(2)
    expect(screen.getByText('Basics')).toBeInTheDocument()
    expect(screen.getByText('Downstream')).toBeInTheDocument()
    expect(screen.getByText('System & Ops')).toBeInTheDocument()
    expect(searchMock).not.toHaveBeenCalled()
  })

  it('matches settings sections locally while typing', async () => {
    renderModal()

    const input = screen.getByPlaceholderText(
      'Search pages, settings, sites, accounts, logs…'
    )
    fireEvent.change(input, { target: { value: 'schedul' } })

    await vi.waitFor(() => {
      expect(screen.getByText('Scheduled Tasks')).toBeInTheDocument()
    })
    // The local layer works without a backend response.
    expect(searchMock).not.toHaveBeenCalled()
  })

  it('navigates to the matched settings section on select', async () => {
    renderModal()

    const input = screen.getByPlaceholderText(
      'Search pages, settings, sites, accounts, logs…'
    )
    fireEvent.change(input, { target: { value: 'operational events' } })

    const entry = await screen.findByText('Operational Events')
    fireEvent.click(entry)

    expect(navigateMock).toHaveBeenCalledWith({
      to: '/settings/operations/program-logs',
    })
  })
})

describe('search palette entity deep links', () => {
  it('deep-links check-in results with account and text filters', async () => {
    searchMock.mockResolvedValue({
      ...EMPTY_RESPONSE,
      checkinLogs: [
        {
          id: 11,
          accountId: 7,
          accountUsername: 'alice',
          message: 'timeout connecting',
        },
      ],
    })
    renderModal()
    await typeQuery('timeout')

    fireEvent.click(await screen.findByText('timeout connecting'))

    expect(navigateMock).toHaveBeenCalledWith({
      to: '/checkin',
      search: { accountId: 7, q: 'timeout' },
    })
  })

  it('deep-links proxy log results straight to /proxy-logs with filters', async () => {
    searchMock.mockResolvedValue({
      ...EMPTY_RESPONSE,
      proxyLogs: [
        {
          id: 21,
          modelRequested: 'gpt-4o',
          status: 'failed',
        },
      ],
    })
    renderModal()
    await typeQuery('gpt')

    fireEvent.click(await screen.findByText('gpt-4o'))

    expect(navigateMock).toHaveBeenCalledWith({
      to: '/proxy-logs',
      search: { q: 'gpt-4o', status: 'failed' },
    })
  })
})
