// Behavior tests for ChannelInlineProbe: mode toggle (`models` / `chat`),
// request dispatch to `POST /api/admin/test-channel`, loading + abort flow,
// success / failure / HTTP transport error rendering, optional additive fields
// (`firstByteLatencyMs`, `upstreamReportedModel`), and keyboard submission.

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

import { parseChannelProbeResult, probeChannel } from '../api'
import { ChannelInlineProbe } from '../components/channel-inline-probe'

const transportState = vi.hoisted(() => ({
  request: vi.fn(),
}))

vi.mock('@/lib/api/transport', () => ({
  request: transportState.request,
}))

beforeEach(() => {
  transportState.request.mockReset()
})

afterEach(() => cleanup())

describe('probeChannel API contract', () => {
  it('posts channelId, mode, and trimmed model to /api/admin/test-channel', async () => {
    transportState.request.mockResolvedValue({
      success: true,
      statusCode: 200,
      latencyMs: 142,
      firstByteLatencyMs: 68,
      upstreamReportedModel: 'gpt-4o-2026-01',
      truncatedBody: '{"id":"chatcmpl-1"}',
      error: null,
    })

    const result = await probeChannel({
      channelId: 17,
      mode: 'chat',
      model: '  gpt-4o  ',
    })

    expect(transportState.request).toHaveBeenCalledTimes(1)
    const [url, options] = transportState.request.mock.calls[0]
    expect(url).toBe('/api/admin/test-channel')
    expect(options.method).toBe('POST')
    expect(options.skipBusinessError).toBe(true)
    expect(options.skipErrorHandler).toBe(true)
    expect(JSON.parse(options.body as string)).toEqual({
      channelId: 17,
      mode: 'chat',
      model: 'gpt-4o',
    })
    expect(result).toMatchObject({
      success: true,
      statusCode: 200,
      latencyMs: 142,
      firstByteLatencyMs: 68,
      upstreamReportedModel: 'gpt-4o-2026-01',
      truncatedBody: '{"id":"chatcmpl-1"}',
      error: null,
    })
  })

  it('omits model when empty and rejects malformed envelopes', async () => {
    transportState.request.mockResolvedValue({
      success: true,
      statusCode: 200,
      latencyMs: 55,
      truncatedBody: '',
      error: null,
    })

    await probeChannel({ channelId: 9, mode: 'models', model: '   ' })
    const [, options] = transportState.request.mock.calls[0]
    expect(JSON.parse(options.body as string)).toEqual({
      channelId: 9,
      mode: 'models',
    })

    expect(() => parseChannelProbeResult({ success: 'yes' })).toThrow(
      'Invalid channel probe response'
    )
  })
})

