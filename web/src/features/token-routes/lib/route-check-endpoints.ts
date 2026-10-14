/**
 * Client protocol surface shared by route checks. A route workspace and a
 * route detail check must offer the same endpoints; a second list would let
 * one surface claim a protocol the other never evaluates.
 */
export const routeCheckEndpoints = [
  ['/v1/chat/completions', 'OpenAI Chat'],
  ['/v1/responses', 'OpenAI Responses'],
  ['/v1/messages', 'Anthropic Messages'],
  ['/v1beta/models/{model}:generateContent', 'Gemini generateContent'],
  ['/v1/messages/count_tokens', 'Anthropic count_tokens'],
  ['/v1/embeddings', 'OpenAI Embeddings'],
] as const
