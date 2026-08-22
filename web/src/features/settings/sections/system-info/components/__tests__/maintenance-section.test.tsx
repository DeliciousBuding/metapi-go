// Regression test for the maintenance section's usage purge. "Clear usage
// logs" is an irreversible delete and must open a destructive confirmation
// before the mutation fires (issue #889).
import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
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

import { MaintenanceSection } from '../maintenance-section'

const {
  mockClearRuntimeCache,
  mockClearUsageData,
  mockToastSuccess,
  mockToastError,
} = vi.hoisted(() => ({
  mockClearRuntimeCache: vi.fn(),
  mockClearUsageData: vi.fn(),
  mockToastSuccess: vi.fn(),
  mockToastError: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  api: {
    clearRuntimeCache: mockClearRuntimeCache,
    clearUsageData: mockClearUsageData,
  },
}))

vi.mock('@/lib/toast', () => ({
  toast: {
    success: mockToastSuccess,
    error: mockToastError,
    info: vi.fn(),
    warning: vi.fn(),
  },
}))

// The section only uses the router Link; stub it so the test needs no router.
// className passes through so hit-area assertions can read it.
vi.mock('@tanstack/react-router', () => ({
  Link: (props: { children?: React.ReactNode; className?: string }) => (
    <a href='#' className={props.className}>
      {props.children}
    </a>
  ),
}))

beforeAll(() => {
  // base-ui AlertDialog queries matchMedia under jsdom.
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
  mockClearRuntimeCache.mockReset()
  mockClearUsageData.mockReset()
  mockToastSuccess.mockReset()
  mockToastError.mockReset()
  mockClearRuntimeCache.mockResolvedValue({ success: true })
  mockClearUsageData.mockResolvedValue({ success: true })
})

afterEach(() => cleanup())

function renderMaintenanceSection() {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <MaintenanceSection />
    </QueryClientProvider>
  )
}

// The confirm dialog action shares the trigger's label; the dialog's action
// is the last matching button in the tree.
function clickConfirmAction() {
  const confirmButtons = screen.getAllByRole('button', {
    name: 'Clear usage logs',
  })
  const confirmButton = confirmButtons.at(-1)
  if (!confirmButton) {
    throw new Error('Confirm action button not found')
  }
  fireEvent.click(confirmButton)
}

describe('MaintenanceSection — clear usage guard', () => {
  it('requires a destructive confirmation before clearing usage data', async () => {
    renderMaintenanceSection()

    fireEvent.click(
      await screen.findByRole('button', { name: 'Clear usage logs' })
    )

    expect(await screen.findByText('Clear usage logs?')).toBeInTheDocument()
    expect(
      screen.getByText(
        'This permanently deletes all recorded usage data. This action cannot be undone.'
      )
    ).toBeInTheDocument()
    expect(mockClearUsageData).not.toHaveBeenCalled()

    clickConfirmAction()

    await waitFor(() => {
      expect(mockClearUsageData).toHaveBeenCalledTimes(1)
    })
  })

  it('does not clear usage data when the confirmation is cancelled', async () => {
    renderMaintenanceSection()

    fireEvent.click(
      await screen.findByRole('button', { name: 'Clear usage logs' })
    )
    await screen.findByText('Clear usage logs?')

    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))

    expect(mockClearUsageData).not.toHaveBeenCalled()
  })

  it('keeps the cache rebuild a one-click, non-destructive action', async () => {
    renderMaintenanceSection()

    fireEvent.click(
      await screen.findByRole('button', {
        name: 'Clear cache & rebuild routes',
      })
    )

    await waitFor(() => {
      expect(mockClearRuntimeCache).toHaveBeenCalledTimes(1)
    })
  })
})

// WCAG 2.5.8 closeout (F-line residual E): the "View Program Logs" hint is a
// text-xs inline link whose ~16px line measured as a TINY-HIT. Inline text
// links are exempt from 2.5.8, so this is best-effort: `py-1` click padding
// widens the hit area to ~24px without moving the line (vertical padding on
// an inline element never grows the line box).
describe('MaintenanceSection — program-logs link hit area', () => {
  it('adds vertical click padding to the inline program-logs link', () => {
    renderMaintenanceSection()

    const programLogsLink = screen.getByRole('link', {
      name: 'View Program Logs',
    })

    expect(programLogsLink.classList).toContain('py-1')
  })
})
