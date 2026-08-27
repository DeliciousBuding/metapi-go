// Behavior tests for the per-row re-run action in the comparison table
// (Wave 11 feedback loops). Every settled row — failed rows included — gets
// a re-run button with a channel-specific aria-label; the button shows the
// canonical Spinner (role=status) and is disabled while that row's probe is
// in flight, with no cross-talk to other rows.
import '@testing-library/jest-dom/vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'
import type { ChannelRow } from '@/features/channels'

import { BatchResults } from './batch-results'

// The comparison table runs outside a mounted router in this unit test, so
// render `Link` as a plain anchor (same seam as batch-results.test.tsx).
vi.mock('@tanstack/react-router', () => ({
  Link: ({
    children,
    to,
    search,
  }: {
    children?: ReactNode
    to: string
    search?: Record<string, number | string | undefined>
  }) => {
    const query = search
      ? new URLSearchParams(
          Object.entries(search)
            .filter(([, value]) => value !== undefined)
            .map(([key, value]) => [key, String(value)])
        ).toString()
      : ''
    return <a href={query ? `${to}?${query}` : to}>{children}</a>
  },
}))

const channels: ChannelRow[] = [
  {
    id: 1,
    routeId: 10,
    name: 'Fast channel',
    site: { id: 20, name: 'Primary site' },
    type: 'account',
    status: 'enabled',
    models: 'gpt-4o',
    priority: 0,
    weight: 10,
    responseMs: null,
    cooldownUntil: null,
    cooldownReasonCode: null,
    cooldownReason: null,
    cooldownReasonAt: null,
    enabled: true,
    manualOverride: false,
  },
  {
    id: 2,
    routeId: 11,
    name: 'Broken channel',
    site: { id: 21, name: 'Backup site' },
    type: 'account',
    status: 'enabled',
    models: 'gpt-4o',
    priority: 0,
    weight: 10,
    responseMs: null,
    cooldownUntil: null,
    cooldownReasonCode: null,
    cooldownReason: null,
    cooldownReasonAt: null,
    enabled: true,
    manualOverride: false,
  },
]

const settledResults = [
  { channelId: 1, status: 'success' as const, latencyMs: 120 },
  {
    channelId: 2,
    status: 'failure' as const,
    latencyMs: 30,
    error: 'upstream down',
  },
]

afterEach(() => cleanup())

describe('BatchResults per-row re-run', () => {
  it('renders a re-run button on every row, failed rows included', () => {
    render(
      <BatchResults
        channels={channels}
        isRunning={false}
        results={settledResults}
        onRerunRow={vi.fn()}
      />
    )

    expect(
      screen.getByRole('button', { name: 'Re-run probe for Fast channel' })
    ).toBeInTheDocument()
    // The failed row is re-runnable too.
    expect(
      screen.getByRole('button', { name: 'Re-run probe for Broken channel' })
    ).toBeInTheDocument()
  })

  it('re-runs the clicked row via onRerunRow', () => {
    const onRerunRow = vi.fn()
    render(
      <BatchResults
        channels={channels}
        isRunning={false}
        results={settledResults}
        onRerunRow={onRerunRow}
      />
    )

    fireEvent.click(
      screen.getByRole('button', { name: 'Re-run probe for Broken channel' })
    )

    expect(onRerunRow).toHaveBeenCalledTimes(1)
    expect(onRerunRow).toHaveBeenCalledWith(2)
  })

  it('shows the spinner and disables only the re-running row', () => {
    render(
      <BatchResults
        channels={channels}
        isRunning={false}
        results={settledResults}
        rerunningChannelIds={new Set([2])}
        onRerunRow={vi.fn()}
      />
    )

    const pendingButton = screen.getByRole('button', {
      name: /Re-run probe for Broken channel/,
    })
    expect(pendingButton).toBeDisabled()
    expect(within(pendingButton).getByRole('status')).toBeInTheDocument()

    // Other rows stay clickable — no global lock.
    expect(
      screen.getByRole('button', { name: 'Re-run probe for Fast channel' })
    ).toBeEnabled()
  })

  it('locks re-run buttons while a full comparison is running', () => {
    render(
      <BatchResults
        channels={channels}
        isRunning
        results={settledResults}
        onRerunRow={vi.fn()}
      />
    )

    expect(
      screen.getByRole('button', { name: 'Re-run probe for Fast channel' })
    ).toBeDisabled()
    expect(
      screen.getByRole('button', { name: 'Re-run probe for Broken channel' })
    ).toBeDisabled()
  })

  it('keeps the legacy shape without an actions column when onRerunRow is absent', () => {
    render(
      <BatchResults
        channels={channels}
        isRunning={false}
        results={settledResults}
      />
    )

    expect(
      screen.queryByRole('button', { name: /Re-run probe/ })
    ).not.toBeInTheDocument()
  })
})
