// Security tests for the site-announcements page (#986): announcement
// Titles and previews stay literal. Expanded Markdown must not execute HTML
// or unsafe links; source navigation resolves against the trusted Site URL.

import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
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
    getSiteAnnouncementHistory: vi.fn(),
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
// The page reads its filters/page cursor from the URL; the
// tests mount it bare, so stub the router hooks with a default search.
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => vi.fn(),
  useSearch: () => ({}),
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

  it('rejects a hostile sourceUrl and falls back to the trusted Site home', async () => {
    const { container } = renderPage()

    await screen.findByText(HOSTILE_TITLE)

    const anchors = [...container.querySelectorAll('a[href]')]
    expect(
      anchors.some(
        (anchor) => anchor.getAttribute('href') === HOSTILE_SOURCE_URL
      )
    ).toBe(false)
    expect(container.textContent).not.toContain(HOSTILE_SOURCE_URL)
    const trusted = anchors.find(
      (anchor) => anchor.getAttribute('href') === 'https://alpha.example/'
    )
    expect(trusted).toBeDefined()
    expect(trusted).toHaveAttribute('target', '_blank')
    expect(trusted).toHaveAttribute('rel', 'noopener noreferrer')
  })

  it('renders Markdown tables and code inline while blocking executable HTML and links', async () => {
    const markdown =
      '# 服务更新\n\n[查看文档](https://docs.example/notice)\n\n| 模型 | 状态 |\n| --- | --- |\n| 示例模型 | 可用 |\n\n```json\n{"ready":true}\n```\n\n<script>alert(1)</script>\n\n[危险链接](javascript:alert(1))'
    const current = {
      id: 1,
      siteId: 7,
      title: '更新说明',
      content: markdown,
      firstSeenAt: '2026-10-18T00:00:00Z',
      lastSeenAt: '2026-10-18T01:00:00Z',
      platform: 'newapi',
      level: 'info',
      revisionId: 0,
    }
    mockApi.getSiteAnnouncements.mockResolvedValue([current])
    mockApi.getSiteAnnouncementHistory.mockResolvedValue([current])
    const { container } = renderPage()
    fireEvent.click(await screen.findByRole('button', { name: '更新说明' }))
    expect(
      await screen.findByRole('heading', { name: '服务更新' })
    ).toBeInTheDocument()
    expect(screen.getByRole('link', { name: '查看文档' })).toHaveAttribute(
      'href',
      'https://docs.example/notice'
    )
    expect(screen.getByRole('cell', { name: '示例模型' })).toBeInTheDocument()
    expect(container.querySelector('pre code')).toHaveTextContent(
      '{"ready":true}'
    )
    expect(container.querySelector('script')).toBeNull()
    expect(container.querySelector('a[href^="javascript:"]')).toBeNull()
  })
})
