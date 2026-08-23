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

describe('AvailabilitySection attention deep links', () => {
  it('links an expired-account item to /accounts?accountId=N', async () => {
    mockGetAttention.mockResolvedValue({
      items: [
        {
          severity: 'critical',
          category: 'expired_account',
          label: 'Account expired: expired-user',
          target: '/accounts?accountId=5',
          createdAt: '',
          params: { username: 'expired-user' },
        },
      ],
      total: 1,
    })

    renderWithClient(<AvailabilitySection />)

    const link = await screen.findByRole('link', {
      name: 'Account expired: expired-user',
    })
    expect(link).toHaveAttribute('href', '/accounts?accountId=5')
    // Full text available as an unhover tooltip when the row truncates.
    expect(link).toHaveAttribute('title', 'Account expired: expired-user')
  })

  it('links a disabled-site item to /sites?edit=N', async () => {
    mockGetAttention.mockResolvedValue({
      items: [
        {
          severity: 'warning',
          category: 'disabled_site',
          label: 'Site disabled: legacy-site',
          target: '/sites?edit=3',
          createdAt: '',
          params: { name: 'legacy-site' },
        },
      ],
      total: 1,
    })

    renderWithClient(<AvailabilitySection />)

    const link = await screen.findByRole('link', {
      name: 'Site disabled: legacy-site',
    })
    expect(link).toHaveAttribute('href', '/sites?edit=3')
    expect(link).toHaveAttribute('title', 'Site disabled: legacy-site')
  })

  it('links an event item to the settings program-logs section', async () => {
    mockGetAttention.mockResolvedValue({
      items: [
        {
          severity: 'warning',
          category: 'event',
          label: 'Upstream rate limited',
          target: '/settings/operations/program-logs',
          createdAt: '',
        },
      ],
      total: 1,
    })

    renderWithClient(<AvailabilitySection />)

    const link = await screen.findByRole('link', {
      name: 'Upstream rate limited',
    })
    expect(link).toHaveAttribute('href', '/settings/operations/program-logs')
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

  it('falls back to the raw English label when category params are missing', async () => {
    // Old backend payload: category present but no structured params.
    // The panel must render the verbatim label, not a string with empty
    // placeholders.
    mockGetAttention.mockResolvedValue({
      items: [
        {
          severity: 'warning',
          category: 'low_balance',
          label: 'Low balance: svc-oneapi-02 (0.00)',
          target: '/accounts?accountId=2',
          createdAt: '',
        },
      ],
      total: 1,
    })

    renderWithClient(<AvailabilitySection />)

    expect(
      await screen.findByText('Low balance: svc-oneapi-02 (0.00)')
    ).toBeInTheDocument()
  })

  it('localizes a low-balance item from its structured params', async () => {
    // New backend payload: params drive the i18n template. The rendered
    // amount comes from params.balance (5.43), not the English verbatim
    // label (which says 0.32) — proving the template path is used.
    mockGetAttention.mockResolvedValue({
      items: [
        {
          severity: 'warning',
          category: 'low_balance',
          label: 'Low balance: svc-oneapi-02 (0.32)',
          target: '/accounts?accountId=2',
          createdAt: '',
          params: { username: 'svc-oneapi-02', balance: 5.432 },
        },
      ],
      total: 1,
    })

    renderWithClient(<AvailabilitySection />)

    expect(
      await screen.findByText('Low balance: svc-oneapi-02 (5.43)')
    ).toBeInTheDocument()
    expect(
      screen.queryByText('Low balance: svc-oneapi-02 (0.32)')
    ).not.toBeInTheDocument()
  })

  it('localizes an unknown-balance item without a numeric amount', async () => {
    // A NULL balance must read as "unknown" — never "(0.00)".
    mockGetAttention.mockResolvedValue({
      items: [
        {
          severity: 'warning',
          category: 'balance_unknown',
          label: 'Balance unknown: svc-oneapi-02',
          target: '/accounts?accountId=2',
          createdAt: '',
          params: { username: 'svc-oneapi-02' },
        },
      ],
      total: 1,
    })

    renderWithClient(<AvailabilitySection />)

    expect(
      await screen.findByText('Balance unknown: svc-oneapi-02')
    ).toBeInTheDocument()
    expect(screen.queryByText(/0\.00/)).not.toBeInTheDocument()
  })
})
