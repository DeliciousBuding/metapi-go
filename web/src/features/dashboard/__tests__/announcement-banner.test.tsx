import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import type { ReactNode } from 'react'
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
import { api } from '@/lib/api'
import { productAnnouncementKeys } from '@/lib/product-announcements'
import { toast } from '@/lib/toast'

import { AnnouncementBanner } from '../components/announcement-banner'

vi.mock('@/lib/api', () => ({
  api: {
    getActiveAnnouncements: vi.fn(),
    dismissAnnouncement: vi.fn(),
  },
}))

vi.mock('@/lib/toast', () => ({
  toast: {
    error: vi.fn(),
  },
}))

const mockGetActive = vi.mocked(api.getActiveAnnouncements)
const mockDismiss = vi.mocked(api.dismissAnnouncement)
const mockToastError = vi.mocked(toast.error)

const active = {
  id: 1,
  title: 'Scheduled maintenance',
  message: 'Downtime expected tonight.',
  severity: 'warning' as const,
  link: null,
  enabled: true,
  dismissed: false,
  createdAt: '2026-08-12 00:00:00',
  updatedAt: '2026-08-12 00:00:00',
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
})

// The banner now reads announcements through TanStack Query so dashboard
// section switches reuse the cached list — each case gets a fresh client.
function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: 0, refetchOnWindowFocus: false },
    },
  })
}

function renderWithClient(ui: ReactNode) {
  const queryClient = createQueryClient()
  return {
    queryClient,
    ...render(
      <QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>
    ),
  }
}

beforeEach(() => {
  mockGetActive.mockReset()
  mockDismiss.mockReset()
  mockToastError.mockReset()
})

afterEach(() => cleanup())

describe('AnnouncementBanner', () => {
  it('renders active announcements and hides dismissed ones', async () => {
    mockGetActive.mockResolvedValue({
      items: [active, { ...active, id: 2, dismissed: true }],
    })

    renderWithClient(<AnnouncementBanner />)

    await waitFor(() => {
      expect(screen.getByText('Scheduled maintenance')).toBeInTheDocument()
    })
    expect(screen.getByText('Downtime expected tonight.')).toBeInTheDocument()
    expect(screen.getByRole('alert')).toBeInTheDocument()
  })

  it('renders only safe absolute http/https announcement links', async () => {
    mockGetActive.mockResolvedValue({
      items: [{ ...active, link: '  https://docs.example.com/status  ' }],
    })

    renderWithClient(<AnnouncementBanner />)

    const link = await screen.findByRole('link', { name: 'learn more' })
    expect(link).toHaveAttribute('href', 'https://docs.example.com/status')
    expect(link).toHaveAttribute('target', '_blank')
    expect(link).toHaveAttribute('rel', 'noopener noreferrer')
  })

  it.each([
    'javascript:alert(1)',
    'data:text/html,boom',
    'file:///tmp/notice',
    'ftp://example.com/notice',
    '//example.com/notice',
    '/relative/notice',
    'relative/notice',
    'not a url',
  ])('does not render an anchor for unsafe link %s', async (link) => {
    mockGetActive.mockResolvedValue({ items: [{ ...active, link }] })

    renderWithClient(<AnnouncementBanner />)

    await screen.findByText('Scheduled maintenance')
    expect(screen.queryByRole('link', { name: 'learn more' })).toBeNull()
  })

  it('renders null when there are no active announcements', async () => {
    mockGetActive.mockResolvedValue({ items: [] })

    const { container } = renderWithClient(<AnnouncementBanner />)

    await waitFor(() => {
      expect(mockGetActive).toHaveBeenCalledTimes(1)
    })
    expect(container).toBeEmptyDOMElement()
  })

  it('reuses the cached announcements on remount within staleTime', async () => {
    mockGetActive.mockResolvedValue({ items: [active] })

    const queryClient = createQueryClient()
    const renderBanner = () =>
      render(
        <QueryClientProvider client={queryClient}>
          <AnnouncementBanner />
        </QueryClientProvider>
      )

    const firstMount = renderBanner()
    await waitFor(() => {
      expect(screen.getByRole('alert')).toBeInTheDocument()
    })
    firstMount.unmount()

    // Second mount within the 60s stale window must not refetch.
    renderBanner()
    await waitFor(() => {
      expect(screen.getByRole('alert')).toBeInTheDocument()
    })
    expect(mockGetActive).toHaveBeenCalledTimes(1)
  })

  it('persists dismissal via the API and removes the banner', async () => {
    mockGetActive.mockResolvedValue({ items: [active] })
    mockDismiss.mockResolvedValue({ success: true })

    const { queryClient } = renderWithClient(<AnnouncementBanner />)

    await waitFor(() => {
      expect(screen.getByRole('alert')).toBeInTheDocument()
    })

    fireEvent.click(
      screen.getByRole('button', { name: 'Dismiss announcement' })
    )

    await waitFor(() => {
      expect(mockDismiss).toHaveBeenCalledWith(1)
    })
    await waitFor(() => {
      expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    })
    // The cached list is patched in place so a remount stays dismissed.
    expect(queryClient.getQueryData(productAnnouncementKeys.active())).toEqual(
      []
    )
  })

  it('keeps the banner visible when dismissal fails', async () => {
    mockGetActive.mockResolvedValue({ items: [active] })
    mockDismiss.mockRejectedValue(new Error('boom'))

    renderWithClient(<AnnouncementBanner />)

    await waitFor(() => {
      expect(screen.getByRole('alert')).toBeInTheDocument()
    })

    fireEvent.click(
      screen.getByRole('button', { name: 'Dismiss announcement' })
    )

    await waitFor(() => {
      expect(mockDismiss).toHaveBeenCalledWith(1)
      expect(mockToastError).toHaveBeenCalledWith(
        'Failed to dismiss announcement. Try again.'
      )
    })
    expect(screen.getByRole('alert')).toBeInTheDocument()
    expect(screen.getByText('Scheduled maintenance')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Dismiss announcement' })
    ).toBeEnabled()
  })
})
