import '@testing-library/jest-dom/vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'
import type { ChannelRow } from '@/features/channels'

import { BatchResults } from './batch-results'

// The comparison table runs outside a mounted router in this unit test, so
// render `Link` as a plain anchor that serializes `to` + `search` — enough
// to assert the channel deep-link targets.
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
    name: 'Primary channel',
    site: { id: 20, name: 'Primary site' },
    type: 'account',
    status: 'enabled',
    models: 'gpt-5.5',
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

afterEach(() => cleanup())

describe('BatchResults', () => {
  it('renders the summary, channel identity, and upstream error', () => {
    render(
      <BatchResults
        channels={channels}
        isRunning={false}
        results={[
          {
            channelId: 1,
            status: 'failure',
            latencyMs: 123,
            error: 'upstream unavailable',
          },
        ]}
      />
    )

    expect(screen.getByText('0 succeeded / 1 failed')).toBeInTheDocument()
    expect(screen.getByText('Primary channel')).toBeInTheDocument()
    expect(screen.getByText('Primary site')).toBeInTheDocument()
    expect(screen.getByText('upstream unavailable')).toBeInTheDocument()
    // The failed row's channel identity deep-links into the channels page
    // detail sheet instead of ending as plain text.
    expect(
      screen.getByRole('link', { name: 'Primary channel' })
    ).toHaveAttribute('href', '/channels?channelId=1')
  })

  it('announces a pending comparison before rows settle', () => {
    render(<BatchResults channels={channels} isRunning results={[]} />)

    expect(screen.getByText('Waiting for response…')).toBeInTheDocument()
  })

  it('reports stopped probes separately from failures', () => {
    render(
      <BatchResults
        channels={channels}
        isRunning={false}
        results={[{ channelId: 1, status: 'aborted' }]}
      />
    )

    expect(
      screen.getByText('0 succeeded / 0 failed / 1 stopped')
    ).toBeInTheDocument()
  })
})

describe('BatchResults bulk disable of failed channels', () => {
  const failure = {
    channelId: 1,
    status: 'failure',
    latencyMs: 123,
    error: 'upstream unavailable',
  } as const
  const success = {
    channelId: 1,
    status: 'success',
    latencyMs: 123,
    error: undefined,
  } as const

  it('offers the action only when at least one row failed', () => {
    const onDisableFailed = vi.fn()
    const { rerender } = render(
      <BatchResults
        channels={channels}
        isRunning={false}
        results={[success]}
        onDisableFailed={onDisableFailed}
      />
    )
    expect(
      screen.queryByRole('button', { name: 'Disable failed channels' })
    ).not.toBeInTheDocument()

    rerender(
      <BatchResults
        channels={channels}
        isRunning={false}
        results={[failure]}
        onDisableFailed={onDisableFailed}
      />
    )
    fireEvent.click(
      screen.getByRole('button', { name: 'Disable failed channels' })
    )
    expect(onDisableFailed).toHaveBeenCalledTimes(1)
  })

  it('blocks the action while the bulk disable is in flight or comparing', () => {
    render(
      <BatchResults
        channels={channels}
        isRunning={false}
        isDisablingFailed
        results={[failure]}
        onDisableFailed={vi.fn()}
      />
    )
    expect(
      screen.getByRole('button', { name: /Disable failed channels/ })
    ).toBeDisabled()
  })
})
