import { z } from 'zod'

export type CredentialVerifyProtocol = 'chat' | 'responses' | 'messages'

const VERIFY_MAX_OUTPUT_TOKENS = 32
const ANTHROPIC_VERSION = '2023-06-01'
const chatResponseSchema = z.object({
  choices: z
    .array(
      z.object({
        message: z.object({
          role: z.literal('assistant'),
          content: z.string().trim().min(1),
        }),
      })
    )
    .min(1),
})
const responsesResponseSchema = z.object({
  object: z.literal('response'),
  status: z.literal('completed'),
  output: z.array(z.unknown()).min(1),
})
const messagesResponseSchema = z.object({
  type: z.literal('message'),
  role: z.literal('assistant'),
  content: z.array(z.unknown()).min(1),
})

/** Parsed upstream/gateway error body, truncated — never the key. */
export async function readErrorMessage(res: Response): Promise<string> {
  const text = (await res.text()).slice(0, 500)
  try {
    const parsed = JSON.parse(text) as {
      error?: { message?: unknown }
      message?: unknown
    }
    const message = parsed.error?.message ?? parsed.message
    if (typeof message === 'string' && message.trim() !== '') return message
  } catch {
    // not JSON — fall through to the raw text
  }
  return text
}

export function classifyFetchError(
  error: unknown,
  timeoutCopy: string
): { status: number | null; message: string } {
  if (error instanceof DOMException && error.name === 'AbortError') {
    return { status: null, message: timeoutCopy }
  }
  return {
    status: null,
    message: error instanceof Error ? error.message : String(error),
  }
}

export function verificationRequest(
  protocol: CredentialVerifyProtocol,
  apiBase: string,
  model: string,
  apiKey: string
): { url: string; headers: Record<string, string>; body: unknown } {
  if (protocol === 'responses') {
    return {
      url: `${apiBase}/responses`,
      headers: {
        Authorization: `Bearer ${apiKey}`,
        'Content-Type': 'application/json',
      },
      body: {
        model,
        input: 'ping',
        max_output_tokens: VERIFY_MAX_OUTPUT_TOKENS,
        stream: false,
      },
    }
  }
  if (protocol === 'messages') {
    return {
      url: `${apiBase}/messages`,
      headers: {
        'x-api-key': apiKey,
        'anthropic-version': ANTHROPIC_VERSION,
        'Content-Type': 'application/json',
      },
      body: {
        model,
        max_tokens: VERIFY_MAX_OUTPUT_TOKENS,
        messages: [{ role: 'user', content: 'ping' }],
        stream: false,
      },
    }
  }
  return {
    url: `${apiBase}/chat/completions`,
    headers: {
      Authorization: `Bearer ${apiKey}`,
      'Content-Type': 'application/json',
    },
    body: {
      model,
      messages: [{ role: 'user', content: 'ping' }],
      max_tokens: VERIFY_MAX_OUTPUT_TOKENS,
      stream: false,
    },
  }
}

export function validVerificationResponse(
  protocol: CredentialVerifyProtocol,
  body: unknown
): boolean {
  if (!body || typeof body !== 'object') return false
  const error = (body as Record<string, unknown>).error
  if (error !== undefined && error !== null) return false

  if (protocol === 'responses') {
    const parsed = responsesResponseSchema.safeParse(body)
    if (!parsed.success) return false
    return parsed.data.output.some(
      (item) =>
        item !== null &&
        typeof item === 'object' &&
        (item as Record<string, unknown>).type === 'message' &&
        (item as Record<string, unknown>).role === 'assistant' &&
        Array.isArray((item as Record<string, unknown>).content) &&
        ((item as Record<string, unknown>).content as unknown[]).some(
          (part) =>
            part !== null &&
            typeof part === 'object' &&
            (part as Record<string, unknown>).type === 'output_text' &&
            typeof (part as Record<string, unknown>).text === 'string' &&
            ((part as Record<string, unknown>).text as string).trim() !== ''
        )
    )
  }
  if (protocol === 'messages') {
    const parsed = messagesResponseSchema.safeParse(body)
    if (!parsed.success) return false
    return parsed.data.content.some(
      (block) =>
        block !== null &&
        typeof block === 'object' &&
        (block as Record<string, unknown>).type === 'text' &&
        typeof (block as Record<string, unknown>).text === 'string' &&
        ((block as Record<string, unknown>).text as string).trim() !== ''
    )
  }
  return chatResponseSchema.safeParse(body).success
}

export function invalidResponseKey(protocol: CredentialVerifyProtocol): string {
  if (protocol === 'responses') return 'connect.verify.invalidResponsesResponse'
  if (protocol === 'messages') return 'connect.verify.invalidMessagesResponse'
  return 'connect.verify.invalidChatResponse'
}
