/* eslint-disable no-nested-ternary -- error-shape selection uses chained ternaries */
// metapi-go/features/model-tester — TanStack Query mutation for the chat test.
//
// `useTestModel` is a `useMutation` whose `mutationFn` runs the chat test:
// with `stream: true` it opens the SSE stream (`api.testChatStream`) and
// consumes it in a reader loop; otherwise it posts a single sync request
// (`api.testChatSync`) and parses the JSON body as one delta. Both paths
// normalize across OpenAI / Claude / Responses / Gemini protocols (ported
// from the legacy `pages/ModelTester.tsx` parser). Each parsed delta is
// forwarded to the caller's `onDelta` callback so the response viewer can
// render content/reasoning as it arrives; when the test finishes the
// resolved `TestResponse` summary is returned and `onDone` fires. The
// caller passes an `AbortSignal` so the Stop button cancels an in-flight
// test.
//
// `buildChatPayload` renders the conversation history into the request
// `messages` array (system → prior user/assistant turns → current prompt)
// so multi-turn context travels on the wire. Backends that only consume a
// single message keep working: they ignore the extra entries and read the
// last user prompt.
//
// A mutation (rather than a query) is the right shape here: the test is a
// user-initiated command with a single terminal resolution, not a cacheable
// read, and TanStack's `isPending` / `mutateAsync` / `reset` map cleanly
// onto the tester's run/stop/clear lifecycle.

import { useMutation } from '@tanstack/react-query'

import { api } from '@/lib/api'

import type {
  ChatMessage,
  ChatTestPayload,
  TestFormValues,
  TestModelVariables,
  TestResponse,
  TestStreamDelta,
} from './types'

const MAX_RAW_EVENTS = 200

export function buildChatPayload(
  values: TestFormValues,
  history: ChatMessage[] = []
): ChatTestPayload {
  const messages: Array<{ role: string; content: string }> = []
  if (values.systemPrompt.trim().length > 0) {
    messages.push({ role: 'system', content: values.systemPrompt.trim() })
  }
  for (const message of history) {
    messages.push({ role: message.role, content: message.content })
  }
  messages.push({ role: 'user', content: values.prompt })
  return {
    model: values.model,
    messages,
    targetFormat: values.targetFormat,
    stream: values.stream,
    temperature: values.temperature,
    top_p: values.topP,
    max_tokens: values.maxTokens > 0 ? values.maxTokens : undefined,
  }
}

function extractErrorMessage(payload: unknown): string {
  if (!payload || typeof payload !== 'object') return ''
  const record = payload as Record<string, unknown>
  const message =
    typeof record.message === 'string'
      ? record.message
      : typeof record.error === 'string'
        ? record.error
        : typeof record.error === 'object' && record.error !== null
          ? (record.error as Record<string, unknown>).message
          : undefined
  return typeof message === 'string' ? message : ''
}

function parseSseBlock(block: string): { event: string; data: string | null } {
  const lines = block.split(/\r?\n/)
  let event = 'message'
  const dataLines: string[] = []
  for (const line of lines) {
    if (line.startsWith('event:')) {
      event = line.slice(6).trim()
      continue
    }
    if (line.startsWith('data:')) {
      dataLines.push(line.slice(5).trimStart())
    }
  }
  return {
    event,
    data: dataLines.length > 0 ? dataLines.join('\n') : null,
  }
}

/**
 * Normalize a parsed SSE event payload into a content/reasoning/done delta.
 * Faithful port of the legacy `parseAnyStreamDelta`: handles OpenAI
 * `choices[].delta`, Anthropic `content_block_delta` / `message_stop`,
 * Responses-API `response.output_text.delta` /
 * `response.reasoning_summary_text.delta` / `response.completed`, and
 * Gemini `candidates[].content.parts[]`.
 */
