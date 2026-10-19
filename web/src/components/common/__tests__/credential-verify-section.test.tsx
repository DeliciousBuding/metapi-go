// Behavior tests for the Connect dialog's verify section: the models smoke
// plus one minimal request in the explicitly selected protocol, the failure
// surfaces, and the jump into the key-filtered logs. fetch is stubbed at the
// global edge so the assertions pin the exact wire calls.

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

import i18n from '@/i18n/config'

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

async function checkModelsAndWait(models = [{ id: 'gpt-5.5' }]) {
  mockFetch.mockResolvedValueOnce(jsonResponse(200, { data: models }))
  fireEvent.click(
    screen.getByRole('button', { name: 'Check available models' })
  )
  await screen.findByRole('combobox')
}

function selectProtocol(
  name: 'Chat Completions' | 'Responses' | 'Anthropic Messages'
) {
  fireEvent.click(screen.getByRole('radio', { name }))
}

describe('CredentialVerifySection', () => {
  it('clears the previous chat result when selecting another model', async () => {
    mockFetch
      .mockResolvedValueOnce(
        jsonResponse(200, { data: [{ id: 'gpt-5.5' }, { id: 'claude-x' }] })
      )
      .mockResolvedValueOnce(
        jsonResponse(200, {
          choices: [{ message: { role: 'assistant', content: 'pong' } }],
          error: null,
        })
      )
    renderSection()
    fireEvent.click(
      screen.getByRole('button', { name: 'Check available models' })
    )
    fireEvent.click(
      await screen.findByRole('button', { name: 'Send test request' })
    )
    await screen.findByText(/Chat Completions request succeeded in/)
    fireEvent.click(screen.getByRole('combobox'))
    const option = await screen.findByRole('option', { name: 'claude-x' })
    fireEvent.pointerDown(option, { pointerType: 'mouse' })
    fireEvent.click(option)
    expect(screen.getByRole('combobox')).toHaveTextContent('claude-x')
    expect(
      screen.queryByText(/Chat Completions request succeeded in/)
    ).not.toBeInTheDocument()
    expect(mockFetch).toHaveBeenCalledTimes(2)
  })

  it('clears the previous result when selecting another protocol', async () => {
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
      await screen.findByRole('button', { name: 'Send test request' })
    )
    await screen.findByText(/Chat Completions request succeeded in/)
    selectProtocol('Responses')
    expect(
      screen.queryByText(/Chat Completions request succeeded in/)
    ).not.toBeInTheDocument()
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
      await screen.findByRole('button', { name: 'Send test request' })
    )
    expect(screen.getByRole('combobox')).toBeDisabled()
    expect(screen.getByRole('radio', { name: 'Responses' })).toHaveAttribute(
      'aria-disabled',
      'true'
    )
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
      screen.queryByRole('button', { name: 'Send test request' })
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
      await screen.findByRole('button', { name: 'Send test request' })
    )

    expect(
      await screen.findByText(/Chat Completions request succeeded in/)
    ).toBeInTheDocument()
    const [url, init] = mockFetch.mock.calls[1] as [string, RequestInit]
    expect(url).toBe('http://gateway.example.com/v1/chat/completions')
    expect(JSON.parse(String(init.body))).toEqual({
      model: 'gpt-5.5',
      messages: [{ role: 'user', content: 'ping' }],
      max_tokens: 32,
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
      await screen.findByRole('button', { name: 'Send test request' })
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
      await screen.findByRole('button', { name: 'Send test request' })
    )

    expect(
      await screen.findByText(/HTTP 200:.*valid Chat Completions response/)
    ).toBeInTheDocument()
    expect(screen.queryByText(/request succeeded in/)).not.toBeInTheDocument()
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
      screen.queryByRole('button', { name: 'Send test request' })
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
      screen.queryByRole('button', { name: 'Send test request' })
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

  it('sends a finite non-streaming Responses request with its native body', async () => {
    mockFetch
      .mockResolvedValueOnce(jsonResponse(200, { data: [{ id: 'gpt-5.5' }] }))
      .mockResolvedValueOnce(
        jsonResponse(200, {
          id: 'resp_fixture',
          object: 'response',
          status: 'completed',
          error: null,
          output: [
            {
              type: 'reasoning',
              summary: [{ type: 'summary_text', text: 'thinking' }],
            },
            {
              type: 'message',
              role: 'assistant',
              content: [{ type: 'output_text', text: 'pong' }],
            },
          ],
        })
      )
    renderSection()
    await checkModelsAndWait()
    selectProtocol('Responses')
    fireEvent.click(screen.getByRole('button', { name: 'Send test request' }))

    expect(
      await screen.findByText(/Responses request succeeded in/)
    ).toBeInTheDocument()
    const [url, init] = mockFetch.mock.calls[1] as [string, RequestInit]
    expect(url).toBe('http://gateway.example.com/v1/responses')
    expect((init.headers as Record<string, string>).Authorization).toBe(
      'Bearer sk-plain-demo'
    )
    expect(JSON.parse(String(init.body))).toEqual({
      model: 'gpt-5.5',
      input: 'ping',
      max_output_tokens: 32,
      stream: false,
    })
  })

  it('sends one Anthropic Messages request with native headers and body', async () => {
    mockFetch
      .mockResolvedValueOnce(jsonResponse(200, { data: [{ id: 'claude-x' }] }))
      .mockResolvedValueOnce(
        jsonResponse(200, {
          id: 'msg_fixture',
          type: 'message',
          role: 'assistant',
          content: [
            { type: 'thinking', thinking: 'thinking' },
            { type: 'text', text: 'pong' },
          ],
          stop_reason: 'end_turn',
          error: null,
        })
      )
    renderSection()
    await checkModelsAndWait([{ id: 'claude-x' }])
    selectProtocol('Anthropic Messages')
    fireEvent.click(screen.getByRole('button', { name: 'Send test request' }))

    expect(
      await screen.findByText(/Anthropic Messages request succeeded in/)
    ).toBeInTheDocument()
    const [url, init] = mockFetch.mock.calls[1] as [string, RequestInit]
    expect(url).toBe('http://gateway.example.com/v1/messages')
    expect(init.headers).toMatchObject({
      'x-api-key': 'sk-plain-demo',
      'anthropic-version': '2023-06-01',
    })
    expect(
      (init.headers as Record<string, string>).Authorization
    ).toBeUndefined()
    expect(JSON.parse(String(init.body))).toEqual({
      model: 'claude-x',
      max_tokens: 32,
      messages: [{ role: 'user', content: 'ping' }],
      stream: false,
    })
  })

  it.each([
    {
      protocol: 'Chat Completions' as const,
      response: {
        choices: [{ message: { role: 'assistant', content: '   ' } }],
        error: null,
      },
    },
    {
      protocol: 'Responses' as const,
      response: {
        object: 'response',
        status: 'completed',
        error: null,
        output: [
          {
            type: 'message',
            role: 'assistant',
            content: [{ type: 'output_text', text: '' }],
          },
        ],
      },
    },
    {
      protocol: 'Anthropic Messages' as const,
      response: {
        type: 'message',
        role: 'assistant',
        content: [],
        stop_reason: 'end_turn',
        error: null,
      },
    },
    {
      protocol: 'Responses' as const,
      response: {
        object: 'response',
        status: 'completed',
        error: null,
        output: [
          {
            type: 'reasoning',
            summary: [{ type: 'summary_text', text: 'thinking only' }],
          },
        ],
      },
    },
    {
      protocol: 'Responses' as const,
      response: {
        object: 'response',
        status: 'incomplete',
        error: null,
        output: [
          {
            type: 'message',
            role: 'assistant',
            content: [{ type: 'output_text', text: 'pong' }],
          },
        ],
      },
    },
    {
      protocol: 'Responses' as const,
      response: {
        object: 'response',
        status: 'completed',
        error: { message: 'upstream failed' },
        output: [
          {
            type: 'message',
            role: 'assistant',
            content: [{ type: 'output_text', text: 'pong' }],
          },
        ],
      },
    },
    {
      protocol: 'Anthropic Messages' as const,
      response: {
        type: 'message',
        role: 'assistant',
        content: [{ type: 'thinking', thinking: 'thinking only' }],
        stop_reason: 'end_turn',
        error: null,
      },
    },
  ])(
    'rejects an HTTP 200 $protocol response without assistant text',
    async ({ protocol, response }) => {
      mockFetch
        .mockResolvedValueOnce(jsonResponse(200, { data: [{ id: 'gpt-5.5' }] }))
        .mockResolvedValueOnce(jsonResponse(200, response))
      renderSection()
      await checkModelsAndWait()
      selectProtocol(protocol)
      fireEvent.click(screen.getByRole('button', { name: 'Send test request' }))

      expect(
        await screen.findByText(/HTTP 200:.*valid .* response/)
      ).toBeInTheDocument()
      expect(screen.queryByText(/request succeeded in/)).not.toBeInTheDocument()
    }
  )

  it('does not retry with another protocol after the selected protocol fails', async () => {
    mockFetch
      .mockResolvedValueOnce(jsonResponse(200, { data: [{ id: 'gpt-5.5' }] }))
      .mockResolvedValueOnce(
        jsonResponse(503, { error: { message: 'Responses unavailable' } })
      )
      .mockResolvedValueOnce(
        jsonResponse(200, {
          choices: [{ message: { role: 'assistant', content: 'pong' } }],
        })
      )
    renderSection()
    await checkModelsAndWait()
    selectProtocol('Responses')
    fireEvent.click(screen.getByRole('button', { name: 'Send test request' }))

    expect(
      await screen.findByText(/HTTP 503: Responses unavailable/)
    ).toBeInTheDocument()
    expect(mockFetch).toHaveBeenCalledTimes(2)
  })

  it('renders the protocol selector and request action in Chinese', async () => {
    await i18n.changeLanguage('zhCN')
    mockFetch.mockResolvedValueOnce(
      jsonResponse(200, { data: [{ id: 'gpt-5.5' }] })
    )
    try {
      renderSection()
      fireEvent.click(screen.getByRole('button', { name: '检查可用模型' }))
      expect(await screen.findByText('协议')).toBeInTheDocument()
      expect(
        screen.getByRole('radio', { name: 'Chat Completions' })
      ).toBeInTheDocument()
      expect(
        screen.getByRole('radio', { name: 'Responses' })
      ).toBeInTheDocument()
      expect(
        screen.getByRole('radio', { name: 'Anthropic Messages' })
      ).toBeInTheDocument()
      expect(
        screen.getByRole('button', { name: '发送测试请求' })
      ).toBeInTheDocument()
    } finally {
      await i18n.changeLanguage('en')
    }
  })
})
