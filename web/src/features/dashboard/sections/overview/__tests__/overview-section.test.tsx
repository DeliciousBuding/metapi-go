// Onboarding, resource error handling and maintenance controls. Independent
// request/insight panels are tested beside their components.
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
import { reportFixture } from './report-fixture'

vi.mock('@/lib/api', () => ({
  api: {
    getDashboardSnapshot: vi.fn(),
    getOverviewReport: vi.fn(),
    getSchedulerStatus: vi.fn(),
    probeModelsNow: vi.fn(),
    getDownstreamApiKeys: vi.fn(),
  },
}))

// The onboarding checklist reads its route count off the token-routes summary
// query; the key count comes from the api mock above.
const onboardingState = vi.hoisted(() => ({
  routes: [] as unknown[],
}))
vi.mock('@/features/token-routes', () => ({
  useRoutes: () => ({ data: onboardingState.routes }),
}))

vi.mock('@tanstack/react-router', () => ({
  Link: ({ to, children }: { to?: string; children: ReactNode }) => (
    <a href={to} data-testid='router-link'>
      {children}
    </a>
  ),
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
const mockGetSchedulerStatus = vi.mocked(api.getSchedulerStatus)
const mockProbeModelsNow = vi.mocked(api.probeModelsNow)
const mockGetDownstreamApiKeys = vi.mocked(api.getDownstreamApiKeys)

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
  localStorage.removeItem('metapi.overview.period')
  vi.mocked(api.getOverviewReport)
    .mockReset()
    .mockResolvedValue(reportFixture())
  mockGetDashboardSnapshot.mockReset()
  mockGetSchedulerStatus.mockReset()
  mockProbeModelsNow.mockReset()
  mockGetDownstreamApiKeys.mockReset()
  // Non-checklist queries resolve to empty-but-valid shapes so the section
  // renders without throwing; the snapshot is overridden per test.
  mockGetSchedulerStatus.mockResolvedValue({ items: [], generatedAt: '' })
  // Routes + keys default to "none built", so the onboarding checklist can
  // reach a verdict instead of staying hidden behind an unanswered count.
  onboardingState.routes = []
  mockGetDownstreamApiKeys.mockResolvedValue({ items: [] })
})

afterEach(() => cleanup())

describe('OverviewSection onboarding checklist', () => {
  it('renders the four journey steps + "Create site" CTA on a fresh deployment', async () => {
    mockGetDashboardSnapshot.mockResolvedValue({
      siteCount: 0,
      totalAccounts: 0,
    })

    renderWithClient(<OverviewSection />)

    // Wait for the snapshot query to resolve and the panel heading to mount.
    const heading = await screen.findByRole('heading', {
      name: 'Welcome to Metapi',
      level: 2,
    })
    expect(heading).toBeInTheDocument()
    expect(
      screen.getByText(
        'Four steps to a working call path: sites → accounts → routes → keys.'
      )
    ).toBeInTheDocument()

    // The whole journey is listed, not just the first gap — the operator can
    // see that a key is where this ends (/v1 is not callable without one).
    const steps = screen.getAllByRole('listitem')
    expect(steps).toHaveLength(4)

    const cta = screen.getByRole('link', { name: /Create site/i })
    expect(cta).toHaveAttribute('href', '/sites')
  })

  it('advances the CTA to the next gap instead of retiring at the first site', async () => {
    // The pre-checklist banner disappeared here (siteCount > 0), which is
    // exactly the dead end the four-step panel exists to close.
    mockGetDashboardSnapshot.mockResolvedValue({
      siteCount: 3,
      totalAccounts: 0,
    })

    renderWithClient(<OverviewSection />)

    const cta = await screen.findByRole('link', { name: /Add account/i })
    expect(cta).toHaveAttribute('href', '/accounts')
    expect(
      screen.queryByRole('link', { name: /Create site/i })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('heading', { name: 'Welcome to Metapi' })
    ).toBeInTheDocument()
  })

  it('retires the checklist once sites, accounts, routes and keys all exist', async () => {
    onboardingState.routes = [{ id: 1 }]
    mockGetDownstreamApiKeys.mockResolvedValue({ items: [{ id: 2 }] })
    mockGetDashboardSnapshot.mockResolvedValue({
      siteCount: 3,
      totalAccounts: 4,
    })

    renderWithClient(<OverviewSection />)

    await waitFor(() => {
      expect(mockGetDashboardSnapshot).toHaveBeenCalledTimes(1)
    })
    await waitFor(() => {
      expect(
        screen.queryByRole('heading', { name: 'Welcome to Metapi' })
      ).not.toBeInTheDocument()
    })
    expect(screen.queryAllByRole('listitem')).toHaveLength(0)
  })

  it('does not flash the checklist while the snapshot is still loading', async () => {
    // Never resolves — simulates the loading window before the first byte
    // lands. siteCount stays undefined, so the panel must not render.
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

    // The failure is no longer silent: a banner replaces the
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

  it('keeps an unavailable balance unknown instead of reporting zero', async () => {
    mockGetDashboardSnapshot.mockResolvedValue({
      siteCount: 3,
      totalBalance: null,
    })
    renderWithClient(<OverviewSection />)
    expect(await screen.findByText('Active-site balance —')).toBeInTheDocument()
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

vi.mock('../components/request-metrics', () => ({ RequestMetrics: () => null }))

vi.mock('../components/request-trend', () => ({ RequestTrend: () => null }))

vi.mock('../components/attention-panel', () => ({ AttentionPanel: () => null }))

vi.mock('../components/upstream-health-panel', () => ({
  UpstreamHealthPanel: () => null,
}))

vi.mock('../components/model-usage-panel', () => ({
  ModelUsagePanel: () => null,
}))

describe('overview report range', () => {
  it('switches the shared analytical query to all retained history', async () => {
    mockGetDashboardSnapshot.mockResolvedValue({
      siteCount: 1,
      totalAccounts: 1,
    })
    renderWithClient(<OverviewSection />)
    await waitFor(() =>
      expect(api.getOverviewReport).toHaveBeenCalledWith('7d')
    )
    fireEvent.click(screen.getByRole('button', { name: 'All time' }))
    await waitFor(() =>
      expect(api.getOverviewReport).toHaveBeenCalledWith('all')
    )
    expect(screen.getByRole('button', { name: 'All time' })).toHaveAttribute(
      'aria-pressed',
      'true'
    )
  })
})
