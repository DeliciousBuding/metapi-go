// Behavior test for the attention panel's relative-timestamp rendering.
//
// Closes availability gap 5: the attention/alerts list showed severity +
// message but never the alert's `createdAt`, so an operator could not tell
// a 30-second-old critical alert from one 3 hours stale. Each item now
// renders a muted relative timestamp ("2 hours ago") next to the message
// with the absolute timestamp as a hover tooltip. Items missing `createdAt`
// render no timestamp (no "ago" with no date).
//
// The realtime ops hook + api.getAttention are stubbed (mirrors the sibling
// sparkline test). `Date.now` is pinned so the relative formatter produces a
// deterministic "2 hours ago" regardless of when the test runs.

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

const mockGetAttention = vi.mocked(api.getAttention)
const mockUseRealtimeOps = vi.mocked(useRealtimeOps)

// Pin "now" so the relative formatter is deterministic. The attention item
// below is 2 hours older than this fixed instant → "2 hours ago" in en.
const FIXED_NOW = new Date('2026-01-15T12:00:00Z').getTime()
const CREATED_AT = new Date('2026-01-15T10:00:00Z').toISOString()

const IDLE_SAMPLE: RealtimeOpsSample = {
  qps: 0,
  successRate: 0,
  lifetime: 0,
  uptimeSeconds: 0,
  spark: [],
  connected: false,
  gaveUp: false,
}

/** Wrap a realtime sample in the hook's `{ sample, lastFrameAt, reconnect }` return shape. */
function realtimeReturn(sample: RealtimeOpsSample) {
  return { sample, lastFrameAt: null, reconnect: vi.fn() }
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
  // Sibling realtime ops panel renders idle without a live WebSocket.
  mockUseRealtimeOps.mockReturnValue(realtimeReturn(IDLE_SAMPLE))
  vi.spyOn(Date, 'now').mockReturnValue(FIXED_NOW)
})

afterEach(() => {
  vi.restoreAllMocks()
  cleanup()
})

describe('AvailabilitySection attention item createdAt', () => {
  it('renders a relative timestamp when an item has createdAt', async () => {
    mockGetAttention.mockResolvedValue({
      items: [
        {
          severity: 'critical',
          category: 'account',
          label: 'Account expired',
          target: '/accounts/1',
          createdAt: CREATED_AT,
        },
      ],
      total: 1,
    })

    const { container } = renderWithClient(<AvailabilitySection />)

    const timestamp = await screen.findByText('2 hours ago')
    expect(timestamp).toBeInTheDocument()
    expect(timestamp.tagName).toBe('TIME')

    const timeElement = container.querySelector('time')
    expect(timeElement).not.toBeNull()
    expect(timeElement?.getAttribute('datetime')).toBe(CREATED_AT)
    // The hover tooltip carries a localized absolute date (non-empty) so a
    // stale alert can be read in full without expanding the row.
    expect(timeElement?.getAttribute('title')?.length).toBeGreaterThan(0)
  })

  it('renders no timestamp when an item is missing createdAt', async () => {
    mockGetAttention.mockResolvedValue({
      items: [
        {
          severity: 'info',
          category: 'site',
          label: 'Site disabled',
          target: '',
          createdAt: '',
        },
      ],
      total: 1,
    })

    const { container } = renderWithClient(<AvailabilitySection />)

    // Wait for the attention list to render (item label is the stable hook).
    await screen.findByText('Site disabled')

    expect(container.querySelector('time')).toBeNull()
  })
})
