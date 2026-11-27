import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'
import { api } from '@/lib/api'

import { GatewayRequestDetailSheet } from '../components/gateway-request-detail-sheet'
import { gatewayRequestFixture } from './gateway-request-fixture'

vi.mock('@tanstack/react-router', () => ({
  Link: ({
    to,
    children,
    search,
  }: {
    to?: string
    children: ReactNode
    search?: Record<string, string | number>
  }) => (
    <a
      href={`${to ?? '/token-routes'}?${new URLSearchParams(Object.entries(search ?? {}).map(([key, value]) => [key, String(value)]))}`}
    >
      {children}
    </a>
  ),
}))

beforeEach(() => {
  vi.spyOn(api, 'getGatewayRequest').mockResolvedValue(gatewayRequestFixture())
  vi.spyOn(api, 'getProxyLogDetail').mockResolvedValue({
    ...gatewayRequestFixture().attempts[0],
    status: 'success',
    modelRequested: 'synthetic-model',
    modelActual: 'synthetic-upstream-model',
    latencyMs: 115,
    retryCount: 0,
  })
})
afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

function renderDetail() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const close = vi.fn()
  render(
    <QueryClientProvider client={client}>
      <GatewayRequestDetailSheet id='synthetic-request-1' onClose={close} />
    </QueryClientProvider>
  )
  return { client, close }
}

