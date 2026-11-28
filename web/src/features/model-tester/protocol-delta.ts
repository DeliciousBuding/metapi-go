/* eslint-disable no-nested-ternary -- error-shape selection uses chained ternaries */

import type { TestStreamDelta } from './types'

/**
 * Normalize a parsed event payload into a content/reasoning/done delta.
 * Faithful port of the legacy `parseAnyStreamDelta`: handles OpenAI
 * `choices[].delta`, Anthropic `content_block_delta` / `message_stop`,
 * Responses-API `response.output_text.delta` /
 * `response.reasoning_summary_text.delta` / `response.completed`, and
 * Gemini `candidates[].content.parts[]`.
 */
export function parseAnyStreamDelta(eventPayload: unknown): TestStreamDelta {
  if (!eventPayload || typeof eventPayload !== 'object') return {}
  const payload = eventPayload as Record<string, unknown>

  if (payload.type === 'message' && Array.isArray(payload.content)) {
    const blocks = payload.content as Array<Record<string, unknown>>
    return {
      contentDelta: blocks
        .filter((block) => block.type === 'text')
        .map((block) => (typeof block.text === 'string' ? block.text : ''))
        .join(''),
      reasoningDelta: blocks
        .filter((block) => block.type === 'thinking')
        .map((block) =>
          typeof block.thinking === 'string' ? block.thinking : ''
        )
        .join(''),
      done: Boolean(payload.stop_reason),
    }
  }
  if (Array.isArray(payload.output)) {
    const output = payload.output as Array<Record<string, unknown>>
    const content = output
      .filter((item) => item.type === 'message')
      .flatMap((item) =>
        Array.isArray(item.content)
          ? (item.content as Array<Record<string, unknown>>)
          : []
      )
    const summary = output
      .filter((item) => item.type === 'reasoning')
      .flatMap((item) =>
        Array.isArray(item.summary)
          ? (item.summary as Array<Record<string, unknown>>)
          : []
      )
    return {
      contentDelta: content
        .map((part) =>
          typeof part.text === 'string'
            ? part.text
            : typeof part.refusal === 'string'
              ? part.refusal
              : ''
        )
        .join(''),
      reasoningDelta: summary
        .map((part) => (typeof part.text === 'string' ? part.text : ''))
        .join(''),
      done: payload.status === 'completed',
    }
  }

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
