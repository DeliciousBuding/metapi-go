// Behavior test for the realtime ops panel's gave-up reconnect affordance.
//
// Closes availability gap 3: after MAX_FAILS consecutive WebSocket failures
// the hook gave up permanently and the panel rendered zeroed metrics
// (qps=0, successRate=0) indistinguishable from "no traffic". The panel now
// surfaces a "Realtime connection lost." notice + a "Reconnect" button that
// re-triggers the hook's connection. The zeroed metrics/sparkline stay hidden
// while gaveUp so the operator can tell a dead connection from idle traffic.
//
// The realtime ops hook is stubbed (the WS is a browser boundary outside the
// panel behavior under test); api.getAttention is stubbed so the sibling
// Attention panel resolves without a network call. The mock `reconnect` is a
// plain spy so the test asserts the panel wires it to the button's onClick.

import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
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

// The post-give-up sample: the hook set DISCONNECTED (gaveUp=true) after
// MAX_FAILS. The zeroed metrics + empty sparkline would look like "no
// traffic" without the gaveUp branch.
const GAVE_UP_SAMPLE: RealtimeOpsSample = {
  qps: 0,
  successRate: 0,
  lifetime: 0,
  spark: [],
  connected: false,
  gaveUp: true,
}

const CONNECTED_SAMPLE: RealtimeOpsSample = {
  qps: 5,
  successRate: 0.99,
  lifetime: 120,
  spark: [{ qps: 5, successRate: 0.99 }],
  connected: true,
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
  // Sibling Attention panel resolves to an empty list so it never throws.
  mockGetAttention.mockResolvedValue({ items: [], total: 0 })
})

afterEach(() => cleanup())

describe('AvailabilitySection realtime reconnect affordance', () => {
  it('renders the connection-lost notice + Reconnect button when gaveUp', () => {
    const reconnect = vi.fn()
    mockUseRealtimeOps.mockReturnValue({
      sample: GAVE_UP_SAMPLE,
      lastFrameAt: null,
      reconnect,
    })

    renderWithClient(<AvailabilitySection />)

    expect(screen.getByText('Realtime connection lost.')).toBeInTheDocument()
    const button = screen.getByRole('button', { name: 'Reconnect' })
    expect(button).toBeInTheDocument()
  })

  it('calls reconnect when the operator clicks the Reconnect button', () => {
    const reconnect = vi.fn()
    mockUseRealtimeOps.mockReturnValue({
      sample: GAVE_UP_SAMPLE,
      lastFrameAt: null,
      reconnect,
    })

    renderWithClient(<AvailabilitySection />)

    fireEvent.click(screen.getByRole('button', { name: 'Reconnect' }))

    expect(reconnect).toHaveBeenCalledTimes(1)
  })

  it('does not surface the Reconnect button while the stream is live', () => {
    const reconnect = vi.fn()
    mockUseRealtimeOps.mockReturnValue({
      sample: CONNECTED_SAMPLE,
      lastFrameAt: null,
      reconnect,
    })

    renderWithClient(<AvailabilitySection />)

    expect(
      screen.queryByRole('button', { name: 'Reconnect' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByText('Realtime connection lost.')
    ).not.toBeInTheDocument()
  })
})
