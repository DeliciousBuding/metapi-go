// Behavior test for the dashboard overview onboarding banner.
//
// Closes the first-time-landing dead-end (user-perspective #1): a first-time
// user with zero sites sees a "Create site" CTA that deep-links to /sites.
// Asserts:
//   (a) the banner + CTA render when the dashboard snapshot reports zero
//       sites, and the CTA points at /sites;
//   (b) the banner is absent once the user has at least one site;
//   (c) the banner does not flash while the snapshot is still loading
//       (siteCount undefined) — no false "empty state" before the first byte.
//
// The dashboard snapshot query (api.getDashboardSnapshot) is the SOLE signal
// for the empty state — no extra query is added. Sibling overview widgets
// (AnnouncementBanner, TodaySnapshotStrip, StatCard) are stubbed so the test
// exercises the OverviewSection banner wiring in isolation; each sibling has
// its own coverage. TodaySnapshotStrip pulls a live WebSocket (useRealtimeOps)
// and StatCard uses requestAnimationFrame (CountUp) + recharts — both are
// browser/animation boundaries outside the banner behavior under test.

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
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'
import { api } from '@/lib/api'

import { OverviewSection } from '../overview-section'

vi.mock('@/lib/api', () => ({
  api: {
    getDashboardSnapshot: vi.fn(),
    getBalanceHistory: vi.fn(),
    getSchedulerStatus: vi.fn(),
    probeModelsNow: vi.fn(),
  },
}))

vi.mock('@tanstack/react-router', () => ({
  Link: ({ to, children }: { to?: string; children: ReactNode }) => (
    <a href={to} data-testid='router-link'>
      {children}
    </a>
  ),
}))

vi.mock('@/features/dashboard/components/today-snapshot', () => ({
  TodaySnapshotStrip: () => null,
}))
vi.mock('@/features/dashboard/components/announcement-banner', () => ({
  AnnouncementBanner: () => null,
}))
vi.mock('@/features/dashboard/components/stat-card', () => ({
  StatCard: () => <div data-testid='stat-card' />,
}))

const { mockToastSuccess } = vi.hoisted(() => ({
  mockToastSuccess: vi.fn(),
}))
vi.mock('@/lib/toast', () => ({
  toast: {
    success: mockToastSuccess,
    error: vi.fn(),
    warning: vi.fn(),
    info: vi.fn(),
  },
}))

const mockGetDashboardSnapshot = vi.mocked(api.getDashboardSnapshot)
const mockGetBalanceHistory = vi.mocked(api.getBalanceHistory)
const mockGetSchedulerStatus = vi.mocked(api.getSchedulerStatus)
const mockProbeModelsNow = vi.mocked(api.probeModelsNow)

function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: 0, refetchOnWindowFocus: false },
    },
  })
}

function renderWithClient(ui: ReactNode) {
  const queryClient = createQueryClient()
  return render(
    <QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>
  )
}

beforeEach(() => {
  mockGetDashboardSnapshot.mockReset()
  mockGetBalanceHistory.mockReset()
  mockGetSchedulerStatus.mockReset()
  mockProbeModelsNow.mockReset()
  // Non-banner queries resolve to empty-but-valid shapes so the section
  // renders without throwing; the snapshot is overridden per test.
  mockGetBalanceHistory.mockResolvedValue({ series: [], days: 8 })
  mockGetSchedulerStatus.mockResolvedValue({ items: [], generatedAt: '' })
})

afterEach(() => cleanup())

describe('OverviewSection onboarding banner', () => {
  it('renders the banner + "Create site" CTA when there are zero sites', async () => {
    mockGetDashboardSnapshot.mockResolvedValue({ siteCount: 0 })

    renderWithClient(<OverviewSection />)

    // Wait for the snapshot query to resolve and the banner heading to mount.
    const heading = await screen.findByRole('heading', {
      name: 'Welcome to Metapi',
      level: 2,
    })
    expect(heading).toBeInTheDocument()
    expect(
      screen.getByText('Create your first site to start aggregating AI APIs.')
    ).toBeInTheDocument()

    // StatCard links are stubbed, so the only link on the page is the CTA.
    const cta = screen.getByRole('link', { name: /Create site/i })
    expect(cta).toHaveAttribute('href', '/sites')
  })

  it('hides the banner once the user has at least one site', async () => {
    mockGetDashboardSnapshot.mockResolvedValue({ siteCount: 3 })

    renderWithClient(<OverviewSection />)

    await waitFor(() => {
      expect(mockGetDashboardSnapshot).toHaveBeenCalledTimes(1)
    })

    await waitFor(() => {
      expect(
        screen.queryByRole('heading', { name: 'Welcome to Metapi' })
      ).not.toBeInTheDocument()
    })
    expect(
      screen.queryByRole('link', { name: /Create site/i })
    ).not.toBeInTheDocument()
  })

  it('does not flash the banner while the snapshot is still loading', async () => {
    // Never resolves — simulates the loading window before the first byte
    // lands. siteCount stays undefined, so the banner must not render.
    mockGetDashboardSnapshot.mockImplementation(
      () => new Promise<{ siteCount: number }>(() => {})
    )

    renderWithClient(<OverviewSection />)

    expect(
      screen.queryByRole('heading', { name: 'Welcome to Metapi' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: /Create site/i })
    ).not.toBeInTheDocument()
  })
})

