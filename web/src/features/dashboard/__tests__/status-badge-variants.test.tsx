// Badge recipe convergence regression (audit P1 #1): the dashboard panels
// render their status pills through the design-system Badge variants instead
// of hand-rolled spans that duplicate the soft recipes. The scheduler table
// (overview) and the severity pills (availability) must therefore expose the
// semantic `data-variant` contract.

import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'
import { api } from '@/lib/api'

import { AvailabilitySection } from '../sections/availability/availability-section'
import { OverviewSection } from '../sections/overview/overview-section'

vi.mock('@/lib/api', () => ({
  api: {
    getDashboardSnapshot: vi.fn(),
    getBalanceHistory: vi.fn(),
    getSchedulerStatus: vi.fn(),
    getAttention: vi.fn(),
    getActiveAnnouncements: vi.fn(),
    dismissAnnouncement: vi.fn(),
  },
}))

// TodaySnapshotStrip renders a router Link; the test has no RouterProvider,
// so degrade Link to a plain anchor while keeping the rest of the module.
vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    Link: ({ to, children }: { to?: unknown; children?: ReactNode }) => (
      <a href={typeof to === 'string' ? to : '/'}>{children}</a>
    ),
  }
})

const mockSnapshot = vi.mocked(api.getDashboardSnapshot)
const mockBalanceHistory = vi.mocked(api.getBalanceHistory)
const mockSchedulerStatus = vi.mocked(api.getSchedulerStatus)
const mockAttention = vi.mocked(api.getAttention)
const mockAnnouncements = vi.mocked(api.getActiveAnnouncements)

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )
  }
}

function renderSection(element: ReactNode) {
  return render(element, { wrapper: createWrapper() })
}

function readTableBadgeVariants(container: HTMLElement): string[] {
  return [...container.querySelectorAll('table [data-slot="badge"]')].map(
    (badge) => badge.getAttribute('data-variant') ?? ''
  )
}

beforeEach(() => {
  mockSnapshot.mockReset().mockResolvedValue({})
  mockBalanceHistory.mockReset().mockResolvedValue({ series: [], days: 8 })
  mockAnnouncements.mockReset().mockResolvedValue({ items: [] })
  mockAttention.mockReset().mockResolvedValue({ items: [], total: 0 })
  mockSchedulerStatus.mockReset()
})

afterEach(() => cleanup())

describe('overview scheduler status pills', () => {
  it('renders an enabled job with a failed run as success + destructive badges', async () => {
    mockSchedulerStatus.mockResolvedValue({
      items: [
        {
          job: 'probe-job',
          enabled: true,
          lastStatus: 'failed',
          runs24h: 3,
          success24h: 2,
        },
      ],
      generatedAt: '2026-08-18T00:00:00Z',
    })

    const { container } = renderSection(<OverviewSection />)

    await waitFor(() => {
      expect(screen.getByText('probe-job')).toBeInTheDocument()
    })
    expect(readTableBadgeVariants(container)).toEqual([
      'success',
      'destructive',
    ])
  })

  it('renders a disabled job with a running status as secondary + info badges', async () => {
    mockSchedulerStatus.mockResolvedValue({
      items: [
        {
          job: 'probe-job',
          enabled: false,
          lastStatus: 'running',
          runs24h: 0,
          success24h: 0,
        },
      ],
      generatedAt: '2026-08-18T00:00:00Z',
    })

    const { container } = renderSection(<OverviewSection />)

    await waitFor(() => {
      expect(screen.getByText('probe-job')).toBeInTheDocument()
    })
    expect(readTableBadgeVariants(container)).toEqual(['secondary', 'info'])
  })
})

describe('availability severity pills', () => {
  it('renders attention items through the destructive/warning/info badge variants', async () => {
    mockAttention.mockResolvedValue({
      items: [
        {
          severity: 'critical',
          category: 'account',
          label: 'Expired account',
          target: '',
          createdAt: '2026-08-18T00:00:00Z',
        },
        {
          severity: 'warning',
          category: 'balance',
          label: 'Low balance',
          target: '',
          createdAt: '2026-08-18T00:00:00Z',
        },
        {
          severity: 'info',
          category: 'event',
          label: 'Site event',
          target: '',
          createdAt: '2026-08-18T00:00:00Z',
        },
      ],
      total: 3,
    })

    const { container } = renderSection(<AvailabilitySection />)

    await waitFor(() => {
      expect(screen.getByText('Expired account')).toBeInTheDocument()
    })
    const variants = [...container.querySelectorAll('[data-slot="badge"]')].map(
      (badge) => badge.getAttribute('data-variant') ?? ''
    )
    expect(variants).toEqual(['destructive', 'warning', 'info'])
  })
})
