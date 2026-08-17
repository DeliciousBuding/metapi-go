// metapi-go/features/model-tester — error-mapping + payload unit tests.
//
// `resolveTestResponseError` turns non-ok test responses into user-facing
// messages: backend 501s are known limitations (the sync harness requires a
// forced channel) and map to a friendly localized key instead of the raw
// "not implemented" text. `buildChatPayload` renders the conversation history
// into the request `messages` array and forwards the targeted `channelId`.

import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  buildChatPayload,
  resolveTestResponseError,
  runChatProbe,
} from '../api'
import type { ChatMessage, TestFormValues } from '../types'

const testChatSync = vi.hoisted(() => vi.fn())

vi.mock('@/lib/api', () => ({
  api: { testChatSync },
}))

beforeEach(() => {
  testChatSync.mockReset()
})

function formValues(overrides: Partial<TestFormValues> = {}): TestFormValues {
  return {
    model: 'gpt-4o',
    systemPrompt: '',
    prompt: 'current question',
    targetFormat: 'openai',
    temperature: 0.7,
    topP: 1,
    maxTokens: 1024,
    compareChannels: false,
    channelIds: [],
    ...overrides,
  }
}

function historyTurn(role: ChatMessage['role'], content: string): ChatMessage {
  return { id: `${role}-${content}`, role, content }
}

describe('buildChatPayload', () => {
  it('keeps the single-shot shape when history is empty', () => {
    const payload = buildChatPayload(formValues())
    expect(payload.messages).toEqual([
      { role: 'user', content: 'current question' },
    ])
  })

  it('prepends the system prompt, then history, then the current prompt', () => {
    const payload = buildChatPayload(
      formValues({ systemPrompt: '  behave  ' }),
      [
        historyTurn('user', 'first question'),
        historyTurn('assistant', 'first answer'),
      ]
    )
    expect(payload.messages).toEqual([
      { role: 'system', content: 'behave' },
      { role: 'user', content: 'first question' },
      { role: 'assistant', content: 'first answer' },
      { role: 'user', content: 'current question' },
    ])
  })

  it('maps sampling params onto the wire (snake_case + omitted max_tokens at 0)', () => {
    const payload = buildChatPayload(
      formValues({ temperature: 0.2, topP: 0.9, maxTokens: 0 })
    )
    expect(payload.temperature).toBe(0.2)
    expect(payload.top_p).toBe(0.9)
    expect(payload.max_tokens).toBeUndefined()
    expect(payload.targetFormat).toBe('openai')
  })

  it('forwards channelId when a channel is targeted', () => {
    const payload = buildChatPayload(formValues({ channelId: 42 }))
    expect(payload.channelId).toBe(42)
  })

  it('omits channelId when no channel is targeted', () => {
    const payload = buildChatPayload(formValues())
    expect(payload.channelId).toBeUndefined()
  })
})

describe('resolveTestResponseError', () => {
  it('maps the sync 501 (forced-channel harness required) to notAvailable', async () => {
    const response = new Response(
      JSON.stringify({
        success: false,
        message:
          'Chat test requires channelId, siteId, or forcedChannelId for the forced-channel harness',
      }),
      { status: 501 }
    )
    expect(await resolveTestResponseError(response)).toBe(
      'modelTester.error.notAvailable'
    )
  })

  it('maps 401 to the session-expired key', async () => {
    const response = new Response(JSON.stringify({ message: 'unauthorized' }), {
      status: 401,
    })
    expect(await resolveTestResponseError(response)).toBe(
      'modelTester.error.sessionExpired'
    )
  })

  it('maps 403 to the session-expired key', async () => {
    const response = new Response(JSON.stringify({ error: 'forbidden' }), {
      status: 403,
    })
    expect(await resolveTestResponseError(response)).toBe(
      'modelTester.error.sessionExpired'
    )
  })

  it('falls back to the parsed backend error text for other statuses', async () => {
    const response = new Response(
      JSON.stringify({ success: false, message: 'boom' }),
      { status: 400 }
    )
    expect(await resolveTestResponseError(response)).toBe('boom')
  })

  it('falls back to HTTP status when the body is not JSON', async () => {
    const response = new Response('plain text body', { status: 500 })
    expect(await resolveTestResponseError(response)).toBe('plain text body')
  })
})

describe('runChatProbe', () => {
  it('unwraps the harness envelope and preserves upstream timing truth', async () => {
    const truncatedBody = JSON.stringify({
      choices: [
        {
          message: { content: 'upstream answer' },
          finish_reason: 'stop',
        },
      ],
    })
    testChatSync.mockResolvedValue(
      new Response(
        JSON.stringify({
          success: true,
          statusCode: 200,
          latencyMs: 321,
          truncatedBody,
          error: null,
        }),
        { status: 200 }
      )
    )

    const result = await runChatProbe(buildChatPayload(formValues()))

    expect(result).toEqual({
      content: 'upstream answer',
      reasoningContent: '',
      doneReceived: true,
      statusCode: 200,
      latencyMs: 321,
      rawEvents: [truncatedBody],
      empty: false,
      error: undefined,
    })
  })

  it('extracts token usage from the bounded upstream body', async () => {
    const truncatedBody = JSON.stringify({
      choices: [
        {
          message: { content: 'upstream answer' },
          finish_reason: 'stop',
        },
      ],
      usage: {
        prompt_tokens: 12,
        completion_tokens: 8,
        total_tokens: 20,
      },
    })
    testChatSync.mockResolvedValue(
      new Response(
        JSON.stringify({
          success: true,
          statusCode: 200,
          latencyMs: 321,
          truncatedBody,
          error: null,
        }),
        { status: 200 }
      )
    )

    const result = await runChatProbe(buildChatPayload(formValues()))

    expect(result.promptTokens).toBe(12)
    expect(result.completionTokens).toBe(8)
    expect(result.totalTokens).toBe(20)
  })

  it('derives total tokens when the upstream omits total_tokens', async () => {
    const truncatedBody = JSON.stringify({
      choices: [{ message: { content: 'ok' }, finish_reason: 'stop' }],
      usage: { input_tokens: 5, output_tokens: 7 },
    })
    testChatSync.mockResolvedValue(
      new Response(
        JSON.stringify({
          success: true,
          statusCode: 200,
          latencyMs: 10,
          truncatedBody,
          error: null,
        }),
        { status: 200 }
      )
    )

    const result = await runChatProbe(buildChatPayload(formValues()))

    expect(result.promptTokens).toBe(5)
    expect(result.completionTokens).toBe(7)
    expect(result.totalTokens).toBe(12)
  })

  it('surfaces the harness error instead of parsing it as model content', async () => {
    testChatSync.mockResolvedValue(
      new Response(
        JSON.stringify({
          success: false,
          statusCode: 503,
          latencyMs: 45,
          truncatedBody: '{"error":{"message":"provider unavailable"}}',
          error: 'forced channel request failed',
        }),
        { status: 200 }
      )
    )

    const result = await runChatProbe(buildChatPayload(formValues()))

    expect(result.content).toBe('')
    expect(result.statusCode).toBe(503)
    expect(result.latencyMs).toBe(45)
    expect(result.error).toBe('forced channel request failed')
  })

  it('rejects malformed harness responses explicitly', async () => {
    testChatSync.mockResolvedValue(
      new Response(JSON.stringify({ content: 'not an envelope' }), {
        status: 200,
      })
    )

    await expect(runChatProbe(buildChatPayload(formValues()))).rejects.toThrow(
      'Invalid chat test harness response'
    )
  })
})
