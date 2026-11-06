// @vitest-environment node
import { describe, expect, it } from 'vitest'

import { connectionSchema } from '../lib/upstream-config'

describe('Responses transport permissions', () => {
  it('keeps WebSocket and compaction endpoint schemes separate', () => {
    const schema = connectionSchema.shape.endpointConfig
    expect(
      schema.safeParse({
        responsesCompact: {
          url: 'https://provider.example/compact',
          auth: 'bearer',
        },
        responsesWebsocket: {
          url: 'wss://provider.example/responses',
          auth: 'bearer',
        },
      }).success
    ).toBe(true)
    for (const [key, url] of [
      ['responses', 'wss://provider.example/responses'],
      ['responsesCompact', 'wss://provider.example/compact'],
      ['responsesWebsocket', 'https://provider.example/responses'],
    ]) {
      expect(schema.safeParse({ [key]: { url, auth: 'bearer' } }).success).toBe(
        false
      )
    }
    expect(
      schema.safeParse({
        responsesWebsocket: {
          url: 'wss://provider.example/responses',
          auth: 'none',
        },
      }).success
    ).toBe(false)
  })
})
