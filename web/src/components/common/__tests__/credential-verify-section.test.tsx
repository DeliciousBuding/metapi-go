// Behavior tests for the Connect dialog's verify section: the two real
// requests the browser makes with the key (models smoke + one minimal chat
// generation), the failure surfaces, and the jump into the key-filtered
// logs. fetch is stubbed at the global edge so the assertions pin the exact
// wire calls (URL, Authorization header, body shape).

import '@testing-library/jest-dom/vitest'
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
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

import { CredentialVerifySection } from '../credential-verify-section'

// The real TanStack Router Link needs a router context; render a plain
// anchor so the href is queryable.
vi.mock('@tanstack/react-router', () => ({
  Link: ({
    to,
    search,
    children,
  }: {
    to: string
    search?: Record<string, unknown>
    children: ReactNode
  }) => {
    const query = search
      ? '?' + new URLSearchParams(search as Record<string, string>).toString()
      : ''
    return <a href={to + query}>{children}</a>
  },
}))

const { mockFetch } = vi.hoisted(() => ({ mockFetch: vi.fn() }))
vi.stubGlobal('fetch', mockFetch)

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
  mockFetch.mockReset()
})

afterEach(() => cleanup())

function jsonResponse(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

function renderSection() {
  return render(
    <CredentialVerifySection
      keyId={7}
      apiBase='http://gateway.example.com/v1'
      apiKey='sk-plain-demo'
    />
  )
}

describe('CredentialVerifySection', () => {
  it('clears the previous chat result when selecting another model', async () => {
    mockFetch
      .mockResolvedValueOnce(
        jsonResponse(200, { data: [{ id: 'gpt-5.5' }, { id: 'claude-x' }] })
      )
      .mockResolvedValueOnce(
        jsonResponse(200, { choices: [{ message: { role: 'assistant' } }] })
      )
    renderSection()
    fireEvent.click(
      screen.getByRole('button', { name: 'Check available models' })
    )
    fireEvent.click(
      await screen.findByRole('button', { name: 'Send a test chat' })
    )
    await screen.findByText(/Succeeded in/)
    fireEvent.click(screen.getByRole('combobox'))
    const option = await screen.findByRole('option', { name: 'claude-x' })
    fireEvent.pointerDown(option, { pointerType: 'mouse' })
    fireEvent.click(option)
    expect(screen.getByRole('combobox')).toHaveTextContent('claude-x')
    expect(screen.queryByText(/Succeeded in/)).not.toBeInTheDocument()
    expect(mockFetch).toHaveBeenCalledTimes(2)
  })

  it('keeps the tested model fixed and blocks another check while chat is pending', async () => {
    let finish!: (response: Response) => void
    const pending = new Promise<Response>((resolve) => {
      finish = resolve
    })
    mockFetch
      .mockResolvedValueOnce(
        jsonResponse(200, { data: [{ id: 'gpt-5.5' }, { id: 'claude-x' }] })
      )
      .mockReturnValueOnce(pending)
    renderSection()
    fireEvent.click(
      screen.getByRole('button', { name: 'Check available models' })
    )
    fireEvent.click(
      await screen.findByRole('button', { name: 'Send a test chat' })
    )
    expect(screen.getByRole('combobox')).toBeDisabled()
    expect(
      screen.getByRole('button', { name: 'Check available models' })
    ).toBeDisabled()
    await act(async () =>
      finish(
        jsonResponse(200, {
          choices: [{ message: { role: 'assistant', content: 'pong' } }],
        })
      )
    )
    expect(screen.getByRole('combobox')).not.toBeDisabled()
  })

  it('aborts a pending check when the dialog closes', async () => {
    let finish!: (response: Response) => void
    const pending = new Promise<Response>((resolve) => {
      finish = resolve
    })
    mockFetch.mockReturnValueOnce(pending)
    const view = renderSection()
    fireEvent.click(
      screen.getByRole('button', { name: 'Check available models' })
    )
    const signal = (mockFetch.mock.calls[0][1] as RequestInit).signal
    view.unmount()
    expect(signal?.aborted).toBe(true)
    await act(async () =>
      finish(jsonResponse(200, { data: [{ id: 'late-model' }] }))
    )
  })

  it('runs the models smoke with the key and reports the listed count', async () => {
    mockFetch.mockResolvedValue(
      jsonResponse(200, { data: [{ id: 'gpt-5.5' }, { id: 'claude-x' }] })
    )
    renderSection()

    fireEvent.click(
      screen.getByRole('button', { name: 'Check available models' })
    )

    expect(
      await screen.findByText('2 models listed for this key')
    ).toBeInTheDocument()
    const [url, init] = mockFetch.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('http://gateway.example.com/v1/models')
    expect((init.headers as Record<string, string>).Authorization).toBe(
      'Bearer sk-plain-demo'
    )
  })

  it('stops at the models step on failure and links the key-filtered logs', async () => {
    mockFetch.mockResolvedValue(
      jsonResponse(401, { error: { message: 'invalid key' } })
    )
    renderSection()

    fireEvent.click(
      screen.getByRole('button', { name: 'Check available models' })
    )

    expect(await screen.findByText(/HTTP 401: invalid key/)).toBeInTheDocument()
    // The chat step never unlocks…
    expect(
      screen.queryByRole('button', { name: 'Send a test chat' })
    ).toBeNull()
    // …and the jump-to-logs link is pre-filtered to this key.
    const logsLink = screen.getByRole('link', { name: /View this key's logs/ })
    expect(logsLink).toHaveAttribute('href', '/proxy-logs?downstreamKeyId=7')
    expect(mockFetch).toHaveBeenCalledTimes(1)
  })

  it('sends one minimal non-stream chat completion for the selected model', async () => {
    mockFetch
      .mockResolvedValueOnce(jsonResponse(200, { data: [{ id: 'gpt-5.5' }] }))
      .mockResolvedValueOnce(
        jsonResponse(200, {
          choices: [{ message: { role: 'assistant', content: 'pong' } }],
        })
      )
    renderSection()

    fireEvent.click(
      screen.getByRole('button', { name: 'Check available models' })
    )
    fireEvent.click(
      await screen.findByRole('button', { name: 'Send a test chat' })
    )

    expect(await screen.findByText(/Succeeded in/)).toBeInTheDocument()
    const [url, init] = mockFetch.mock.calls[1] as [string, RequestInit]
    expect(url).toBe('http://gateway.example.com/v1/chat/completions')
    expect(JSON.parse(String(init.body))).toEqual({
      model: 'gpt-5.5',
      messages: [{ role: 'user', content: 'ping' }],
      max_tokens: 1,
      stream: false,
    })
    // The success state also offers the key-filtered logs jump.
    expect(
      screen.getByRole('link', { name: /View this key's logs/ })
    ).toHaveAttribute('href', '/proxy-logs?downstreamKeyId=7')
  })

  it('surfaces the upstream status and parsed error on chat failure', async () => {
    mockFetch
      .mockResolvedValueOnce(jsonResponse(200, { data: [{ id: 'gpt-5.5' }] }))
      .mockResolvedValueOnce(
        jsonResponse(503, { error: { message: 'All channels exhausted' } })
      )
    renderSection()

    fireEvent.click(
      screen.getByRole('button', { name: 'Check available models' })
    )
    fireEvent.click(
      await screen.findByRole('button', { name: 'Send a test chat' })
    )

    expect(
      await screen.findByText(/HTTP 503: All channels exhausted/)
    ).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: /View this key's logs/ })
    ).toBeInTheDocument()
  })

  it.each([
    ['HTML instead of JSON', '<html>Sign in</html>'],
    ['an error envelope', JSON.stringify({ error: { message: 'failed' } })],
    ['empty choices', JSON.stringify({ choices: [] })],
    ['a choice without a message', JSON.stringify({ choices: [{}] })],
    ['an empty message', JSON.stringify({ choices: [{ message: {} }] })],
  ])('rejects HTTP 200 with %s as a failed chat check', async (_, body) => {
    mockFetch
      .mockResolvedValueOnce(jsonResponse(200, { data: [{ id: 'gpt-5.5' }] }))
      .mockResolvedValueOnce(new Response(body, { status: 200 }))
    renderSection()

    fireEvent.click(
      screen.getByRole('button', { name: 'Check available models' })
    )
    fireEvent.click(
      await screen.findByRole('button', { name: 'Send a test chat' })
    )

    expect(
      await screen.findByText(/HTTP 200:.*valid Chat Completions response/)
    ).toBeInTheDocument()
    expect(screen.queryByText(/Succeeded in/)).not.toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: /View this key's logs/ })
    ).toHaveAttribute('href', '/proxy-logs?downstreamKeyId=7')
  })

  it('rejects a malformed model list without treating it as an empty catalog', async () => {
    mockFetch.mockResolvedValue(
      jsonResponse(200, { error: { message: 'blocked' } })
    )
    renderSection()

    fireEvent.click(
      screen.getByRole('button', { name: 'Check available models' })
    )

    expect(await screen.findByText(/valid model list/)).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Send a test chat' })
    ).not.toBeInTheDocument()
    expect(screen.queryByText(/no models right now/)).not.toBeInTheDocument()
  })

  it('reports a key with no callable models honestly instead of offering a chat', async () => {
    mockFetch.mockResolvedValue(jsonResponse(200, { data: [] }))
    renderSection()

    fireEvent.click(
      screen.getByRole('button', { name: 'Check available models' })
    )

    expect(
      await screen.findByText(/can call no models right now/)
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Send a test chat' })
    ).toBeNull()
  })

  it('classifies an aborted request as a timeout instead of a stack trace', async () => {
    mockFetch.mockRejectedValue(new DOMException('aborted', 'AbortError'))
    renderSection()

    fireEvent.click(
      screen.getByRole('button', { name: 'Check available models' })
    )

    expect(
      await screen.findByText('Request timed out or network error')
    ).toBeInTheDocument()
  })
})
