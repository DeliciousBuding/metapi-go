import '@testing-library/jest-dom/vitest'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'

import '@/i18n/config'
import type { ChannelRow } from '@/features/channels'

import { BatchResults } from './batch-results'

const channels: ChannelRow[] = [
  {
    id: 1,
    routeId: 10,
    name: 'Primary channel',
    site: { id: 20, name: 'Primary site' },
    type: 'account',
    status: 'enabled',
    models: 'gpt-4o',
    priority: 0,
    weight: 10,
    responseMs: null,
    cooldownUntil: null,
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
