/* eslint-disable no-nested-ternary -- error-shape selection uses chained ternaries */

import { parseAnyStreamDelta } from './protocol-delta'
import type { TestResponse } from './types'

type ChatTestHarnessEnvelope = {
  success: boolean
  statusCode: number
  latencyMs: number
  firstByteLatencyMs?: number | null
  upstreamReportedModel?: string | null
  basis?: string
  truncatedBody: string
  error: string | null
  bodyTruncated?: boolean
}

type ParsedUpstreamBody = {
  content: string
  reasoningContent: string
  doneReceived: boolean
  rawEvents: string[]
  error?: string
  promptTokens?: number
  completionTokens?: number
  totalTokens?: number
}

export function extractErrorMessage(payload: unknown): string {
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

  const firstByteLatencyMs =
    typeof record.firstByteLatencyMs === 'number' &&
    Number.isFinite(record.firstByteLatencyMs)
      ? record.firstByteLatencyMs
      : record.firstByteLatencyMs === null
        ? null
        : undefined
  const upstreamReportedModel =
    typeof record.upstreamReportedModel === 'string'
      ? record.upstreamReportedModel.trim() || null
      : record.upstreamReportedModel === null
        ? null
        : undefined
  const basis =
    typeof record.basis === 'string' && record.basis.trim().length > 0
      ? record.basis.trim()
      : undefined

  return {
    success: record.success,
    statusCode: record.statusCode,
    latencyMs: record.latencyMs,
    ...(firstByteLatencyMs !== undefined ? { firstByteLatencyMs } : {}),
    ...(upstreamReportedModel !== undefined ? { upstreamReportedModel } : {}),
    ...(basis !== undefined ? { basis } : {}),
    truncatedBody: record.truncatedBody,
    error: record.error as string | null,
    bodyTruncated: record.bodyTruncated === true,
  }
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
    const usage = parseUsage(payload)
    return {
      content: delta.contentDelta ?? '',
      reasoningContent: delta.reasoningDelta ?? '',
      doneReceived: Boolean(delta.done),
      rawEvents: [truncatedBody],
      error: error || undefined,
      ...usage,
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

function toTokenNumber(value: unknown): number | undefined {
  if (typeof value === 'number' && Number.isFinite(value)) return value
  if (typeof value === 'string') {
    const parsed = Number(value)
    if (Number.isFinite(parsed)) return parsed
  }
  return undefined
}

/**
 * Extract token usage from a parsed upstream body. Reads the OpenAI /
 * Responses `usage` object (`prompt_tokens` / `completion_tokens` /
 * `total_tokens`), the Claude `usage` object (`input_tokens` /
 * `output_tokens`, total derived), and the Gemini `usageMetadata` object
 * (`promptTokenCount` / `candidatesTokenCount` / `totalTokenCount`).
 * Returns an empty object when the upstream body carries no usage (errors,
 * non-JSON, or truncated bodies that failed to parse).
 */
function parseUsage(payload: unknown): {
  promptTokens?: number
  completionTokens?: number
  totalTokens?: number
} {
  if (!payload || typeof payload !== 'object') return {}
  const record = payload as Record<string, unknown>

  const usage = record.usage as Record<string, unknown> | undefined
  if (usage && typeof usage === 'object') {
    const prompt = toTokenNumber(usage.prompt_tokens ?? usage.input_tokens)
    const completion = toTokenNumber(
      usage.completion_tokens ?? usage.output_tokens
    )
    const total = toTokenNumber(usage.total_tokens)
    if (
      prompt !== undefined ||
      completion !== undefined ||
      total !== undefined
    ) {
      const promptValue = prompt ?? 0
      const completionValue = completion ?? 0
      return {
        promptTokens: promptValue,
        completionTokens: completionValue,
        totalTokens: total ?? promptValue + completionValue,
      }
    }
  }

  const usageMetadata = record.usageMetadata as
    | Record<string, unknown>
    | undefined
  if (usageMetadata && typeof usageMetadata === 'object') {
    const prompt = toTokenNumber(usageMetadata.promptTokenCount)
    const completion = toTokenNumber(usageMetadata.candidatesTokenCount)
    const total = toTokenNumber(usageMetadata.totalTokenCount)
    if (
      prompt !== undefined ||
      completion !== undefined ||
      total !== undefined
    ) {
      const promptValue = prompt ?? 0
      const completionValue = completion ?? 0
      return {
        promptTokens: promptValue,
        completionTokens: completionValue,
        totalTokens: total ?? promptValue + completionValue,
      }
    }
  }

  return {}
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

export function normalizeProbeResponse(responseText: string): TestResponse {
  const envelope = parseChatTestHarnessEnvelope(responseText)
  const upstreamBody = parseUpstreamBody(envelope.truncatedBody)
  const error = resolveHarnessError(envelope, upstreamBody.error)

  return {
    content: upstreamBody.content,
    reasoningContent: upstreamBody.reasoningContent,
    doneReceived: upstreamBody.doneReceived,
    statusCode: envelope.statusCode,
    latencyMs: envelope.latencyMs,
    ...(envelope.firstByteLatencyMs !== undefined
      ? { firstByteLatencyMs: envelope.firstByteLatencyMs }
      : {}),
    ...(envelope.upstreamReportedModel !== undefined
      ? { upstreamReportedModel: envelope.upstreamReportedModel }
      : {}),
    ...(envelope.basis !== undefined ? { basis: envelope.basis } : {}),
    rawEvents: upstreamBody.rawEvents,
    empty: !upstreamBody.content && !upstreamBody.reasoningContent,
    error,
    promptTokens: upstreamBody.promptTokens,
    completionTokens: upstreamBody.completionTokens,
    totalTokens: upstreamBody.totalTokens,
    bodyTruncated: envelope.bodyTruncated,
  }
}
