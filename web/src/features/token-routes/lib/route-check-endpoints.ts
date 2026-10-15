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

/**
 * Map a recorded downstream path to the endpoint contract the route checks
 * can evaluate. Gemini's concrete model path collapses to the shared
 * template; unknown or non-checkable surfaces return null so callers do not
 * silently diagnose a different protocol.
 */
export function routeCheckEndpointFromPath(
  path: string | null | undefined
): string | null {
  const normalized = (path ?? '').trim().split('?')[0].replace(/\/+$/, '')
  const exact = routeCheckEndpoints.find(
    ([endpoint]) => endpoint === normalized
  )
  if (exact) return exact[0]
  if (
    /^\/v1beta\/models\/[^/]+:(?:generateContent|streamGenerateContent)$/i.test(
      normalized
    )
  ) {
    return '/v1beta/models/{model}:generateContent'
  }
  return null
}
