// Behavior tests for the site probe report panel.
//
// The panel is the honest report surface for POST-probe-stream: it renders
// live incremental results, marks probe-machinery `error` rows separately
// from failures, shows the truncation banner when the pass was cut short,
// and never turns a stopped/failed run into a success.

import '@testing-library/jest-dom/vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'
import type { SiteProbeResult } from '@/lib/api/sites'

import { SiteProbePanel } from '../site-probe-panel'

const { mockStreamSiteProbe, mockProbeSiteNow } = vi.hoisted(() => ({
  mockStreamSiteProbe: vi.fn(),
  mockProbeSiteNow: vi.fn(),
}))

vi.mock('@/lib/api/sites', () => ({
  sitesApi: {
    probeSiteNow: mockProbeSiteNow,
    streamSiteProbe: mockStreamSiteProbe,
  },
}))

type CapturedHandlers = {
  onResult?: (result: SiteProbeResult) => void
  onComplete?: (payload: {
    totalModels: number
    available: number
    unavailable: number
    truncated?: boolean
    reason?: string
  }) => void
  signal?: AbortSignal
}

let captured: CapturedHandlers | null = null
let streamPromise: Promise<void> | null = null

beforeEach(() => {
  mockStreamSiteProbe.mockReset()
  mockProbeSiteNow.mockReset()
  captured = null
  streamPromise = null
  mockStreamSiteProbe.mockImplementation(
    (_siteId: number, handlers: CapturedHandlers) => {
      captured = handlers
      streamPromise = new Promise<void>(() => {})
      return streamPromise
    }
  )
})

afterEach(() => cleanup())

const successRow: SiteProbeResult = {
  channelId: 1,
  accountId: 10,
  model: 'gpt-4o',
  status: 'success',
  latencyMs: 320,
}
const failureRow: SiteProbeResult = {
  channelId: 2,
  accountId: 11,
  model: 'gpt-4o-mini',
  status: 'failure',
  latencyMs: 0,
  error: 'upstream 503',
}
const errorRow: SiteProbeResult = {
  channelId: 3,
  accountId: 12,
  model: 'claude-sonnet-4',
  status: 'error',
  latencyMs: 0,
  error: 'target load failed',
}

describe('SiteProbePanel', () => {
  it('starts idle with an honest empty state and a run button', () => {
    render(<SiteProbePanel siteId={7} />)
    expect(
      screen.getByText(
        'No probe results yet. Run a probe to check the models available on this site.'
      )
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Run probe' })
    ).toBeInTheDocument()
  })

  it('streams incremental results and renders the final honest summary', async () => {
    render(<SiteProbePanel siteId={7} />)
    fireEvent.click(screen.getByRole('button', { name: 'Run probe' }))

    await waitFor(() =>
      expect(mockStreamSiteProbe).toHaveBeenCalledWith(7, expect.anything())
    )
    expect(captured).not.toBeNull()
    expect(screen.getByRole('button', { name: /Stop/ })).toBeInTheDocument()

    // Live incremental arrival: one success, then one failure with its
    // honest error text.
    captured?.onResult?.(successRow)
    expect(await screen.findByText('gpt-4o')).toBeInTheDocument()
    captured?.onResult?.(failureRow)
    await waitFor(() =>
      expect(screen.getByText('gpt-4o-mini')).toBeInTheDocument()
    )
    expect(screen.getByText('Failed')).toBeInTheDocument()
    expect(screen.getByText('upstream 503')).toBeInTheDocument()

    // A probe-machinery error row keeps its own badge + error text — it is
    // never rendered as a success.
    captured?.onResult?.(errorRow)
    await waitFor(() =>
      expect(screen.getByText('Probe error')).toBeInTheDocument()
    )

    captured?.onComplete?.({
      totalModels: 3,
      available: 1,
      unavailable: 2,
    })
    await waitFor(() =>
      expect(
        screen.getByText(/Total 3 · available 1 · unavailable 2/)
      ).toBeInTheDocument()
    )
    expect(
      screen.getByRole('button', { name: 'Run probe' })
    ).toBeInTheDocument()
  })

  it('shows the truncation banner with the reason when the pass is cut short', async () => {
    render(<SiteProbePanel siteId={7} />)
    fireEvent.click(screen.getByRole('button', { name: 'Run probe' }))
    await waitFor(() => expect(captured).not.toBeNull())

    captured?.onComplete?.({
      totalModels: 2,
      available: 1,
      unavailable: 1,
      truncated: true,
      reason: 'context canceled',
    })
    expect(
      await screen.findByText('Probe pass was cut short: context canceled')
    ).toBeInTheDocument()
  })

  it('marks an aborted run as stopped instead of success', async () => {
    // Reject when the caller aborts.
    mockStreamSiteProbe.mockImplementation(
      (_siteId: number, handlers: CapturedHandlers) =>
        new Promise<void>((_, reject) => {
          captured = handlers
          handlers.signal?.addEventListener('abort', () => {
            const err = new Error('aborted')
            err.name = 'AbortError'
            reject(err)
          })
        })
    )
    render(<SiteProbePanel siteId={7} />)
    fireEvent.click(screen.getByRole('button', { name: 'Run probe' }))
    await waitFor(() => expect(captured).not.toBeNull())

    fireEvent.click(screen.getByRole('button', { name: /Stop/ }))
    await waitFor(() =>
      expect(
        screen.getByText(
          'Probe stopped — only the results received so far are shown.'
        )
      ).toBeInTheDocument()
    )
    expect(screen.queryByText(/summary|Total/)).not.toBeInTheDocument()
  })

  it('renders the failure message on stream error', async () => {
    mockStreamSiteProbe.mockImplementation(
      (_siteId: number) =>
        new Promise<void>((_, reject) => reject(new Error('network down')))
    )
    render(<SiteProbePanel siteId={7} />)
    fireEvent.click(screen.getByRole('button', { name: 'Run probe' }))
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Probe failed: network down'
    )
  })
})
