// Behavior tests for the site-announcements actions (#986 Lane C):
// mark-read / mark-all-read, the ConfirmDialog guard on the destructive
// clear, and the sync-now background task observed through api.getTask.

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

const { mockApi, mockToast, mockRouter } = vi.hoisted(() => ({
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
  mockRouter: { navigate: vi.fn() },
}))

vi.mock('@/lib/api', () => ({ api: mockApi }))
vi.mock('@/lib/toast', () => ({ toast: mockToast }))
// The page reads its filters/page cursor from the URL (W19-T1 P2-l); the
// tests mount it bare, so stub the router hooks with a default search.
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => mockRouter.navigate,
  useSearch: () => ({}),
}))

function makeAnnouncement(overrides: Record<string, unknown> = {}) {
  return {
    id: 1,
    siteId: 7,
    platform: 'newapi',
    title: 'Scheduled maintenance',
    content: 'Upstream will be unreachable around midnight.',
    level: 'info',
    sourceKey: 'notice-1',
    sourceUrl: null,
    startsAt: null,
    endsAt: null,
    firstSeenAt: '2026-08-20T01:00:00Z',
    lastSeenAt: '2026-08-20T01:00:00Z',
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
]

const runningSyncTask = {
  id: 'sync-task-1',
  type: 'site-announcements-sync',
  title: 'Sync site announcements',
  status: 'running',
  message: 'Syncing 1 site',
  error: null,
  result: null,
  createdAt: '2026-08-20T10:00:00Z',
  updatedAt: '2026-08-20T10:00:01Z',
}

const succeededSyncTask = {
  ...runningSyncTask,
  status: 'succeeded',
  message: 'Sync finished',
  finishedAt: '2026-08-20T10:00:05Z',
  result: {
    scannedSites: 1,
    inserted: 3,
    updated: 2,
    unsupported: 0,
    notifications: 0,
    events: 3,
    failed: 0,
    failedSites: [],
  },
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
  for (const fn of Object.values(mockApi)) fn.mockReset()
  for (const fn of Object.values(mockToast)) fn.mockReset()
  mockApi.getSites.mockResolvedValue(SITES)
  mockApi.getSiteAnnouncements.mockResolvedValue([makeAnnouncement()])
  mockApi.markSiteAnnouncementRead.mockResolvedValue({ success: true })
  mockApi.markAllSiteAnnouncementsRead.mockResolvedValue({ success: true })
  mockApi.clearSiteAnnouncements.mockResolvedValue({ success: true })
  mockApi.syncSiteAnnouncements.mockResolvedValue({
    success: true,
    queued: true,
    reused: false,
    taskId: 'sync-task-1',
  })
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

describe('SiteAnnouncementsPage — actions', () => {
  it('marks a single announcement read through the row action', async () => {
    renderPage()

    const markRead = await screen.findByRole('button', { name: 'Mark read' })
    fireEvent.click(markRead)

    await waitFor(() => {
      expect(mockApi.markSiteAnnouncementRead).toHaveBeenCalledWith(1)
    })
  })

  it('marks all announcements read', async () => {
    renderPage()

    // Wait for the page to resolve: Mark-all-read stays disabled while no
    // unread rows are visible.
    await screen.findByText('Scheduled maintenance')
    const markAll = screen.getByRole('button', { name: 'Mark all read' })
    fireEvent.click(markAll)

    await waitFor(() => {
      expect(mockApi.markAllSiteAnnouncementsRead).toHaveBeenCalled()
    })
    await waitFor(() => {
      expect(mockToast.success).toHaveBeenCalledWith(
        'All announcements marked as read.'
      )
    })
  })

  it('clearing all announcements requires the destructive confirm dialog', async () => {
    renderPage()

    const clearButton = await screen.findByRole('button', { name: 'Clear all' })
    fireEvent.click(clearButton)

    expect(mockApi.clearSiteAnnouncements).not.toHaveBeenCalled()
    expect(
      await screen.findByText('Clear all announcements?')
    ).toBeInTheDocument()

    // The dialog action shares the trigger label and is the last match.
    const confirmButtons = screen.getAllByRole('button', { name: 'Clear all' })
    const confirmButton = confirmButtons.at(-1)
    if (!confirmButton) throw new Error('confirm button not rendered')
    fireEvent.click(confirmButton)

    await waitFor(() => {
      expect(mockApi.clearSiteAnnouncements).toHaveBeenCalled()
    })
    await waitFor(() => {
      expect(mockToast.success).toHaveBeenCalledWith(
        'All announcements cleared.'
      )
    })
  })

  it(
    'sync now queues the background task, polls api.getTask and surfaces the result',
    { timeout: 15_000 },
    async () => {
      mockApi.getTask
        .mockResolvedValueOnce({ success: true, task: runningSyncTask })
        .mockResolvedValueOnce({ success: true, task: succeededSyncTask })
      renderPage()

      const syncButton = await screen.findByRole('button', { name: 'Sync now' })
      fireEvent.click(syncButton)

      await waitFor(() => {
        expect(mockApi.syncSiteAnnouncements).toHaveBeenCalledWith(undefined)
      })
      await waitFor(() => {
        expect(mockApi.getTask).toHaveBeenCalledWith('sync-task-1')
      })

      // Poll interval is 2s; wait for the terminal poll to land and toast.
      await waitFor(
        () => {
          expect(mockToast.success).toHaveBeenCalledWith(
            'Sync finished: 3 new, 2 updated.'
          )
        },
        { timeout: 6000 }
      )

      // The list is invalidated after a successful sync (refetched).
      await waitFor(() => {
        expect(mockApi.getSiteAnnouncements.mock.calls.length).toBeGreaterThan(
          1
        )
      })
    }
  )

  it(
    'surfaces the sync task error on failure',
    { timeout: 15_000 },
    async () => {
      mockApi.getTask
        .mockResolvedValueOnce({ success: true, task: runningSyncTask })
        .mockResolvedValueOnce({
          success: true,
          task: {
            ...runningSyncTask,
            status: 'failed',
            error: 'upstream unreachable',
            finishedAt: '2026-08-20T10:00:05Z',
          },
        })
      renderPage()

      fireEvent.click(await screen.findByRole('button', { name: 'Sync now' }))

      await waitFor(
        () => {
          expect(mockToast.error).toHaveBeenCalledWith('upstream unreachable')
        },
        { timeout: 6000 }
      )
    }
  )
})
