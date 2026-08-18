// Behavior test for the realtime ops sparkline health colouring.
//
// Closes the availability-section gap (dashboard review): the realtime ops
// panel's sparkline was volume-only + monochrome, so a slow degradation
// (success rate falling while volume holds) was invisible at a glance. The
// sparkline now colours each bar by per-second success-rate health band and
// exposes the current band as the sparkline's accessible name, so:
//   (a) the bar colour shifts green → amber → red as success degrades;
//   (b) the accessible name (not the colour alone) carries the health state
//       for assistive tech + gives the test a stable, non-brittle contract.
//
// The realtime ops WebSocket hook (useRealtimeOps) is stubbed so the panel
// renders with controlled spark samples; api.getAttention is stubbed so the
// sibling Attention panel resolves without a network call. Each case asserts
// the sparkline's accessible name reflects the LATEST sample's health band.
//
// Note on latency: the realtime ops stream carries no latency field
// (handler/shared/realtime.go: RealtimePoint = {Ts, Total, Success}), so the
// panel intentionally displays no latency stat — this test does not assert one.

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

/**
 * Build a connected realtime sample whose sparkline ends at the given point.
 * Earlier bars are a healthy baseline so the LATEST point is what determines
 * the sparkline's accessible name (the "latest wins" behaviour under test).
 */
function sampleWithLatest(point: {
  qps: number
  successRate: number
}): RealtimeOpsSample {
  return {
    qps: point.qps,
    successRate: point.successRate,
    lifetime: 120,
    spark: [
      { qps: 10, successRate: 0.99 },
      { qps: 10, successRate: 0.97 },
      point,
    ],
    connected: true,
    gaveUp: false,
  }
}

const IDLE_SAMPLE: RealtimeOpsSample = {
  qps: 0,
  successRate: 0,
  lifetime: 0,
  spark: [],
  connected: false,
  gaveUp: false,
}

beforeEach(() => {
  mockGetAttention.mockReset()
  mockUseRealtimeOps.mockReset()
  // Sibling Attention panel resolves to an empty list so it never throws.
  mockGetAttention.mockResolvedValue({ items: [], total: 0 })
})

afterEach(() => cleanup())

describe('AvailabilitySection realtime sparkline health', () => {
  it('labels the sparkline healthy when the latest sample is healthy', async () => {
    mockUseRealtimeOps.mockReturnValue(
      sampleWithLatest({ qps: 12, successRate: 0.99 })
    )

    renderWithClient(<AvailabilitySection />)

    expect(
      await screen.findByRole('img', { name: 'Traffic healthy' })
    ).toBeInTheDocument()
  })

  it('labels the sparkline degraded when success falls to the amber band', async () => {
    mockUseRealtimeOps.mockReturnValue(
      sampleWithLatest({ qps: 12, successRate: 0.85 })
    )

    renderWithClient(<AvailabilitySection />)

    expect(
      await screen.findByRole('img', { name: 'Traffic degraded' })
    ).toBeInTheDocument()
  })

  it('labels the sparkline unhealthy when success drops below the degraded band', async () => {
    mockUseRealtimeOps.mockReturnValue(
      sampleWithLatest({ qps: 12, successRate: 0.5 })
    )

    renderWithClient(<AvailabilitySection />)

    expect(
      await screen.findByRole('img', { name: 'Traffic unhealthy' })
    ).toBeInTheDocument()
  })

  it('labels the sparkline idle when there is no recent traffic', async () => {
    mockUseRealtimeOps.mockReturnValue(IDLE_SAMPLE)

    renderWithClient(<AvailabilitySection />)

    expect(
      await screen.findByRole('img', { name: 'No recent traffic' })
    ).toBeInTheDocument()
  })

  it('derives the accessible name from the latest sample (latest wins)', async () => {
    // History is healthy, but the latest second is unhealthy — the name must
    // reflect the current state, not the historical baseline.
    mockUseRealtimeOps.mockReturnValue(
      sampleWithLatest({ qps: 12, successRate: 0.4 })
    )

    renderWithClient(<AvailabilitySection />)

    expect(
      await screen.findByRole('img', { name: 'Traffic unhealthy' })
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('img', { name: 'Traffic healthy' })
    ).not.toBeInTheDocument()
  })
})
