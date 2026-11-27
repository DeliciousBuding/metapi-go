import { describe, expect, it } from 'vitest'

import { modelTesterSearchSchema } from '@/routes/_authenticated/model-tester'

// ---------------------------------------------------------------------------
// modelTesterSearchSchema (route-level validateSearch) — tolerant deep link
// ---------------------------------------------------------------------------

describe('modelTesterSearchSchema', () => {
  it('accepts a plain model deep-link param', () => {
    expect(modelTesterSearchSchema.parse({ model: 'gpt-5.5' }).model).toBe(
      'gpt-5.5'
    )
  })

  it('returns undefined for an absent model param', () => {
    expect(modelTesterSearchSchema.parse({}).model).toBeUndefined()
  })

  it('tolerates router-parsed primitives without throwing', () => {
    expect(modelTesterSearchSchema.parse({ model: 123 }).model).toBe(123)
    expect(modelTesterSearchSchema.parse({ model: true }).model).toBe(true)
  })

  it('accepts valid targetFormat and channelId deep-link params and ignores invalid ones', () => {
    expect(
      modelTesterSearchSchema.parse({
        model: 'claude-sonnet',
        targetFormat: 'claude',
        channelId: '12',
      })
    ).toEqual({
      model: 'claude-sonnet',
      targetFormat: 'claude',
      channelId: 12,
    })

    expect(
      modelTesterSearchSchema.parse({
        targetFormat: 'bogus',
        channelId: 0,
      })
    ).toEqual({
      model: undefined,
      targetFormat: undefined,
      channelId: undefined,
    })
  })
})
