// @vitest-environment node
import { describe, expect, it } from 'vitest'

import { connectionSchema, upstreamProtocols } from '../lib/upstream-config'

describe('Decisions endpoint and authorization metadata', () => {
  it('uses an independent native JSON bit rather than a generation capability', () => {
    expect(
      upstreamProtocols.find((protocol) => protocol.key === 'decisions')
    ).toEqual({
      key: 'decisions',
      bit: 67108864,
      group: 'native',
      convertible: false,
      authModes: ['bearer'],
    })
    expect(
      connectionSchema.shape.endpointConfig.safeParse({
        decisions: {
          url: 'https://example.invalid/custom/decide',
          auth: 'bearer',
        },
      }).success
    ).toBe(true)
  })

  it('rejects transport, authentication and generation-profile mismatches', () => {
    for (const endpoint of [
      { url: 'wss://example.invalid/decide', auth: 'bearer' },
      { url: 'https://example.invalid/decide', auth: 'x-api-key' },
      { url: 'https://example.invalid/decide', auth: 'none' },
      {
        url: 'https://example.invalid/decide',
        auth: 'bearer',
        profile: 'codex',
      },
      {
        url: 'https://example.invalid/decide',
        auth: 'bearer',
        modelPath: true,
      },
    ]) {
      expect(
        connectionSchema.shape.endpointConfig.safeParse({ decisions: endpoint })
          .success
      ).toBe(false)
    }
  })
})
