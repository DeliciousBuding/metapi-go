// metapi-go/features/model-tester — domain types for the model tester.
//
// The tester drives `api.testChatSync`, which POSTs a `ChatTestPayload` to
// `/api/test/chat` (the forced-channel harness). The endpoint returns a
// harness envelope whose `truncatedBody` contains the bounded upstream
// response. These types model the request, normalized upstream content, and
// the final response the viewer renders after the synchronous probe finishes.

export type TestTargetFormat = 'openai' | 'claude' | 'responses' | 'gemini'

/**
 * One turn in the tester conversation. The user prompt is appended when a
 * run starts; the assistant turn is appended when the run finishes with
 * non-empty content. `id` is a UI-only stable key for list rendering — the
 * wire payload only carries `role` + `content`.
 */
export type ChatMessage = {
  id: string
  role: 'user' | 'assistant'
  content: string
}

/**
 * Form values produced by `testerSchema`. The tester builds the
 * `ChatTestPayload.messages` array from `systemPrompt` (role=system, when
 * present) + `prompt` (role=user). `channelId` targets a specific channel via
 * the forced-channel harness; when absent the backend returns its honest 501
 * "requires channelId" residual. `compareChannels` + `channelIds` drive the
 * batch latency comparison (single-run path keeps using `channelId`).
 * Numeric fields are plain `z.number()` so RHF input/output types match.
 */
export type TestFormValues = {
  model: string
  channelId?: number
  compareChannels: boolean
  channelIds?: number[]
  systemPrompt: string
  prompt: string
  targetFormat: TestTargetFormat
  temperature: number
  topP: number
  maxTokens: number
}

/**
 * Variables accepted by the `useTestModel` mutation. `history` carries the
 * prior conversation turns (excluding the current user prompt, which
 * `buildChatPayload` appends last) so multi-turn context is sent on the
 * wire when the backend honors the `messages` array. `onDelta` is invoked
 * with the single normalized delta so the viewer can render content /
 * reasoning; `onDone` fires once when the run finishes (success or empty).
 * `signal` lets the caller abort an in-flight test (the Stop button).
 */
export type TestModelVariables = {
  payload: TestFormValues
  history?: ChatMessage[]
  onDelta?: (delta: TestStreamDelta) => void
  onDone?: (summary: TestResponse) => void
  signal?: AbortSignal
}

/**
 * Normalized response delta. `contentDelta` is the assistant text;
 * `reasoningDelta` is the chain-of-thought text (Claude thinking / DeepSeek
 * reasoning / Responses reasoning summary). `done` marks a terminal
 * structural event (finish_reason / message_stop / response.completed).
 */
export type TestStreamDelta = {
  contentDelta?: string
  reasoningDelta?: string
  done?: boolean
}

/**
 * Finalised test response the viewer renders after the run finishes.
 * `content` / `reasoningContent` come from the harness's bounded upstream
 * body. `statusCode`, `latencyMs`, and `error` preserve upstream harness
 * truth. `promptTokens` / `completionTokens` / `totalTokens` are extracted
 * from the upstream body's `usage` object (OpenAI `prompt_tokens` /
 * Claude `input_tokens` / Gemini `usageMetadata`) when the upstream
 * response carries them; absent for non-JSON or truncated bodies. The
 * synchronous harness does not emit stream chunks. `rawEvents` contains
 * the actual upstream body for the raw debug tab.
 */
export type TestResponse = {
  content: string
  reasoningContent: string
  doneReceived: boolean
  statusCode: number
  latencyMs: number
  rawEvents: string[]
  empty: boolean
  error?: string
  promptTokens?: number
  completionTokens?: number
  totalTokens?: number
}

/**
 * Wire shape for `/api/test/chat`. Defined locally because `lib/api.ts` keeps
 * `TestChatRequestPayload` un-exported during the rewrite; the structural
 * shape is identical so `api.testChatSync` accepts it via structural typing.
 */
export type ChatTestPayload = {
  model: string
  messages: Array<{ role: string; content: string }>
  channelId?: number
  targetFormat?: TestTargetFormat
  temperature?: number
  top_p?: number
  max_tokens?: number
}

/**
 * Settled result for one channel in a batch latency comparison. `status`
 * distinguishes a clean response from an upstream error (the probe resolved
 * with a `TestResponse.error`) or a caller abort. `statusCode` and `latencyMs`
 * preserve the harness result when the probe reached the upstream.
 */
export type BatchProbeResult = {
  channelId: number
  status: 'success' | 'failure' | 'aborted'
  statusCode?: number
  latencyMs?: number
  error?: string
}
