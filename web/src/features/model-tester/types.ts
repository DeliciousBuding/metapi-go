// metapi-go/features/model-tester — domain types for the model tester.
//
// The tester drives `api.testChatStream`, which POSTs a
// `TestChatRequestPayload` to `/api/test/chat/stream` and returns a raw
// `Response` whose body is an SSE stream. These types model the request
// envelope, the per-chunk delta (normalized across OpenAI / Claude /
// Responses / Gemini protocols), and the finalised response the viewer
// renders after the stream closes.

export type TestTargetFormat = 'openai' | 'claude' | 'responses' | 'gemini'

/**
 * Form values produced by `testerSchema`. The tester builds the
 * `TestChatRequestPayload.messages` array from `systemPrompt` (role=system,
 * when present) + `prompt` (role=user). Numeric fields are plain `z.number()`
 * so RHF input/output types match.
 */
export type TestFormValues = {
  model: string
  systemPrompt: string
  prompt: string
  targetFormat: TestTargetFormat
  temperature: number
  topP: number
  maxTokens: number
  stream: boolean
}

/**
 * Variables accepted by the `useTestModel` mutation. `onDelta` is invoked
 * for every parsed SSE chunk during streaming so the viewer can render
 * content/reasoning as it arrives; `onDone` fires once when the stream
 * closes (success or empty). `signal` lets the caller abort an in-flight
 * test (the Stop button).
 */
export type TestModelVariables = {
  payload: TestFormValues
  onDelta?: (delta: TestStreamDelta) => void
  onDone?: (summary: TestResponse) => void
  signal?: AbortSignal
}

/**
 * Normalized per-chunk delta. `contentDelta` is the assistant text token;
 * `reasoningDelta` is the chain-of-thought text token (Claude thinking /
 * DeepSeek reasoning / Responses reasoning summary). `done` marks a
 * terminal structural event (finish_reason / message_stop / response.completed).
 */
export type TestStreamDelta = {
  contentDelta?: string
  reasoningDelta?: string
  done?: boolean
}

/**
 * Finalised test response the viewer renders after the stream closes.
 * `content` / `reasoningContent` are the accumulated full strings; `latencyMs`
 * is the wall-clock from request start to stream end; `chunks` is the count
 * of SSE data events consumed (capped for memory); `rawEvents` stores the
 * last N raw data lines for the JSON/raw debug tab.
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
 * Wire shape for `/api/test/chat/stream`. Defined locally because
 * `lib/api.ts` keeps `TestChatRequestPayload` un-exported during the
 * rewrite; the structural shape is identical so `api.testChatStream`
 * accepts it via structural typing.
 */
export type ChatTestPayload = {
  model: string
  messages: Array<{ role: string; content: string }>
  targetFormat?: TestTargetFormat
  stream?: boolean
  temperature?: number
  top_p?: number
  max_tokens?: number
}
