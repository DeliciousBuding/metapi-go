// metapi-go/layout — command palette tests.
// Covers the local navigation layer (quick entries + settings matching),
// the actions layer (registry rendering, keyboard execution, confirmation
// gate, mutation feedback) and the filtered deep links for entity results.

import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
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

const {
  navigateMock,
  searchMock,
  onOpenChangeMock,
  checkinAllMock,
  rebuildMock,
  refreshDecisionsMock,
  toastSuccessMock,
  toastErrorMock,
} = vi.hoisted(() => ({
  navigateMock: vi.fn(),
  searchMock: vi.fn(),
  onOpenChangeMock: vi.fn(),
  checkinAllMock: vi.fn(),
  rebuildMock: vi.fn(),
  refreshDecisionsMock: vi.fn(),
  toastSuccessMock: vi.fn(),
  toastErrorMock: vi.fn(),
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

// The actions layer reuses the feature mutation hooks; the tests drive them
// through the public barrels so the palette's execution paths run for real.
vi.mock('@/features/checkin', () => ({
  useManualCheckin: () => ({ mutateAsync: checkinAllMock }),
}))

vi.mock('@/features/token-routes', () => ({
  useRebuildRoutes: () => ({ mutate: rebuildMock, isPending: false }),
  useRefreshRouteDecisions: () => ({
    mutate: refreshDecisionsMock,
    isPending: false,
  }),
}))

vi.mock('@/lib/toast', () => ({
  toast: {
    success: toastSuccessMock,
    error: toastErrorMock,
    info: vi.fn(),
    warning: vi.fn(),
  },
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

const PLACEHOLDER = 'Search pages, settings, actions, sites, accounts, logs…'

function renderModal() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <SearchModal open onOpenChange={onOpenChangeMock} />
    </QueryClientProvider>
  )
}

async function typeQuery(value: string) {
  const input = screen.getByPlaceholderText(PLACEHOLDER)
  fireEvent.change(input, { target: { value } })
  // Wait for the ~250 ms debounce + mocked backend round-trip to settle.
  await vi.waitFor(() => {
    expect(searchMock).toHaveBeenCalledWith(value, expect.anything())
  })
}

beforeEach(async () => {
  navigateMock.mockReset()
  onOpenChangeMock.mockReset()
  rebuildMock.mockReset()
  refreshDecisionsMock.mockReset()
  toastSuccessMock.mockReset()
  toastErrorMock.mockReset()
  checkinAllMock.mockReset().mockResolvedValue({
    success: true,
    summary: { total: 2, success: 2, failed: 0, skipped: 0 },
  })
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

    const input = screen.getByPlaceholderText(PLACEHOLDER)
    fireEvent.change(input, { target: { value: 'schedul' } })

    await vi.waitFor(() => {
      expect(screen.getByText('Scheduled Tasks')).toBeInTheDocument()
    })
    // The local layer works without a backend response.
    expect(searchMock).not.toHaveBeenCalled()
  })

  it('navigates to the matched settings section on select', async () => {
    renderModal()

    const input = screen.getByPlaceholderText(PLACEHOLDER)
    fireEvent.change(input, { target: { value: 'operational events' } })

    const entry = await screen.findByText('Operational Events')
    fireEvent.click(entry)

    expect(navigateMock).toHaveBeenCalledWith({
      to: '/settings/operations/program-logs',
    })
  })
})

describe('search palette actions layer', () => {
  it('lists every registered action under the Actions heading on an empty query', () => {
    renderModal()

    expect(screen.getByText('Actions')).toBeInTheDocument()
    expect(screen.getByText('Add site')).toBeInTheDocument()
    expect(screen.getByText('Run all check-ins')).toBeInTheDocument()
    expect(screen.getByText('Auto-rebuild routes')).toBeInTheDocument()
    expect(screen.getByText('Refresh route decisions')).toBeInTheDocument()
    // Navigation quick entries keep rendering alongside the actions layer.
    expect(screen.getByText('Pages')).toBeInTheDocument()
  })

  it('renders bilingual action titles in zh-CN', async () => {
    // i18next resources are keyed by the interface code `zhCN`.
    await i18n.changeLanguage('zhCN')
    renderModal()

    expect(screen.getByText('操作')).toBeInTheDocument()
    expect(screen.getByText('添加站点')).toBeInTheDocument()
    expect(screen.getByText('运行所有签到')).toBeInTheDocument()
    expect(screen.getByText('自动重建路由')).toBeInTheDocument()
    expect(screen.getByText('刷新路由决策')).toBeInTheDocument()
  })

  it('deep-links the add-site action through the one-shot create param via keyboard', async () => {
    renderModal()

    const input = screen.getByPlaceholderText(PLACEHOLDER)
    fireEvent.change(input, { target: { value: 'add site' } })
    await screen.findByText('Add site')

    // Keyboard flow: the matched action is the selected item, Enter runs it.
    fireEvent.keyDown(input, { key: 'Enter' })

    expect(navigateMock).toHaveBeenCalledWith({
      to: '/sites',
      search: { create: true },
    })
    expect(onOpenChangeMock).toHaveBeenCalledWith(false)
  })

  it('matches actions by bilingual keywords the English label lacks', async () => {
    renderModal()

    const input = screen.getByPlaceholderText(PLACEHOLDER)
    fireEvent.change(input, { target: { value: '签到' } })

    const entry = await screen.findByText('Run all check-ins')
    fireEvent.click(entry)

    await vi.waitFor(() => {
      expect(checkinAllMock).toHaveBeenCalledTimes(1)
    })
    expect(onOpenChangeMock).toHaveBeenCalledWith(false)
    expect(toastSuccessMock).toHaveBeenCalledWith(
      'Check-in execution complete',
      expect.objectContaining({
        description: 'Total 2 accounts: 2 succeeded, 0 failed, 0 skipped',
      })
    )
  })

  it('reports a partial check-in failure as an error toast with the summary', async () => {
    checkinAllMock.mockResolvedValue({
      success: false,
      summary: { total: 3, success: 2, failed: 1, skipped: 0 },
    })
    renderModal()

    const input = screen.getByPlaceholderText(PLACEHOLDER)
    fireEvent.change(input, { target: { value: 'checkin' } })

    const entry = await screen.findByText('Run all check-ins')
    fireEvent.click(entry)

    await vi.waitFor(() => {
      expect(toastErrorMock).toHaveBeenCalledWith(
        'Check-in partially failed',
        expect.objectContaining({
          description: 'Total 3 accounts: 2 succeeded, 1 failed, 0 skipped',
        })
      )
    })
  })

  it('opens the rebuild confirmation before mutating (keyboard path)', async () => {
    renderModal()

    const input = screen.getByPlaceholderText(PLACEHOLDER)
    fireEvent.change(input, { target: { value: 'rebuild' } })
    await screen.findByText('Auto-rebuild routes')

    fireEvent.keyDown(input, { key: 'Enter' })

    // The palette closes and the same confirmation the routes page uses
    // takes over — the mutation must NOT have fired yet.
    await screen.findByText('Rebuild routes?')
    expect(onOpenChangeMock).toHaveBeenCalledWith(false)
    expect(rebuildMock).not.toHaveBeenCalled()

    fireEvent.click(screen.getByRole('button', { name: 'Auto-rebuild' }))

    expect(rebuildMock).toHaveBeenCalledWith({ refreshModels: true })
  })

  it('cancelling the rebuild confirmation never mutates', async () => {
    renderModal()

    const input = screen.getByPlaceholderText(PLACEHOLDER)
    fireEvent.change(input, { target: { value: 'rebuild' } })
    const entry = await screen.findByText('Auto-rebuild routes')
    fireEvent.click(entry)

    await screen.findByText('Rebuild routes?')
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))

    expect(rebuildMock).not.toHaveBeenCalled()
    expect(screen.queryByText('Rebuild routes?')).not.toBeInTheDocument()
  })

  it('fires refresh-route-decisions directly, without a confirmation', async () => {
    renderModal()

    const input = screen.getByPlaceholderText(PLACEHOLDER)
    fireEvent.change(input, { target: { value: 'decisions' } })

    const entry = await screen.findByText('Refresh route decisions')
    fireEvent.click(entry)

    expect(refreshDecisionsMock).toHaveBeenCalledTimes(1)
    expect(onOpenChangeMock).toHaveBeenCalledWith(false)
    expect(screen.queryByText('Rebuild routes?')).not.toBeInTheDocument()
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
          modelRequested: 'gpt-5.5',
          status: 'failed',
        },
      ],
    })
    renderModal()
    await typeQuery('gpt')

    fireEvent.click(await screen.findByText('gpt-5.5'))

    expect(navigateMock).toHaveBeenCalledWith({
      to: '/proxy-logs',
      search: { q: 'gpt-5.5', status: 'failed' },
    })
  })

  it('deep-links site hits with the one-shot edit param', async () => {
    searchMock.mockResolvedValue({
      ...EMPTY_RESPONSE,
      sites: [{ id: 3, name: 'NewAPI hub', url: 'https://newapi.example' }],
    })
    renderModal()
    await typeQuery('newapi')

    fireEvent.click(await screen.findByText('NewAPI hub'))

    expect(navigateMock).toHaveBeenCalledWith({
      to: '/sites',
      search: { edit: 3 },
    })
  })

  it('deep-links account hits with the one-shot accountId param', async () => {
    searchMock.mockResolvedValue({
      ...EMPTY_RESPONSE,
      accounts: [{ id: 9, username: 'alice', siteName: 'NewAPI hub' }],
    })
    renderModal()
    await typeQuery('alice')

    fireEvent.click(await screen.findByText('alice'))

    expect(navigateMock).toHaveBeenCalledWith({
      to: '/accounts',
      search: { accountId: 9 },
    })
  })

  it('deep-links token hits to the owning account when accountId is present', async () => {
    searchMock.mockResolvedValue({
      ...EMPTY_RESPONSE,
      accountTokens: [
        {
          id: 5,
          name: 'relay-key',
          accountId: 9,
          accountUsername: 'alice',
        },
      ],
    })
    renderModal()
    await typeQuery('relay')

    fireEvent.click(await screen.findByText('relay-key'))

    expect(navigateMock).toHaveBeenCalledWith({
      to: '/accounts',
      search: { accountId: 9 },
    })
  })

  it('falls back to the q-filtered accounts list for token hits without accountId', async () => {
    searchMock.mockResolvedValue({
      ...EMPTY_RESPONSE,
      accountTokens: [{ id: 6, name: 'legacy-key', accountUsername: 'bob' }],
    })
    renderModal()
    await typeQuery('legacy')

    fireEvent.click(await screen.findByText('legacy-key'))

    expect(navigateMock).toHaveBeenCalledWith({
      to: '/accounts',
      search: { q: 'legacy' },
    })
  })

  it('deep-links model hits with the one-shot model param', async () => {
    searchMock.mockResolvedValue({
      ...EMPTY_RESPONSE,
      models: [{ modelName: 'claude-opus-4.7.7', tokenCount: 3 }],
    })
    renderModal()
    await typeQuery('claude')

    fireEvent.click(await screen.findByText('claude-opus-4.7.7'))

    expect(navigateMock).toHaveBeenCalledWith({
      to: '/models',
      search: { model: 'claude-opus-4.7.7' },
    })
  })
})
