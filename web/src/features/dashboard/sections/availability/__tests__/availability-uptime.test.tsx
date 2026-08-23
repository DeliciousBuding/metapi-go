// Behavior test for the realtime uptime display (issue #889): the panel
// rendered the lifetime as permanent minutes ("1440m" after a day), so a
// long-running gateway showed a five-digit minute count. The uptime now
// escalates minutes → hours → days with localized unit labels.

import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen } from '@testing-library/react'
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

function sampleWithUptime(uptimeSeconds: number): RealtimeOpsSample {
  return {
    qps: 0,
    successRate: 0,
    lifetime: 0,
    uptimeSeconds,
    spark: [],
    connected: true,
    gaveUp: false,
  }
}

function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: 0, refetchOnWindowFocus: false },
    },
  })
}

function renderSection() {
  const queryClient = createQueryClient()
  return render(
    <QueryClientProvider client={queryClient}>
      <AvailabilitySection />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  mockGetAttention.mockReset()
  mockGetAttention.mockResolvedValue({ items: [], total: 0 })
  mockUseRealtimeOps.mockReset()
})

afterEach(() => cleanup())

describe('realtime uptime unit escalation', () => {
  it('renders wall-clock uptime, not the lifetime request counter', async () => {
    // The backend sends lifetime = monotonic request count (here 3 real
    // requests) alongside uptimeSeconds = wall-clock runtime. The panel must
    // render the runtime: a 45-minute-old process is "45 min", never "0 min".
    mockUseRealtimeOps.mockReturnValue({
      sample: {
        qps: 0,
        successRate: 0,
        lifetime: 3,
        uptimeSeconds: 45 * 60,
        spark: [],
        connected: true,
        gaveUp: false,
      },
      reconnect: vi.fn(),
      lastFrameAt: null,
    })

    renderSection()

    expect(await screen.findByText('45 min')).toBeInTheDocument()
    expect(screen.queryByText('0 min')).not.toBeInTheDocument()
  })
  it('shows minutes while the session is under an hour', async () => {
    mockUseRealtimeOps.mockReturnValue({
      sample: sampleWithUptime(45 * 60),
      reconnect: vi.fn(),
      lastFrameAt: null,
    })

    renderSection()

    expect(await screen.findByText('45 min')).toBeInTheDocument()
  })

  it('escalates to hours once the session passes an hour', async () => {
    mockUseRealtimeOps.mockReturnValue({
      sample: sampleWithUptime(150 * 60),
      reconnect: vi.fn(),
      lastFrameAt: null,
    })

    renderSection()

    expect(await screen.findByText('2.5 h')).toBeInTheDocument()
  })

  it('escalates to days for multi-day sessions instead of 4-digit minutes', async () => {
    mockUseRealtimeOps.mockReturnValue({
      sample: sampleWithUptime(3 * 24 * 60 * 60),
      reconnect: vi.fn(),
      lastFrameAt: null,
    })

    renderSection()

    expect(await screen.findByText('3 d')).toBeInTheDocument()
    expect(screen.queryByText('4320 m')).not.toBeInTheDocument()
    expect(screen.queryByText(/4320/)).not.toBeInTheDocument()
  })
})
