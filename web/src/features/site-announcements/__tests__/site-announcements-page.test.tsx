// Behavior tests for the site-announcements list page (#986 Lane C):
// row rendering with the site-name join, read/unread distinction, and the
// truthful loading / empty / error states.

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

import { SiteAnnouncementsPage } from '../components/site-announcements-page'

const { mockApi, mockToast } = vi.hoisted(() => ({
  mockApi: {
    getSiteAnnouncements: vi.fn(),
    getSites: vi.fn(),
    markSiteAnnouncementRead: vi.fn(),
    markAllSiteAnnouncementsRead: vi.fn(),
    clearSiteAnnouncements: vi.fn(),
    syncSiteAnnouncements: vi.fn(),
    getTask: vi.fn(),
  },
  mockToast: {
    success: vi.fn(),
    error: vi.fn(),
    info: vi.fn(),
    warning: vi.fn(),
  },
}))

vi.mock('@/lib/api', () => ({ api: mockApi }))
vi.mock('@/lib/toast', () => ({ toast: mockToast }))

function makeAnnouncement(overrides: Record<string, unknown> = {}) {
  return {
    id: 1,
    siteId: 7,
    platform: 'newapi',
    title: 'Scheduled maintenance',
    content: 'Upstream will be unreachable around midnight.',
    level: 'warning',
    sourceKey: 'notice-1',
    sourceUrl: 'https://upstream.example/notice/1',
    startsAt: null,
    endsAt: null,
    firstSeenAt: '2026-08-20T01:00:00Z',
    lastSeenAt: '2026-08-20T02:00:00Z',
    upstreamCreatedAt: null,
    upstreamUpdatedAt: null,
    readAt: null,
    dismissedAt: null,
    rawPayload: null,
    ...overrides,
  }
}

const SITES = [
  {
    id: 7,
    name: 'Alpha NewAPI',
    url: 'https://alpha.example',
    platform: 'newapi',
  },
  {
    id: 9,
    name: 'Beta Sub2API',
    url: 'https://beta.example',
    platform: 'sub2api',
  },
]

beforeAll(() => {
  // base-ui primitives query matchMedia on render; jsdom leaves it undefined.
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
  for (const fn of Object.values(mockApi)) fn.mockReset()
  for (const fn of Object.values(mockToast)) fn.mockReset()
  mockApi.getSites.mockResolvedValue(SITES)
  mockApi.markSiteAnnouncementRead.mockResolvedValue({ success: true })
  mockApi.markAllSiteAnnouncementsRead.mockResolvedValue({ success: true })
  mockApi.clearSiteAnnouncements.mockResolvedValue({ success: true })
})

afterEach(() => cleanup())

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: 0 },
      mutations: { retry: false },
    },
  })
  return render(
    (
      <QueryClientProvider client={queryClient}>
        <SiteAnnouncementsPage />
      </QueryClientProvider>
    ) as ReactElement
  )
}

describe('SiteAnnouncementsPage — list rendering', () => {
  it('renders rows with title, content preview, joined site name, platform and level badge', async () => {
    mockApi.getSiteAnnouncements.mockResolvedValue([
      makeAnnouncement(),
      makeAnnouncement({
        id: 2,
        siteId: 9,
        platform: 'sub2api',
        title: 'Price change',
        content: 'Rates go up next month.',
        level: 'info',
        readAt: '2026-08-21T00:00:00Z',
      }),
    ])
    renderPage()

    expect(await screen.findByText('Scheduled maintenance')).toBeInTheDocument()
    expect(
      screen.getByText('Upstream will be unreachable around midnight.')
    ).toBeInTheDocument()
    expect(screen.getByText('Alpha NewAPI')).toBeInTheDocument()
    expect(screen.getByText('Beta Sub2API')).toBeInTheDocument()
    expect(screen.getByText('newapi')).toBeInTheDocument()
    expect(screen.getByText('Warning')).toBeInTheDocument()
    expect(screen.getByText('Info')).toBeInTheDocument()

    // Unread row carries the unread marker + the per-row mark-read action;
    // the read row has neither.
    expect(screen.getByLabelText('Unread')).toBeInTheDocument()
    const markReadButtons = screen.getAllByRole('button', { name: 'Mark read' })
    expect(markReadButtons).toHaveLength(1)
  })

  it('shows a loading state before the first page resolves', async () => {
    let release: (value: unknown[]) => void = () => {}
    mockApi.getSiteAnnouncements.mockReturnValue(
      new Promise((resolve) => {
        release = resolve
      })
    )
    renderPage()

    expect(await screen.findByText('Site Announcements')).toBeInTheDocument()
    expect(document.querySelector('[aria-busy="true"]')).toBeInTheDocument()

    release([makeAnnouncement()])
    expect(await screen.findByText('Scheduled maintenance')).toBeInTheDocument()
    await waitFor(() => {
      expect(document.querySelector('[aria-busy="true"]')).toBeNull()
    })
  })

  it('shows the honest empty state when there are no announcements', async () => {
    mockApi.getSiteAnnouncements.mockResolvedValue([])
    renderPage()

    expect(await screen.findByText('No announcements yet')).toBeInTheDocument()
    expect(
      screen.getByText(
        'Use Sync now to fetch the latest notices from your upstream sites.'
      )
    ).toBeInTheDocument()
  })

  it('surfaces API failures in an error banner with a working Retry', async () => {
    mockApi.getSiteAnnouncements
      .mockRejectedValueOnce(new Error('boom'))
      .mockResolvedValue([makeAnnouncement()])
    renderPage()

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Failed to load site announcements: boom'
    )

    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(await screen.findByText('Scheduled maintenance')).toBeInTheDocument()
    expect(mockApi.getSiteAnnouncements).toHaveBeenCalledTimes(2)
  })
})
