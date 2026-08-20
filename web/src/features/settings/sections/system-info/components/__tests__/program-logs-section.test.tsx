// Behavior test for the program-logs Clear confirmation (#889): clearing all
// operational events is destructive and must go through a ConfirmDialog —
// the Clear button alone never fires the mutation.

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

import { ProgramLogsSection } from '../program-logs-section'

const { mockGetEvents, mockClearEvents } = vi.hoisted(() => ({
  mockGetEvents: vi.fn(),
  mockClearEvents: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  api: {
    getEvents: mockGetEvents,
    clearEvents: mockClearEvents,
    markEventRead: vi.fn().mockResolvedValue({ success: true }),
    markAllEventsRead: vi.fn().mockResolvedValue({ success: true }),
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
  mockGetEvents.mockReset()
  mockClearEvents.mockReset()
  mockClearEvents.mockResolvedValue({ success: true })
  mockGetEvents.mockResolvedValue({
    items: [
      {
        id: 1,
        type: 'checkin',
        title: 'Checkin finished',
        message: 'ok',
        level: 'info',
        read: false,
        createdAt: '2026-01-01T00:00:00Z',
      },
    ],
    total: 1,
  })
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
        <ProgramLogsSection />
      </QueryClientProvider>
    ) as ReactElement
  )
}

describe('ProgramLogsSection clear confirmation', () => {
  it('Clear requires confirmation before the clear mutation fires', async () => {
    renderSection()

    const clearButton = await screen.findByRole('button', { name: 'Clear' })
    fireEvent.click(clearButton)

    expect(mockClearEvents).not.toHaveBeenCalled()

    // The destructive confirmation dialog appears; confirming fires the api.
    const confirmButtons = await screen.findAllByRole('button', {
      name: 'Clear',
    })
    const confirmClearButton = confirmButtons.at(-1)
    if (!confirmClearButton) throw new Error('confirm button not rendered')
    fireEvent.click(confirmClearButton)

    await waitFor(() => {
      expect(mockClearEvents).toHaveBeenCalledTimes(1)
    })
  })

  it('cancelling the confirmation does not clear events', async () => {
    renderSection()

    const clearButton = await screen.findByRole('button', { name: 'Clear' })
    fireEvent.click(clearButton)

    fireEvent.click(
      await screen.findByRole('button', { name: 'Cancel' })
    )

    expect(mockClearEvents).not.toHaveBeenCalled()
  })
})
