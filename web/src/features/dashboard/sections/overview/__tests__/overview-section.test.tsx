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
import { cleanup, render, screen, waitFor } from '@testing-library/react'
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

const mockGetDashboardSnapshot = vi.mocked(api.getDashboardSnapshot)
const mockGetBalanceHistory = vi.mocked(api.getBalanceHistory)
const mockGetSchedulerStatus = vi.mocked(api.getSchedulerStatus)

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
