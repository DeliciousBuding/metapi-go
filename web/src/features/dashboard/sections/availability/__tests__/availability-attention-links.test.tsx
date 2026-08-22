// Behavior test for the attention panel's deep-link rendering.
//
// Closes the availability attention dead-end: dashboard attention items used
// to render plain `<a href>` anchors to backend target strings (a full-page
// reload, ignoring the SPA router). Items now parse their target into a
// typed router location and render a router `<Link>` — the accounts /
// sites targets land on the real entity (the one-shot `accountId` / `edit`
// deep links), event targets land on the settings program-logs section, and
// unrecognized targets render as plain text instead of a dead link.

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
// (mirrors the batch-results test) — enough to assert the deep-link targets.
vi.mock('@tanstack/react-router', () => ({
  Link: ({
    children,
    to,
    search,
    params,
  }: {
    children?: ReactNode
    to: string
    search?: Record<string, number | string | undefined>
    params?: Record<string, string | undefined>
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
    return <a href={query ? `${path}?${query}` : path}>{children}</a>
  },
}))

const mockGetAttention = vi.mocked(api.getAttention)
const mockUseRealtimeOps = vi.mocked(useRealtimeOps)

const IDLE_SAMPLE: RealtimeOpsSample = {
  qps: 0,
  successRate: 0,
  lifetime: 0,
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

describe('AvailabilitySection attention deep links', () => {
  it('links an expired-account item to /accounts?accountId=N', async () => {
    mockGetAttention.mockResolvedValue({
      items: [
        {
          severity: 'critical',
          category: 'expired_account',
          label: 'Account expired',
          target: '/accounts?accountId=5',
          createdAt: '',
        },
      ],
      total: 1,
    })

    renderWithClient(<AvailabilitySection />)

    const link = await screen.findByRole('link', { name: 'Account expired' })
    expect(link).toHaveAttribute('href', '/accounts?accountId=5')
  })

  it('links a disabled-site item to /sites?edit=N', async () => {
    mockGetAttention.mockResolvedValue({
      items: [
        {
          severity: 'warning',
          category: 'disabled_site',
          label: 'Site disabled',
          target: '/sites?edit=3',
          createdAt: '',
        },
      ],
      total: 1,
    })

    renderWithClient(<AvailabilitySection />)

    const link = await screen.findByRole('link', { name: 'Site disabled' })
    expect(link).toHaveAttribute('href', '/sites?edit=3')
  })

  it('links an event item to the settings program-logs section', async () => {
    mockGetAttention.mockResolvedValue({
      items: [
        {
          severity: 'warning',
          category: 'event',
          label: 'Upstream rate limited',
          target: '/settings/system-info/program-logs',
          createdAt: '',
        },
      ],
      total: 1,
    })

    renderWithClient(<AvailabilitySection />)

    const link = await screen.findByRole('link', {
      name: 'Upstream rate limited',
    })
    expect(link).toHaveAttribute('href', '/settings/system-info/program-logs')
  })

  it('renders plain text (no dead link) for an unrecognized target', async () => {
    mockGetAttention.mockResolvedValue({
      items: [
        {
          severity: 'info',
          category: 'event',
          label: 'Legacy payload',
          target: '/legacy/deadend',
          createdAt: '',
        },
      ],
      total: 1,
    })

    renderWithClient(<AvailabilitySection />)

    expect(await screen.findByText('Legacy payload')).toBeInTheDocument()
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
  })
})
