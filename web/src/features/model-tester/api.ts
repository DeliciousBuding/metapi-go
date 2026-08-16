/* eslint-disable no-nested-ternary -- error-shape selection uses chained ternaries */
// metapi-go/features/model-tester — TanStack Query mutation for the chat test.
//
// `useTestModel` is a `useMutation` whose `mutationFn` runs the chat test:
// it posts a single sync request (`api.testChatSync`) to `/api/test/chat`
// (the forced-channel harness), unwraps the harness envelope, and parses its
// bounded `truncatedBody` as the actual upstream response. Recognized JSON is
// normalized across OpenAI / Claude / Responses / Gemini protocols; non-JSON
// bodies remain honest plain text. The final content is forwarded once so the
// response viewer can render it, while harness status/latency/error remain the
// source of truth. The caller passes an `AbortSignal` so the Stop button
// cancels an in-flight test.
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
  BatchProbeResult,
  ChatMessage,
  ChatTestPayload,
  TestFormValues,
  TestModelVariables,
  TestResponse,
  TestStreamDelta,
} from './types'

type ChatTestHarnessEnvelope = {
  success: boolean
  statusCode: number
  latencyMs: number
  truncatedBody: string
  error: string | null
}

type ParsedUpstreamBody = {
  content: string
  reasoningContent: string
  doneReceived: boolean
  rawEvents: string[]
  error?: string
}

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

