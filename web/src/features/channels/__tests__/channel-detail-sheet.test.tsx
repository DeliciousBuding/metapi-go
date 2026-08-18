// Behavior tests for the channel detail sheet's cooldown action (audit
// 2026-08-18 multi-perspective review: channel-detail-sheet dead-end).
// A channel in cooldown/breaker_open previously had no recovery action in
// the detail view; the sheet now exposes the route-scoped clear-cooldown
// mutation. Asserts what the operator sees and triggers: button presence
// per status, routeId wiring, and the pending guard.

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

import { ChannelDetailSheet } from '../components/channel-detail-sheet'
import type { ChannelRow } from '../types'

const clearState = vi.hoisted(() => ({
  mutate: vi.fn(),
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
    name: 'probe-channel',
    site: { id: 1, name: 'Probe site' },
    type: 'openai',
    status: 'enabled',
    models: 'gpt-*',
    priority: 1,
    weight: 1,
    responseMs: null,
    cooldownUntil: null,
    enabled: true,
    manualOverride: false,
    ...overrides,
  } as ChannelRow
}

function renderSheet(
  channel: ChannelRow | null,
  onEdit?: (channel: ChannelRow) => void
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
    <ChannelDetailSheet
      channel={channel}
      open
      onOpenChange={() => {}}
      onEdit={onEdit}
    />,
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

describe('ChannelDetailSheet cooldown action', () => {
  it('offers the route cooldown clear for a cooldown channel', () => {
    renderSheet(
      makeChannel({
        status: 'cooldown',
        cooldownUntil: '2026-08-18T10:00:00Z',
      })
    )

    expect(
      screen.getByRole('button', { name: 'Clear route cooldown' })
    ).toBeInTheDocument()
    // Scope hint keeps the route-wide blast radius honest.
    expect(
      screen.getByText(/every channel on this channel's route/)
    ).toBeInTheDocument()
  })

  it('offers the route cooldown clear for a breaker_open channel', () => {
    renderSheet(makeChannel({ status: 'breaker_open' }))

    expect(
      screen.getByRole('button', { name: 'Clear route cooldown' })
    ).toBeInTheDocument()
  })

  it('hides the action for a healthy channel', () => {
    renderSheet(makeChannel({ status: 'enabled' }))

    expect(
      screen.queryByRole('button', { name: 'Clear route cooldown' })
    ).not.toBeInTheDocument()
  })

  it('clears the cooldown of the channel route on click', () => {
    renderSheet(makeChannel({ status: 'cooldown' }))

    fireEvent.click(
      screen.getByRole('button', { name: 'Clear route cooldown' })
    )

    expect(clearState.mutate).toHaveBeenCalledTimes(1)
    expect(clearState.mutate.mock.calls[0][0]).toBe(42)
  })

  it('disables the button while the clear is pending', () => {
    clearState.isPending = true
    renderSheet(makeChannel({ status: 'cooldown' }))

    expect(
      screen.getByRole('button', { name: 'Clear route cooldown' })
    ).toBeDisabled()
  })
})

describe('ChannelDetailSheet edit-route action', () => {
  it('renders the edit-route button in the footer for a healthy channel', () => {
    renderSheet(makeChannel({ status: 'enabled' }))

    expect(
      screen.getByRole('button', { name: 'Edit route' })
    ).toBeInTheDocument()
  })

  it('renders the edit-route button alongside the cooldown action', () => {
    renderSheet(makeChannel({ status: 'cooldown' }))

    expect(
      screen.getByRole('button', { name: 'Edit route' })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Clear route cooldown' })
    ).toBeInTheDocument()
  })

  it('calls onEdit with the channel when the edit-route button is clicked', () => {
    const onEdit = vi.fn()
    renderSheet(makeChannel({ routeId: 42 }), onEdit)

    fireEvent.click(screen.getByRole('button', { name: 'Edit route' }))

    expect(onEdit).toHaveBeenCalledTimes(1)
    expect(onEdit).toHaveBeenCalledWith(
      expect.objectContaining({ routeId: 42 })
    )
  })

  it('disables the edit-route button when the channel has no routeId', () => {
    renderSheet(makeChannel({ routeId: 0 }))

    expect(
      screen.getByRole('button', { name: 'Edit route' })
    ).toBeDisabled()
  })
})