describe('gateway request detail', () => {
  it('shows output interruption beside the actual committed 200 and only uses structured diagnosis context', async () => {
    vi.mocked(api.getGatewayRequest).mockResolvedValue(
      gatewayRequestFixture({
        terminalStatus: 'output_interrupted',
        reasonCode: 'unexpected_eof',
      })
    )
    renderDetail()
    expect(await screen.findByText('Output interrupted')).toBeInTheDocument()
    expect(screen.getByText('HTTP 200')).toBeInTheDocument()
    expect(screen.getByText('unexpected_eof')).toBeInTheDocument()
    const href = screen
      .getByRole('link', { name: 'Open route workspace' })
      .getAttribute('href')
    expect(href).toContain('workspaceModel=synthetic-model')
    expect(href).toContain('workspaceKeyId=7')
    expect(screen.queryByText('Completed')).not.toBeInTheDocument()
  })
  it('renders an honest zero-attempt rejection without an invented Attempt 1', async () => {
    vi.mocked(api.getGatewayRequest).mockResolvedValue(
      gatewayRequestFixture({
        terminalStatus: 'rejected_no_route',
        attemptCount: 0,
        attempts: [],
        outputStarted: false,
        httpStatus: 503,
      })
    )
    renderDetail()
    expect(await screen.findByText('No available route')).toBeInTheDocument()
    expect(screen.getByText(/No upstream dispatch/)).toBeInTheDocument()
    expect(screen.queryByText('Attempt 1')).not.toBeInTheDocument()
    expect(api.getProxyLogDetail).not.toHaveBeenCalled()
  })
  it('keeps zero usage and unknown usage separate and loads attempt billing only when expanded', async () => {
    renderDetail()
    const summary = await screen.findByText('Attempt 1')
    expect(api.getProxyLogDetail).not.toHaveBeenCalled()
    const disclosure = summary.closest('details')
    if (!disclosure) throw new Error('Missing attempt disclosure')
    disclosure.open = true
    fireEvent(disclosure, new Event('toggle'))
    await waitFor(() => expect(api.getProxyLogDetail).toHaveBeenCalledWith(17))
    expect(screen.getByTitle(/Input 0/)).toHaveTextContent('0 / —')
    expect(screen.getByText(/Estimated cost: Unknown/)).toBeInTheDocument()
  })
  it('shows a retryable query error rather than a successful empty request', async () => {
    vi.mocked(api.getGatewayRequest)
      .mockRejectedValueOnce(new Error('synthetic read failure'))
      .mockResolvedValueOnce(
        gatewayRequestFixture({ attemptCount: 0, attempts: [] })
      )
    renderDetail()
    expect(
      await screen.findByText(/Could not load request details/)
    ).toBeInTheDocument()
    expect(screen.queryByText(/No upstream dispatch/)).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /Retry/ }))
    expect(await screen.findByText('Completed')).toBeInTheDocument()
  })
  it('reports missing attempts and never renders WebSocket handshake 101 as a turn result', async () => {
    vi.mocked(api.getGatewayRequest).mockResolvedValue(
      gatewayRequestFixture({
        transport: 'websocket',
        terminalStatus: 'client_cancelled',
        httpStatus: null,
        attemptCount: 2,
        attempts: [],
      })
    )
    renderDetail()
    expect(await screen.findByText('Client cancelled')).toBeInTheDocument()
    expect(screen.getByText(/Only 0 of 2 attempts/)).toBeInTheDocument()
    expect(screen.queryByText(/101/)).not.toBeInTheDocument()
  })
  it('keeps nullable imported attempt metadata unknown instead of inventing a success or ordinal', async () => {
    vi.mocked(api.getGatewayRequest).mockResolvedValue(
      gatewayRequestFixture({
        attempts: [
          {
            ...gatewayRequestFixture().attempts[0],
            modelRequested: null,
            modelActual: null,
            status: null,
            latencyMs: null,
            retryCount: null,
          },
        ],
      })
    )
    renderDetail()
    expect(await screen.findByText('Attempt Unknown')).toBeInTheDocument()
    expect(screen.queryByText('Attempt 1')).not.toBeInTheDocument()
    expect(screen.getByTestId('attempt-waterfall-unknown')).toBeInTheDocument()
  })

  it('renders an honest multi-segment waterfall when boundaries are valid and marks inverted boundaries unknown', async () => {
    vi.mocked(api.getGatewayRequest).mockResolvedValue(
      gatewayRequestFixture({
        latencyMs: 500,
        attempts: [
          {
            ...gatewayRequestFixture().attempts[0],
            id: 17,
            retryCount: 0,
            latencyMs: 300,
            firstByteLatencyMs: 60,
            firstOutputLatencyMs: 150,
            isStream: true,
          },
          {
            ...gatewayRequestFixture().attempts[0],
            id: 18,
            retryCount: 1,
            latencyMs: 200,
            firstByteLatencyMs: 120,
            firstOutputLatencyMs: 80,
            isStream: true,
          },
        ],
      })
    )
    renderDetail()
    expect(
      await screen.findByText(/TTFB \(headers\): 60ms/)
    ).toBeInTheDocument()
    expect(screen.getByText(/First output: 150ms/)).toBeInTheDocument()
    expect(screen.getByText(/Stream \/ body: 150ms/)).toBeInTheDocument()
    expect(
      screen.getByText(/Timing boundary unknown or inconsistent/)
    ).toBeInTheDocument()
  })

  it('links to Model Tester without credentials and handles prev/next keyboard navigation', async () => {
    const onPrev = vi.fn()
    const onNext = vi.fn()
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={client}>
        <GatewayRequestDetailSheet
          id='synthetic-request-1'
          onClose={vi.fn()}
          hasPrev
          hasNext
          onNavigatePrev={onPrev}
          onNavigateNext={onNext}
        />
      </QueryClientProvider>
    )
    const reproLink = await screen.findByRole('link', {
      name: 'Reproduce in Model Tester',
    })
    expect(reproLink.getAttribute('href')).toContain(
      '/model-tester?model=synthetic-model&targetFormat=responses'
    )
    fireEvent.keyDown(window, { key: 'ArrowUp' })
    fireEvent.keyDown(window, { key: 'ArrowDown' })
    expect(onPrev).toHaveBeenCalledTimes(1)
    expect(onNext).toHaveBeenCalledTimes(1)

    const input = document.createElement('input')
    document.body.appendChild(input)
    fireEvent.keyDown(input, { key: 'ArrowDown' })
    expect(onNext).toHaveBeenCalledTimes(1)
    input.remove()
  })

  it('deduplicates waterfall legend and TimingCell when expanding a streamTiming attempt', async () => {
    const streamTiming = {
      headerMs: 120,
      firstReasoningMs: 260,
      firstTextMs: 520,
      lastContentMs: 1420,
      maxContentGapMs: 185,
      reasoningEvidence: 'visible' as const,
      decodeTokens: 180,
      decodeWindowMs: 900,
      decodeTps: 64.5,
      decodeStatus: 'measured' as const,
    }
    vi.mocked(api.getGatewayRequest).mockResolvedValue(
      gatewayRequestFixture({
        latencyMs: 1420,
        attempts: [
          {
            ...gatewayRequestFixture().attempts[0],
            id: 17,
            retryCount: 0,
            latencyMs: 1420,
            firstByteLatencyMs: 120,
            firstOutputLatencyMs: 260,
            isStream: true,
            streamTiming,
          },
        ],
      })
    )
    renderDetail()
    // Collapsed: summary waterfall displays the full stage legend for quick scanning.
    expect(
      await screen.findByTestId('attempt-waterfall-legend')
    ).toBeInTheDocument()

    // Expand the attempt: summary switches to compact bar while StreamTimingCard
    // takes ownership of the full stage legend and metrics, avoiding double legends
    // and redundant TimingCell.
    const disclosure = screen.getByText('Attempt 1').closest('details')
    if (!disclosure) throw new Error('Missing attempt disclosure')
    disclosure.open = true
    fireEvent(disclosure, new Event('toggle'))

    await waitFor(() =>
      expect(
        screen.queryByTestId('attempt-waterfall-legend')
      ).not.toBeInTheDocument()
    )
    expect(screen.getByTestId('attempt-waterfall')).toHaveAttribute(
      'data-waterfall-compact',
      'true'
    )
    expect(screen.getByTestId('stream-timing-card')).toBeInTheDocument()
    // Only StreamTimingCard shows the max content gap metric (no duplicate in summary).
    expect(screen.getByTestId('stream-timing-max-gap')).toHaveTextContent(
      '185ms'
    )
    expect(screen.getAllByText('185ms')).toHaveLength(1)
  })
})