function parseChatTestHarnessEnvelope(
  responseText: string
): ChatTestHarnessEnvelope {
  let payload: unknown
  try {
    payload = JSON.parse(responseText) as unknown
  } catch {
    throw new Error('Invalid chat test harness response')
  }

  if (!payload || typeof payload !== 'object') {
    throw new Error('Invalid chat test harness response')
  }

  const record = payload as Record<string, unknown>
  const errorIsValid = record.error === null || typeof record.error === 'string'
  if (
    typeof record.success !== 'boolean' ||
    typeof record.statusCode !== 'number' ||
    typeof record.latencyMs !== 'number' ||
    typeof record.truncatedBody !== 'string' ||
    !errorIsValid
  ) {
    throw new Error('Invalid chat test harness response')
  }

  return {
    success: record.success,
    statusCode: record.statusCode,
    latencyMs: record.latencyMs,
    truncatedBody: record.truncatedBody,
    error: record.error as string | null,
  }
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
    const message = choice?.message as Record<string, unknown> | undefined
    const reasoningDelta =
      typeof delta.reasoning_content === 'string'
        ? delta.reasoning_content
        : typeof delta.reasoning === 'string'
          ? delta.reasoning
          : typeof message?.reasoning_content === 'string'
            ? message.reasoning_content
            : typeof message?.reasoning === 'string'
              ? message.reasoning
              : ''
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

function parseUpstreamBody(truncatedBody: string): ParsedUpstreamBody {
  if (!truncatedBody) {
    return {
      content: '',
      reasoningContent: '',
      doneReceived: false,
      rawEvents: [],
    }
  }

  try {
    const payload = JSON.parse(truncatedBody) as unknown
    const delta = parseAnyStreamDelta(payload)
    const error = extractErrorMessage(payload)
    return {
      content: delta.contentDelta ?? '',
      reasoningContent: delta.reasoningDelta ?? '',
      doneReceived: Boolean(delta.done),
      rawEvents: [truncatedBody],
      error: error || undefined,
    }
  } catch {
    return {
      content: truncatedBody,
      reasoningContent: '',
      doneReceived: true,
      rawEvents: [truncatedBody],
    }
  }
}

function resolveHarnessError(
  envelope: ChatTestHarnessEnvelope,
  upstreamError?: string
): string | undefined {
  const harnessError = envelope.error?.trim()
  if (harnessError) return harnessError
  if (upstreamError) return upstreamError
  if (envelope.success) return undefined
  if (envelope.statusCode > 0) {
    return `upstream status ${envelope.statusCode}`
  }
  return 'Upstream request failed'
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
 * Run a single sync probe against `/api/test/chat` (the forced-channel
 * harness), unwrap its response envelope, and normalize the bounded upstream
 * body. The returned status, latency, and error come from the harness rather
 * than browser timing. No stream chunks or terminal SSE events are invented.
 */
export async function runChatProbe(
  chatPayload: ChatTestPayload,
  signal?: AbortSignal
): Promise<TestResponse> {
  const response = await api.testChatSync(chatPayload, signal)

  if (!response.ok) {
    throw new Error(await resolveTestResponseError(response))
  }

  const envelope = parseChatTestHarnessEnvelope(await response.text())
  const upstreamBody = parseUpstreamBody(envelope.truncatedBody)
  const error = resolveHarnessError(envelope, upstreamBody.error)

  return {
    content: upstreamBody.content,
    reasoningContent: upstreamBody.reasoningContent,
    doneReceived: upstreamBody.doneReceived,
    statusCode: envelope.statusCode,
    latencyMs: envelope.latencyMs,
    chunks: 0,
    rawEvents: upstreamBody.rawEvents,
    empty: !upstreamBody.content && !upstreamBody.reasoningContent,
    error,
  }
}

/**
 * Detect a caller-triggered abort so the UI can show a "stopped" state
 * instead of a hard error. Handles the message variants produced by
 * `AbortController.abort()` across browsers and the fetch polyfill.
 */
export function isAbortError(error: unknown): boolean {
  if (!(error instanceof Error)) return false
  return (
    error.name === 'AbortError' ||
    error.message === 'This operation was aborted' ||
    error.message === 'The user aborted a request.'
  )
}

/**
 * Run a single sync chat test for the viewer. Builds the payload from form
 * values + history, delegates to `runChatProbe`, then forwards the single
 * delta to `onDelta` and the summary to `onDone`.
 */
async function runChatTest(
  variables: TestModelVariables
): Promise<TestResponse> {
  const { payload, history, onDelta, onDone, signal } = variables
  const summary = await runChatProbe(buildChatPayload(payload, history), signal)
  onDelta?.({
    contentDelta: summary.content,
    reasoningDelta: summary.reasoningContent,
    done: summary.doneReceived,
  })
  onDone?.(summary)
  return summary
}

/**
 * Run a batch of channel probes with bounded concurrency and one shared abort
 * lifecycle, settling every request independently so one slow/failing channel
 * never erases the others. Returns one result per probe in input order.
 */
export async function runBatchComparison(
  probes: Array<{
    channelId: number
    run: (signal?: AbortSignal) => Promise<TestResponse>
  }>,
  options: { concurrency?: number; signal?: AbortSignal } = {}
): Promise<BatchProbeResult[]> {
  const concurrency = Math.max(1, options.concurrency ?? 3)
  const results: BatchProbeResult[] = Array.from({ length: probes.length })
  let nextIndex = 0

  async function worker(): Promise<void> {
    while (nextIndex < probes.length) {
      const index = nextIndex
      nextIndex += 1
      const probe = probes[index]
      results[index] = await settleProbe(
        probe.channelId,
        probe.run,
        options.signal
      )
    }
  }

  const workerCount = Math.min(concurrency, probes.length)
  await Promise.all(Array.from({ length: workerCount }, () => worker()))
  return results
}

async function settleProbe(
  channelId: number,
  run: (signal?: AbortSignal) => Promise<TestResponse>,
  signal?: AbortSignal
): Promise<BatchProbeResult> {
  try {
    const summary = await run(signal)
    if (summary.error) {
      return {
        channelId,
        status: 'failure',
        statusCode: summary.statusCode,
        latencyMs: summary.latencyMs,
        error: summary.error,
      }
    }
    return {
      channelId,
      status: 'success',
      statusCode: summary.statusCode,
      latencyMs: summary.latencyMs,
    }
  } catch (error) {
    if (isAbortError(error)) return { channelId, status: 'aborted' }
    return {
      channelId,
      status: 'failure',
      error: error instanceof Error ? error.message : String(error),
    }
  }
}

/**
 * Order batch results for display: completed successes first, sorted by
 * ascending latency, then failures/aborted entries in their original input
 * order (a stable sort so reruns do not jumble otherwise-identical rows).
 */
export function sortBatchResults(
  results: BatchProbeResult[]
): BatchProbeResult[] {
  return results
    .map((result, index) => ({ result, index }))
    .sort((a, b) => {
      const aSuccess = a.result.status === 'success'
      const bSuccess = b.result.status === 'success'
      if (aSuccess && bSuccess) {
        const latencyDiff =
          (a.result.latencyMs ?? 0) - (b.result.latencyMs ?? 0)
        return latencyDiff !== 0 ? latencyDiff : a.index - b.index
      }
      if (aSuccess !== bSuccess) return aSuccess ? -1 : 1
      return a.index - b.index
    })
    .map((entry) => entry.result)
}

export function useTestModel() {
  return useMutation<TestResponse, Error, TestModelVariables>({
    mutationFn: runChatTest,
  })
}
