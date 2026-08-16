/* eslint-disable no-nested-ternary -- error-shape selection uses chained ternaries */
// metapi-go/features/model-tester — TanStack Query mutation for the chat test.
//
// `useTestModel` is a `useMutation` whose `mutationFn` runs the chat test:
// it posts a single sync request (`api.testChatSync`) to `/api/test/chat`
// (the forced-channel harness) and parses the JSON body as one delta,
// normalizing across OpenAI / Claude / Responses / Gemini protocols (ported
// from the legacy `pages/ModelTester.tsx` parser). The single delta is
// forwarded to the caller's `onDelta` callback so the response viewer renders
// the content/reasoning; when the test finishes the resolved `TestResponse`
// summary is returned and `onDone` fires. The caller passes an `AbortSignal`
// so the Stop button cancels an in-flight test.
//
// The tester is sync-only by design: the Go backend returns an honest 501 for
// `/api/test/chat/stream` (SSE is not implemented), so the UI no longer offers
// a stream toggle or a synthetic chunk path. `resolveTestResponseError` maps
// that 501 residual (and the "requires channelId" residual) to a friendly
// localized key.
//
// `buildChatPayload` renders the conversation history into the request
// `messages` array (system → prior user/assistant turns → current prompt)
// so multi-turn context travels on the wire, and forwards the selected
// `channelId` when the operator targets a specific channel.
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
    ...(values.channelId ? { channelId: values.channelId } : {}),
    targetFormat: values.targetFormat,
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

/**
 * Normalize a parsed event payload into a content/reasoning/done delta.
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
 * Run a single sync chat test. Posts to `/api/test/chat` (the forced-channel
 * harness) and parses the single JSON body as one delta so the viewer renders
 * the final content. Forwards the delta to `onDelta` and resolves to a
 * `TestResponse` summary; throws on auth failure, non-ok responses, or caller
 * abort.
 */
async function runChatTest(
  variables: TestModelVariables
): Promise<TestResponse> {
  const { payload, history, onDelta, onDone, signal } = variables
  const chatPayload = buildChatPayload(payload, history)
  const startedAt = performance.now()

  const response = await api.testChatSync(chatPayload, signal)

  if (!response.ok) {
    throw new Error(await resolveTestResponseError(response))
  }

  let content = ''
  let reasoningContent = ''
  let doneReceived = false
  const rawEvents: string[] = []

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

export function useTestModel() {
  return useMutation<TestResponse, Error, TestModelVariables>({
    mutationFn: runChatTest,
  })
}
