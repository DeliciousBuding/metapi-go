export type ModelTesterTargetFormat =
  | 'openai'
  | 'claude'
  | 'responses'
  | 'gemini'

function targetFormatFromEndpoint(
  endpoint: string | null | undefined
): ModelTesterTargetFormat | null {
  switch (endpoint) {
    case '/v1/chat/completions':
      return 'openai'
    case '/v1/responses':
      return 'responses'
    case '/v1/messages':
    case '/v1/messages/count_tokens':
      return 'claude'
    case '/v1beta/models/{model}:generateContent':
      return 'gemini'
    default:
      return null
  }
}

export function buildModelTesterSearch(params: {
  model?: string | null
  endpoint?: string | null
  channelId?: number | null
}): {
  model: string
  targetFormat?: ModelTesterTargetFormat
  channelId?: number
} | null {
  const model = params.model?.trim()
  const targetFormat = targetFormatFromEndpoint(params.endpoint)
  if (!model || !targetFormat) return null
  const channelId =
    typeof params.channelId === 'number' &&
    Number.isInteger(params.channelId) &&
    params.channelId > 0
      ? params.channelId
      : undefined
  return {
    model,
    targetFormat,
    ...(channelId !== undefined ? { channelId } : {}),
  }
}