describe('OverviewSection load-error branch', () => {
  it('surfaces a snapshot failure with an error banner and a working Retry', async () => {
    mockGetDashboardSnapshot
      .mockRejectedValueOnce(new Error('boom'))
      .mockResolvedValue({ siteCount: 3 })

    renderWithClient(<OverviewSection />)

    // The failure is no longer silent (W19-T1 A4#11): a banner replaces the
    // old "—" stat cards until Retry recovers the query.
    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent(
      'Failed to load the dashboard overview: boom'
    )

    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    await waitFor(() => {
      expect(mockGetDashboardSnapshot).toHaveBeenCalledTimes(2)
    })
    await waitFor(() => {
      expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    })
  })

  it('surfaces a balance-history failure even when the snapshot succeeds', async () => {
    mockGetDashboardSnapshot.mockResolvedValue({ siteCount: 3 })
    mockGetBalanceHistory.mockRejectedValue(new Error('spark down'))

    renderWithClient(<OverviewSection />)

    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent(
      'Failed to load the balance history: spark down'
    )
  })
})

describe('OverviewSection model-probe scheduler card', () => {
  it('offers a manual trigger for the model-probe job and queues it', async () => {
    mockGetDashboardSnapshot.mockResolvedValue({ siteCount: 3 })
    mockGetSchedulerStatus.mockResolvedValue({
      items: [
        {
          job: 'model-probe',
          enabled: true,
          lastStatus: 'success',
          runs24h: 2,
          success24h: 2,
        },
        {
          job: 'checkin',
          enabled: true,
          lastStatus: 'success',
          runs24h: 1,
          success24h: 1,
        },
      ],
      generatedAt: '',
    })
    mockProbeModelsNow.mockResolvedValue({
      success: true,
      queued: true,
      reused: false,
      jobId: 'probe-1',
      status: 'pending',
    })

    renderWithClient(<OverviewSection />)

    const trigger = await screen.findByRole('button', {
      name: 'Run now',
    })
    expect(trigger).toBeInTheDocument()
    // Only model-probe exposes a manual trigger — other jobs stay honest.
    expect(screen.getAllByRole('button', { name: 'Run now' })).toHaveLength(1)

    fireEvent.click(trigger)
    await waitFor(() => {
      expect(mockProbeModelsNow).toHaveBeenCalledTimes(1)
    })
    await waitFor(() => {
      expect(mockToastSuccess).toHaveBeenCalledWith(
        'Model availability probe queued — recent runs appear here when the pass completes.'
      )
    })
  })

  it('shows the recent runs list when expanded, with honest failure verdicts', async () => {
    mockGetDashboardSnapshot.mockResolvedValue({ siteCount: 3 })
    mockGetSchedulerStatus.mockResolvedValue({
      items: [
        {
          job: 'model-probe',
          enabled: true,
          lastStatus: 'failed',
          runs24h: 2,
          success24h: 1,
          recentRuns: [
            {
              startedAt: '2026-08-23T01:00:00Z',
              completedAt: '2026-08-23T01:00:05Z',
              accountsConsidered: 2,
              accountsProbed: 2,
              targetsScanned: 3,
              success: 2,
              failed: 1,
              inconclusive: 0,
              skipped: 0,
            },
            {
              startedAt: '2026-08-23T00:30:00Z',
              completedAt: '2026-08-23T00:30:04Z',
              accountsConsidered: 2,
              accountsProbed: 2,
              targetsScanned: 2,
              success: 2,
              failed: 0,
              inconclusive: 0,
              skipped: 0,
            },
          ],
        },
      ],
      generatedAt: '',
    })

    renderWithClient(<OverviewSection />)

    const toggle = await screen.findByRole('button', {
      name: 'Recent runs',
    })
    fireEvent.click(toggle)

    // The failed pass carries the destructive verdict; the clean pass stays
    // success — the list never hides a failed run behind a green badge.
    expect(await screen.findByText('Failed')).toBeInTheDocument()
    expect(screen.getByText('Completed')).toBeInTheDocument()
    expect(screen.getByText('3 targets')).toBeInTheDocument()
    expect(
      screen.getByText('ok 2 / failed 1 / inconclusive 0 / skipped 0')
    ).toBeInTheDocument()
  })

  it('hides the trigger when the model-probe scheduler is disabled', async () => {
    mockGetDashboardSnapshot.mockResolvedValue({ siteCount: 3 })
    mockGetSchedulerStatus.mockResolvedValue({
      items: [
        {
          job: 'model-probe',
          enabled: false,
          lastStatus: 'never',
          note: 'not enabled (MODEL_AVAILABILITY_PROBE_ENABLED)',
          runs24h: 0,
          success24h: 0,
        },
      ],
      generatedAt: '',
    })

    renderWithClient(<OverviewSection />)

    await waitFor(() =>
      expect(
        screen.queryByRole('button', { name: 'Run now' })
      ).not.toBeInTheDocument()
    )
  })
})
