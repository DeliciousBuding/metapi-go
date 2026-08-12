import '@testing-library/jest-dom/vitest'
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
import { api } from '@/lib/api'

import { AnnouncementBanner } from '../components/announcement-banner'

vi.mock('@/lib/api', () => ({
  api: {
    getActiveAnnouncements: vi.fn(),
    dismissAnnouncement: vi.fn(),
  },
}))

const mockGetActive = vi.mocked(api.getActiveAnnouncements)
const mockDismiss = vi.mocked(api.dismissAnnouncement)

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

beforeEach(() => {
  mockGetActive.mockReset()
  mockDismiss.mockReset()
})

afterEach(() => cleanup())

describe('AnnouncementBanner', () => {
  it('renders active announcements and hides dismissed ones', async () => {
    mockGetActive.mockResolvedValue({
      items: [active, { ...active, id: 2, dismissed: true }],
    })

    render(<AnnouncementBanner />)

    await waitFor(() => {
      expect(screen.getByText('Scheduled maintenance')).toBeInTheDocument()
    })
    expect(screen.getByText('Downtime expected tonight.')).toBeInTheDocument()
    expect(screen.getByRole('alert')).toBeInTheDocument()
  })

  it('renders null when there are no active announcements', async () => {
    mockGetActive.mockResolvedValue({ items: [] })

    const { container } = render(<AnnouncementBanner />)

    await waitFor(() => {
      expect(mockGetActive).toHaveBeenCalledTimes(1)
    })
    expect(container).toBeEmptyDOMElement()
  })

  it('persists dismissal via the API and removes the banner', async () => {
    mockGetActive.mockResolvedValue({ items: [active] })
    mockDismiss.mockResolvedValue({ success: true })

    render(<AnnouncementBanner />)

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
  })

  it('keeps the banner visible when dismissal fails', async () => {
    mockGetActive.mockResolvedValue({ items: [active] })
    mockDismiss.mockRejectedValue(new Error('boom'))

    render(<AnnouncementBanner />)

    await waitFor(() => {
      expect(screen.getByRole('alert')).toBeInTheDocument()
    })

    fireEvent.click(
      screen.getByRole('button', { name: 'Dismiss announcement' })
    )

    await waitFor(() => {
      expect(mockDismiss).toHaveBeenCalledWith(1)
    })
    expect(screen.getByRole('alert')).toBeInTheDocument()
    expect(screen.getByText('Scheduled maintenance')).toBeInTheDocument()
  })
})
