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
    children,
    search,
  }: {
    children: ReactNode
    search?: Record<string, string | number>
  }) => (
    <a
      href={`/token-routes?${new URLSearchParams(Object.entries(search ?? {}).map(([key, value]) => [key, String(value)]))}`}
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
    expect(screen.getByText('0')).toBeInTheDocument()
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
  })
})
