// metapi-go/features/model-tester — domain types for the model tester.
//
// The tester drives `api.testChatSync`, which POSTs a `ChatTestPayload` to
// `/api/test/chat` (the forced-channel harness) and returns a single JSON
// body. These types model the request envelope, the normalized response
// delta (across OpenAI / Claude / Responses / Gemini protocols), and the
// finalised response the viewer renders after the run finishes.

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
 * "requires channelId" residual. Numeric fields are plain `z.number()` so RHF
 * input/output types match.
 */
export type TestFormValues = {
  model: string
  channelId?: number
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
 * `content` / `reasoningContent` are the accumulated full strings; `latencyMs`
 * is the wall-clock from request start to completion; `chunks` is the count
 * of data events consumed; `rawEvents` stores the last N raw data lines for
 * the JSON/raw debug tab.
 */
export type TestResponse = {
  content: string
  reasoningContent: string
  doneReceived: boolean
  latencyMs: number
  chunks: number
  rawEvents: string[]
  empty: boolean
  error?: string
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
