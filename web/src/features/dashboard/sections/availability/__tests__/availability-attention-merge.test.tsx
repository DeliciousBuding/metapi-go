// Behavior test for the attention panel's duplicate-event merging (wave 7):
// a persistent "All proxies failed" storm pushes one event row per scan
// window, so the panel stacked identical rows. Consecutive duplicates
// (same category / label / severity / target) merge into one row with a ×N
// count badge; distinct events stay independent. The merged row keeps the
// newest createdAt for its relative timestamp.

import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'
import { useRealtimeOps } from '@/features/dashboard/hooks/use-realtime-ops'
import type { RealtimeOpsSample } from '@/features/dashboard/types'
import { api } from '@/lib/api'

import { AvailabilitySection } from '../availability-section'

vi.mock('@/lib/api', () => ({
  api: {
    getAttention: vi.fn(),
  },
}))

vi.mock('@/features/dashboard/hooks/use-realtime-ops', () => ({
  useRealtimeOps: vi.fn(),
}))

// The attention panel renders outside a mounted router in this unit test, so
// render `Link` as a plain anchor that serializes `to` + `search` + `params`
// (mirrors the sibling attention-links test) — enough to resolve targets.
vi.mock('@tanstack/react-router', () => ({
  Link: ({
    children,
    to,
    search,
    params,
    title,
  }: {
    children?: ReactNode
    to: string
    search?: Record<string, number | string | undefined>
    params?: Record<string, string | undefined>
    title?: string
  }) => {
    const query = search
      ? new URLSearchParams(
          Object.entries(search)
            .filter(([, value]) => value !== undefined)
            .map(([key, value]) => [key, String(value)])
        ).toString()
      : ''
    const path = params
      ? Object.entries(params).reduce(
          (pathname, [key, value]) =>
            pathname.replace(`$${key}`, String(value)),
          to
        )
      : to
    return (
      <a href={query ? path + '?' + query : path} title={title}>
        {children}
      </a>
    )
  },
}))

const mockGetAttention = vi.mocked(api.getAttention)
const mockUseRealtimeOps = vi.mocked(useRealtimeOps)

const IDLE_SAMPLE: RealtimeOpsSample = {
  qps: 0,
  successRate: 0,
  lifetime: 0,
  uptimeSeconds: 0,
  spark: [],
  connected: false,
  gaveUp: false,
}

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
  mockGetAttention.mockReset()
  mockUseRealtimeOps.mockReset()
  mockUseRealtimeOps.mockReturnValue({
    sample: IDLE_SAMPLE,
    lastFrameAt: null,
    reconnect: vi.fn(),
  })
})

afterEach(() => {
  vi.restoreAllMocks()
  cleanup()
})

const PROXY_EVENT = {
  severity: 'critical',
  category: 'event',
  label: 'All proxies failed',
  target: '/settings/system-info/program-logs',
}

describe('AvailabilitySection attention duplicate merging', () => {
  it('merges two identical events into one row with a ×2 count badge', async () => {
    mockGetAttention.mockResolvedValue({
      items: [
        {
          ...PROXY_EVENT,
          createdAt: '2026-08-23T05:01:14Z',
        },
        {
          ...PROXY_EVENT,
          createdAt: '2026-08-23T05:00:45Z',
        },
      ],
      total: 2,
    })

    renderWithClient(<AvailabilitySection />)

    const rows = await screen.findAllByRole('link', {
      name: 'All proxies failed',
    })
    expect(rows).toHaveLength(1)
    expect(screen.getByText('×2')).toBeInTheDocument()
  })

  it('keeps distinct events independent', async () => {
    mockGetAttention.mockResolvedValue({
      items: [
        {
          ...PROXY_EVENT,
          createdAt: '2026-08-23T05:01:14Z',
        },
        {
          severity: 'warning',
          category: 'event',
          label: 'Upstream rate limited',
          target: '/settings/system-info/program-logs',
          createdAt: '2026-08-23T05:02:00Z',
        },
      ],
      total: 2,
    })

    renderWithClient(<AvailabilitySection />)

    const rows = await screen.findAllByRole('link', {
      name: 'All proxies failed',
    })
    expect(rows).toHaveLength(1)
    expect(
      screen.getByRole('link', { name: 'Upstream rate limited' })
    ).toBeInTheDocument()
    expect(screen.queryByText('×2')).not.toBeInTheDocument()
  })

  it('does not merge different targets or severities under the same label', async () => {
    mockGetAttention.mockResolvedValue({
      items: [
        {
          severity: 'critical',
          category: 'event',
          label: 'All proxies failed',
          target: '/settings/system-info/program-logs',
          createdAt: '2026-08-23T05:01:14Z',
        },
        {
          severity: 'warning',
          category: 'event',
          label: 'All proxies failed',
          target: '/settings/system-info/program-logs',
          createdAt: '2026-08-23T05:02:00Z',
        },
      ],
      total: 2,
    })

    renderWithClient(<AvailabilitySection />)

    const rows = await screen.findAllByRole('link', {
      name: 'All proxies failed',
    })
    expect(rows).toHaveLength(2)
    expect(screen.queryByText('×2')).not.toBeInTheDocument()
  })
})