function parseAnyStreamDelta(eventPayload: unknown): TestStreamDelta {
  if (!eventPayload || typeof eventPayload !== 'object') return {}
  const payload = eventPayload as Record<string, unknown>

  if (Array.isArray(payload.choices)) {
    const choice = payload.choices[0] as Record<string, unknown> | undefined
    const delta = (choice?.delta ?? {}) as Record<string, unknown>
    const reasoningDelta =
      typeof delta.reasoning_content === 'string'
        ? delta.reasoning_content
        : typeof delta.reasoning === 'string'
          ? delta.reasoning
          : ''
    const message = choice?.message as Record<string, unknown> | undefined
    const contentDelta =
      typeof delta.content === 'string'
        ? delta.content
        : typeof message?.content === 'string'
          ? message.content
          : ''
    return {
      contentDelta: contentDelta || undefined,
      reasoningDelta: reasoningDelta || undefined,
      done: Boolean(choice?.finish_reason),
    }
  }

  const type = typeof payload.type === 'string' ? payload.type : ''
  if (type) {
    if (
      type === 'response.output_item.added' ||
      type === 'response.output_item.done' ||
      type === 'response.content_part.added' ||
      type === 'response.content_part.done' ||
      type === 'response.output_text.done'
    ) {
      return {}
    }
    if (type === 'response.output_text.delta') {
      const delta = payload.delta
      const text = payload.text
      const content =
        typeof delta === 'string' ? delta : typeof text === 'string' ? text : ''
      return { contentDelta: content || undefined }
    }
    if (
      type === 'response.reasoning_summary_text.delta' ||
      type === 'response.reasoning.delta'
    ) {
      const delta = payload.delta
      const text = payload.text
      const reasoning =
        typeof delta === 'string' ? delta : typeof text === 'string' ? text : ''
      return { reasoningDelta: reasoning || undefined }
    }
    if (type === 'response.completed' || type === 'response.failed') {
      return { done: true }
    }
    if (type === 'content_block_delta') {
      const delta = (payload.delta ?? {}) as Record<string, unknown>
      const deltaType = typeof delta.type === 'string' ? delta.type : ''
      const text = typeof delta.text === 'string' ? delta.text : ''
      if (deltaType === 'thinking_delta') {
        return { reasoningDelta: text || undefined }
      }
      return { contentDelta: text || undefined }
    }
    if (type === 'content_block_start') {
      const block = (payload.content_block ?? {}) as Record<string, unknown>
      const text = typeof block.text === 'string' ? block.text : ''
      return { contentDelta: text || undefined }
    }
    if (type === 'message_delta') {
      const messageDelta = (payload.delta ?? {}) as Record<string, unknown>
      const stopReason =
        typeof messageDelta.stop_reason === 'string'
          ? messageDelta.stop_reason
          : typeof payload.stop_reason === 'string'
            ? payload.stop_reason
            : ''
      return { done: Boolean(stopReason) }
    }
    if (type === 'message_stop') {
      return { done: true }
    }
  }

  if (Array.isArray(payload.candidates)) {
    const candidate = payload.candidates[0] as
      | Record<string, unknown>
      | undefined
    const content = candidate?.content as Record<string, unknown> | undefined
    const parts = content?.parts as unknown[] | undefined
    if (Array.isArray(parts)) {
      const reasoningDelta = parts
        .filter((item) => {
          const part = item as Record<string, unknown>
          return part?.thought === true
        })
        .map((item) => {
          const part = item as Record<string, unknown>
          return typeof part?.text === 'string' ? part.text : ''
        })
        .join('')
      const contentDelta = parts
        .filter((item) => {
          const part = item as Record<string, unknown>
          return !(part?.thought === true)
        })
        .map((item) => {
          const part = item as Record<string, unknown>
          return typeof part?.text === 'string' ? part.text : ''
        })
        .join('')
      return {
        contentDelta: contentDelta || undefined,
        reasoningDelta: reasoningDelta || undefined,
        done: Boolean(candidate?.finishReason),
      }
    }
  }

  return {}
}

async function parseStreamErrorText(response: Response): Promise<string> {
  try {
    const text = await response.text()
    if (!text) return `HTTP ${response.status}`
    try {
      const parsed = JSON.parse(text) as unknown
      const message = extractErrorMessage(parsed)
      return message || text
    } catch {
      return text
    }
  } catch {
    return `HTTP ${response.status}`
  }
}

/**
 * Map a non-ok test response to a user-facing error.
 *
 * Backend 501s are known limitations (no fake SSE stream; the sync harness
 * requires a forced channel), so surface a friendly localized key instead of
 * the raw "not implemented" text. Auth failures map to the session-expired
 * key; every other status falls back to the parsed backend error text.
 */
