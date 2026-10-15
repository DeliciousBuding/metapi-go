import { describe, expect, it } from 'vitest'

import { routeCheckEndpointFromPath } from '../route-check-endpoints'

describe('routeCheckEndpointFromPath', () => {
  it('maps exact route-check endpoints', () => {
    expect(routeCheckEndpointFromPath('/v1/chat/completions')).toBe(
      '/v1/chat/completions'
    )
    expect(routeCheckEndpointFromPath('/v1/messages/count_tokens')).toBe(
      '/v1/messages/count_tokens'
    )
  })

  it('maps concrete Gemini model paths to the shared check template', () => {
    expect(
      routeCheckEndpointFromPath(
        '/v1beta/models/gemini-2.5-pro:streamGenerateContent?alt=sse'
      )
    ).toBe('/v1beta/models/{model}:generateContent')
  })

  it('returns null for surfaces the route checks cannot evaluate', () => {
    expect(routeCheckEndpointFromPath('/v1/images/generations')).toBeNull()
    expect(routeCheckEndpointFromPath('/anthropic/v1/messages')).toBeNull()
    expect(routeCheckEndpointFromPath(null)).toBeNull()
  })
})
