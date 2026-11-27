export type TestChatRequestPayload = {
  model: string
  messages: Array<{ role: string; content: string }>
  targetFormat?: 'openai' | 'claude' | 'responses' | 'gemini'
  stream?: boolean
  channelId?: number | null
  forcedChannelId?: number | null
  temperature?: number
  top_p?: number
  max_tokens?: number
  frequency_penalty?: number
  presence_penalty?: number
  seed?: number
}
