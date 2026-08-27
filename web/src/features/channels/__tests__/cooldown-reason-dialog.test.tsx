// Behavior tests for the cooldown root-cause dialog (P0-3): reason rendering,
// honest legacy-data state, remaining-time countdown, and the route-scoped
// clear action reuse.

import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import {
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from 'vitest'

import '@/i18n/config'

import { CooldownReasonDialog } from '../components/cooldown-reason-dialog'
import type { ChannelRow } from '../types'

type MutateOptions = { onSuccess?: () => void }

const clearState = vi.hoisted(() => ({
  mutate: vi.fn<(routeId: number, options?: MutateOptions) => void>(),
  isPending: false,
}))

vi.mock('@/features/token-routes/api', () => ({
  useClearRouteCooldown: () => ({
    mutate: clearState.mutate,
    isPending: clearState.isPending,
  }),
}))

function makeChannel(overrides: Partial<ChannelRow>): ChannelRow {
  return {
    id: 11,
    routeId: 42,
    name: 'cooling-channel',
    site: { id: 1, name: 'Probe site' },
    type: 'account',
    status: 'cooldown',
    models: 'gpt-*',
    priority: 1,
    weight: 1,
    responseMs: null,
    cooldownUntil: null,
    cooldownReasonCode: null,
    cooldownReason: null,
    cooldownReasonAt: null,
    enabled: true,
    manualOverride: false,
    ...overrides,
  }
}

function renderDialog(
  channel: ChannelRow | null,
  onOpenChange: (open: boolean) => void = () => {}
) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )
  }
  return render(
    <CooldownReasonDialog channel={channel} open onOpenChange={onOpenChange} />,
    { wrapper: Wrapper }
  )
}

beforeAll(() => {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: vi.fn().mockImplementation((query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  })
})

beforeEach(() => {
  clearState.mutate.mockReset()
  clearState.isPending = false
})

afterEach(() => cleanup())

describe('CooldownReasonDialog', () => {
  it('renders trigger code, error summary, recorded-at and countdown', () => {
    const until = new Date(Date.now() + 2 * 3600_000 + 30 * 60_000)
    renderDialog(
      makeChannel({
        cooldownUntil: until.toISOString(),
        cooldownReasonCode: 'upstream_error',
        cooldownReason: 'upstream exploded',
        cooldownReasonAt: '2026-08-28T09:00:00.000Z',
      })
    )

    // Localized label and the raw persisted code are both visible.
    expect(screen.getByText('Upstream server error')).toBeInTheDocument()
    expect(screen.getByText('upstream_error')).toBeInTheDocument()
    expect(screen.getByText('upstream exploded')).toBeInTheDocument()
    expect(screen.getByText(/2026/)).toBeInTheDocument()
    // Live countdown renders a bare HH:MM:SS value (recorded-at carries a
    // date prefix, so the exact matcher isolates the countdown).
    expect(
      screen.getAllByText((content) => /^\d{2}:\d{2}:\d{2}$/.test(content))
        .length
    ).toBeGreaterThan(0)
  })

  it('shows the honest legacy state when no reason was recorded', () => {
    renderDialog(makeChannel({ cooldownUntil: '2026-08-28T10:00:00.000Z' }))

    expect(
      screen.getByText('Reason not recorded (legacy data)')
    ).toBeInTheDocument()
    // No fabricated trigger code or summary in the legacy state.
    expect(screen.queryByText('Upstream server error')).not.toBeInTheDocument()
  })

  it('renders the raw code when it is not part of the known vocabulary', () => {
    renderDialog(
      makeChannel({
        cooldownUntil: '2026-08-28T10:00:00.000Z',
        cooldownReasonCode: 'future_code_from_newer_build',
      })
    )

    // Rendered once as the fallback label and once as the raw-code chip.
    expect(
      screen.getAllByText('future_code_from_newer_build').length
    ).toBeGreaterThanOrEqual(1)
  })

  it('shows a placeholder when the failure carried no error text', () => {
    renderDialog(
      makeChannel({
        cooldownUntil: '2026-08-28T10:00:00.000Z',
        cooldownReasonCode: 'rate_limited',
        cooldownReason: null,
        cooldownReasonAt: '2026-08-28T09:00:00.000Z',
      })
    )

    expect(screen.getByText(/No error text was recorded/)).toBeInTheDocument()
  })

  it('shows a dash for remaining time when no cooldown timestamp exists', () => {
    // breaker_open rows can be purely in-memory: no persisted cooldown_until.
    renderDialog(
      makeChannel({
        status: 'breaker_open',
        cooldownUntil: null,
        cooldownReasonCode: 'network_error',
        cooldownReasonAt: '2026-08-28T09:00:00.000Z',
      })
    )

    expect(screen.getByText('—')).toBeInTheDocument()
  })

  it('clears the route cooldown and closes on success', () => {
    clearState.mutate.mockImplementation((_routeId, options) => {
      options?.onSuccess?.()
    })
    const onOpenChange = vi.fn()
    renderDialog(
      makeChannel({
        cooldownUntil: '2026-08-28T10:00:00.000Z',
        cooldownReasonCode: 'timeout',
        cooldownReasonAt: '2026-08-28T09:00:00.000Z',
      }),
      onOpenChange
    )

    fireEvent.click(
      screen.getByRole('button', { name: 'Clear route cooldown' })
    )

    expect(clearState.mutate).toHaveBeenCalledTimes(1)
    expect(clearState.mutate.mock.calls[0][0]).toBe(42)
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })

  it('disables the clear button while the mutation is pending', () => {
    clearState.isPending = true
    renderDialog(
      makeChannel({
        cooldownUntil: '2026-08-28T10:00:00.000Z',
        cooldownReasonCode: 'timeout',
        cooldownReasonAt: '2026-08-28T09:00:00.000Z',
      })
    )

    expect(
      screen.getByRole('button', { name: /Clear route cooldown/ })
    ).toBeDisabled()
  })
})
