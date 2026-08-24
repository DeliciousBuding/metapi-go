// Security tests for the site-announcements page (#986 Lane C): announcement
// title / content / source are UNTRUSTED upstream data. The body must render
// as plain text (no HTML injection, no markdown), and sourceUrl must never
// become a clickable external link.

import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen } from '@testing-library/react'
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

const { mockApi } = vi.hoisted(() => ({
  mockApi: {
    getSiteAnnouncements: vi.fn(),
    getSites: vi.fn(),
    markSiteAnnouncementRead: vi.fn(),
    markAllSiteAnnouncementsRead: vi.fn(),
    clearSiteAnnouncements: vi.fn(),
    syncSiteAnnouncements: vi.fn(),
    getTask: vi.fn(),
  },
}))

vi.mock('@/lib/api', () => ({ api: mockApi }))
vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn(), info: vi.fn(), warning: vi.fn() },
}))

const HOSTILE_TITLE = '<img src=x onerror=alert(1)>'
const HOSTILE_CONTENT =
  '<script>alert(1)</script> and <b onmouseover=alert(2)>hover me</b>'
const HOSTILE_SOURCE_URL = 'https://evil.example/phish'

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
  for (const fn of Object.values(mockApi)) fn.mockReset()
  mockApi.getSites.mockResolvedValue([
    {
      id: 7,
      name: 'Alpha NewAPI',
      url: 'https://alpha.example',
      platform: 'newapi',
    },
  ])
  mockApi.getSiteAnnouncements.mockResolvedValue([
    {
      id: 1,
      siteId: 7,
      platform: 'newapi',
      title: HOSTILE_TITLE,
      content: HOSTILE_CONTENT,
      level: 'info',
      sourceKey: 'notice-evil',
      sourceUrl: HOSTILE_SOURCE_URL,
      startsAt: null,
      endsAt: null,
      firstSeenAt: '2026-08-20T01:00:00Z',
      lastSeenAt: '2026-08-20T01:00:00Z',
      upstreamCreatedAt: null,
      upstreamUpdatedAt: null,
      readAt: null,
      dismissedAt: null,
      rawPayload: null,
    },
  ])
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

describe('SiteAnnouncementsPage — untrusted upstream content', () => {
  it('renders hostile title and content as literal text, never as elements', async () => {
    const { container } = renderPage()

    // The raw markup strings appear verbatim as text nodes…
    expect(await screen.findByText(HOSTILE_TITLE)).toBeInTheDocument()
    expect(screen.getByText(HOSTILE_CONTENT)).toBeInTheDocument()

    // …and no injected elements exist anywhere in the page.
    expect(container.querySelector('img')).toBeNull()
    expect(container.querySelector('script')).toBeNull()
    expect(container.querySelector('b')).toBeNull()
  })

  it('never renders sourceUrl as a clickable external link', async () => {
    const { container } = renderPage()

    await screen.findByText(HOSTILE_TITLE)

    const anchors = [...container.querySelectorAll('a[href]')]
    expect(
      anchors.some(
        (anchor) => anchor.getAttribute('href') === HOSTILE_SOURCE_URL
      )
    ).toBe(false)
    expect(container.textContent).not.toContain(HOSTILE_SOURCE_URL)
  })
})
