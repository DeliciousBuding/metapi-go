import { api } from '@/lib/api'

import { extractErrorMessage, normalizeProbeResponse } from './probe-response'
import type {
  ChatMessage,
  ChatTestPayload,
  TestFormValues,
  TestResponse,
} from './types'

export function buildChatPayload(
  values: TestFormValues,
  history: ChatMessage[] = []
): ChatTestPayload {
  const messages: Array<{ role: string; content: string }> = []
  if (values.systemPrompt.trim().length > 0) {
    messages.push({ role: 'system', content: values.systemPrompt.trim() })
  }
  for (const [index, message] of history.entries()) {
    const next = history[index + 1]
    if (message.error || message.response?.empty) continue
    if (
      message.role === 'user' &&
      next?.role === 'assistant' &&
      (next.error || next.response?.empty)
    )
      continue
    messages.push({ role: message.role, content: message.content })
  }
  messages.push({ role: 'user', content: values.prompt })
  const enabled = values.enabledParams ?? {
    temperature: true,
    topP: true,
    maxTokens: true,
  }
  return {
    model: values.model,
    messages,
    ...(values.channelId ? { channelId: values.channelId } : {}),
    targetFormat: values.targetFormat,
    ...(enabled.temperature ? { temperature: values.temperature } : {}),
    ...(enabled.topP ? { top_p: values.topP } : {}),
    ...(enabled.maxTokens && values.maxTokens > 0
      ? { max_tokens: values.maxTokens }
      : {}),
  }
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
 * than browser timing. Token usage is extracted from the upstream body when
 * present; the harness does not compute estimated cost (only the proxy
 * executor billing path does, which this probe bypasses).
 */
export async function runChatProbe(
  chatPayload: ChatTestPayload,
  signal?: AbortSignal
): Promise<TestResponse> {
  const response = await api.testChatSync(chatPayload, signal)

  if (!response.ok) {
    throw new Error(await resolveTestResponseError(response))
  }

  return normalizeProbeResponse(await response.text())
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