describe('ChannelInlineProbe component', () => {
  it('pre-fills concrete model, skips wildcard patterns, and declares non-ledger diagnostic scope', () => {
    const view = render(
      <ChannelInlineProbe channelId={11} defaultModel='gpt-5-mini' />
    )
    expect(screen.getByLabelText('Model')).toHaveValue('gpt-5-mini')
    expect(screen.getByText(/not the normal usage ledger/i)).toBeInTheDocument()

    view.rerender(<ChannelInlineProbe channelId={12} defaultModel='gpt-*' />)
    expect(screen.getByLabelText('Model')).toHaveValue('')
  })

  it('renders success with status code, first-byte latency, total latency, and model alias badge', async () => {
    transportState.request.mockResolvedValue({
      success: true,
      statusCode: 200,
      latencyMs: 312,
      firstByteLatencyMs: 95,
      model: 'gpt-5-mini',
      upstreamReportedModel: 'gpt-5-mini-2026-08',
      truncatedBody: '{"choices":[{"message":{"content":"pong"}}]}',
      error: null,
    })

    render(<ChannelInlineProbe channelId={21} defaultModel='gpt-5-mini' />)

    fireEvent.click(screen.getByRole('button', { name: 'Chat' }))
    expect(screen.getByRole('button', { name: 'Chat' })).toHaveAttribute(
      'aria-pressed',
      'true'
    )

    fireEvent.click(screen.getByRole('button', { name: 'Run probe' }))

    expect(await screen.findByText('Success')).toBeInTheDocument()
    expect(screen.getByText('HTTP 200')).toBeInTheDocument()
    expect(screen.getByText('First byte')).toBeInTheDocument()
    expect(screen.getByText('95ms')).toBeInTheDocument()
    expect(screen.getByText('Total')).toBeInTheDocument()
    expect(screen.getByText('312ms')).toBeInTheDocument()
    expect(screen.getByText('gpt-5-mini-2026-08')).toBeInTheDocument()
    expect(
      screen.getByText('gpt-5-mini → gpt-5-mini-2026-08')
    ).toBeInTheDocument()
    expect(
      screen.getByText('{"choices":[{"message":{"content":"pong"}}]}')
    ).toBeInTheDocument()
  })

  it('omits first-byte latency cleanly when older servers do not return firstByteLatencyMs', async () => {
    transportState.request.mockResolvedValue({
      success: true,
      statusCode: 200,
      latencyMs: 1250,
      truncatedBody: '{"data":[{"id":"claude-sonnet-4"}]}',
      error: null,
    })

    render(<ChannelInlineProbe channelId={22} defaultModel='claude-sonnet-4' />)
    fireEvent.click(screen.getByRole('button', { name: 'Run probe' }))

    expect(await screen.findByText('Success')).toBeInTheDocument()
    expect(screen.queryByText('First byte')).not.toBeInTheDocument()
    expect(screen.getByText('1.3s')).toBeInTheDocument()
  })

  it('renders upstream failure responses with status code and redacted error details', async () => {
    transportState.request.mockResolvedValue({
      success: false,
      statusCode: 429,
      latencyMs: 184,
      truncatedBody: '{"error":{"message":"rate limit reached"}}',
      error: 'upstream status 429',
    })

    render(<ChannelInlineProbe channelId={23} defaultModel='gpt-5' />)
    fireEvent.click(screen.getByRole('button', { name: 'Run probe' }))

    expect(await screen.findByText('Failed')).toBeInTheDocument()
    expect(screen.getByText('HTTP 429')).toBeInTheDocument()
    expect(screen.getByText('upstream status 429')).toBeInTheDocument()
    expect(
      screen.getByText('{"error":{"message":"rate limit reached"}}')
    ).toBeInTheDocument()
  })

  it('renders HTTP transport error alerts when /api/admin/test-channel rejects', async () => {
    transportState.request.mockRejectedValue({
      response: {
        status: 404,
        data: { error: 'channel not found' },
      },
    })

    render(<ChannelInlineProbe channelId={999} />)
    fireEvent.click(screen.getByRole('button', { name: 'Run probe' }))

    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent('channel not found')
  })

  it('supports loading state and in-flight cancellation via Stop button', async () => {
    let capturedSignal: AbortSignal | undefined
    transportState.request.mockImplementation(
      (_url: string, options: { signal?: AbortSignal }) => {
        capturedSignal = options.signal
        return new Promise((_resolve, reject) => {
          options.signal?.addEventListener('abort', () => {
            const abortError = new Error('aborted')
            abortError.name = 'AbortError'
            reject(abortError)
          })
        })
      }
    )

    render(<ChannelInlineProbe channelId={25} defaultModel='gpt-5' />)
    fireEvent.click(screen.getByRole('button', { name: 'Run probe' }))

    const stopButton = await screen.findByRole('button', { name: /Stop/ })
    expect(screen.getByLabelText('Model')).toBeDisabled()
    expect(capturedSignal?.aborted).toBe(false)

    fireEvent.click(stopButton)
    expect(capturedSignal?.aborted).toBe(true)

    await waitFor(() => {
      expect(screen.getByText('Request was stopped.')).toBeInTheDocument()
    })
    expect(screen.getByRole('button', { name: 'Run probe' })).toBeEnabled()
  })

  it('triggers probe on Enter key and ignores IME composition confirmations', async () => {
    transportState.request.mockResolvedValue({
      success: true,
      statusCode: 200,
      latencyMs: 80,
      truncatedBody: '',
      error: null,
    })

    render(<ChannelInlineProbe channelId={30} defaultModel='gpt-5' />)
    const input = screen.getByLabelText('Model')

    // Safari IME keyCode 229 must not submit.
    fireEvent.keyDown(input, { key: 'Enter', keyCode: 229 })
    expect(transportState.request).not.toHaveBeenCalled()

    // Plain Enter submits the probe with the updated model value.
    fireEvent.change(input, { target: { value: 'o3-mini' } })
    fireEvent.keyDown(input, { key: 'Enter' })

    expect(await screen.findByText('Success')).toBeInTheDocument()
    const [, options] = transportState.request.mock.calls[0]
    expect(JSON.parse(options.body as string)).toEqual({
      channelId: 30,
      mode: 'models',
      model: 'o3-mini',
    })
  })
})