export async function resolveTestResponseError(
  response: Response
): Promise<string> {
  if (response.status === 501) return 'modelTester.error.notAvailable'
  if (response.status === 401 || response.status === 403) {
    return 'modelTester.error.sessionExpired'
  }
  return parseStreamErrorText(response)
}

/**
 * Run a single chat test (streaming or sync, per `payload.stream`). Streams
 * forward each parsed delta to `onDelta` and resolve to a `TestResponse`
 * summary when the stream closes; sync mode parses the single JSON body as
 * one delta. Throws on auth failure, non-ok responses, or caller abort.
 */
async function runTestStream(
  variables: TestModelVariables
): Promise<TestResponse> {
  const { payload, history, onDelta, onDone, signal } = variables
  const chatPayload = buildChatPayload(payload, history)
  const startedAt = performance.now()

  let content = ''
  let reasoningContent = ''
  let doneReceived = false
  let chunks = 0
  const rawEvents: string[] = []

  const response = payload.stream
    ? await api.testChatStream(chatPayload, signal)
    : await api.testChatSync(chatPayload, signal)

  if (!response.ok) {
    throw new Error(await resolveTestResponseError(response))
  }
  if (!response.body) {
    throw new Error('modelTester.error.emptyStream')
  }

  // Non-streaming mode: the backend returns a single JSON body. Parse it as
  // one delta so the viewer still renders the final content.
  if (!payload.stream) {
    const text = await response.text()
    if (text) {
      rawEvents.push(text)
      try {
        const parsed = JSON.parse(text) as unknown
        const errorText = extractErrorMessage(parsed)
        if (errorText) {
          const summary: TestResponse = {
            content: '',
            reasoningContent: '',
            doneReceived: true,
            latencyMs: Math.round(performance.now() - startedAt),
            chunks: 1,
            rawEvents,
            empty: true,
            error: errorText,
          }
          onDone?.(summary)
          return summary
        }
        const delta = parseAnyStreamDelta(parsed)
        content = delta.contentDelta ?? ''
        reasoningContent = delta.reasoningDelta ?? ''
        doneReceived = Boolean(delta.done)
      } catch {
        content = text
        doneReceived = true
      }
    }
    const summary: TestResponse = {
      content,
      reasoningContent,
      doneReceived,
      latencyMs: Math.round(performance.now() - startedAt),
      chunks: 1,
      rawEvents,
      empty: !content && !reasoningContent,
    }
    onDelta?.({
      contentDelta: content,
      reasoningDelta: reasoningContent,
      done: true,
    })
    onDone?.(summary)
    return summary
  }

  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''

  while (true) {
    const { done, value } = await reader.read()
    if (done) break
    if (!value) continue

    buffer += decoder.decode(value, { stream: true })
    const events = buffer.split(/\r?\n\r?\n/)
    buffer = events.pop() ?? ''

    for (const eventBlock of events) {
      const parsed = parseSseBlock(eventBlock)
      if (!parsed.data) continue

      rawEvents.push(parsed.data)
      if (rawEvents.length > MAX_RAW_EVENTS) {
        rawEvents.splice(0, rawEvents.length - MAX_RAW_EVENTS)
      }
      chunks += 1

      if (parsed.data === '[DONE]') {
        doneReceived = true
        continue
      }

      let eventPayload: unknown
      try {
        eventPayload = JSON.parse(parsed.data)
      } catch {
        continue
      }

      const errorText = extractErrorMessage(eventPayload)
      if (errorText) {
        throw new Error(errorText)
      }

      const delta = parseAnyStreamDelta(eventPayload)
      if (delta.contentDelta) content += delta.contentDelta
      if (delta.reasoningDelta) reasoningContent += delta.reasoningDelta
      if (delta.done) doneReceived = true
      if (delta.contentDelta || delta.reasoningDelta || delta.done) {
        onDelta?.(delta)
      }
    }
  }

  const latencyMs = Math.round(performance.now() - startedAt)
  const empty = !content && !reasoningContent
  const summary: TestResponse = {
    content,
    reasoningContent,
    doneReceived,
    latencyMs,
    chunks,
    rawEvents,
    empty,
  }
  onDone?.(summary)
  return summary
}

export function useTestModel() {
  return useMutation<TestResponse, Error, TestModelVariables>({
    mutationFn: runTestStream,
  })
}
