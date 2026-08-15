import { describe, expect, it } from 'vitest'

import { modelTesterSearchSchema } from '@/routes/_authenticated/model-tester'

// ---------------------------------------------------------------------------
// modelTesterSearchSchema (route-level validateSearch) — tolerant deep link
// ---------------------------------------------------------------------------

describe('modelTesterSearchSchema', () => {
  it('accepts a plain model deep-link param', () => {
    expect(modelTesterSearchSchema.parse({ model: 'gpt-4o' }).model).toBe(
      'gpt-4o'
    )
  })

  it('returns undefined for an absent model param', () => {
    expect(modelTesterSearchSchema.parse({}).model).toBeUndefined()
  })

  it('tolerates router-parsed primitives without throwing', () => {
    expect(modelTesterSearchSchema.parse({ model: 123 }).model).toBe(123)
    expect(modelTesterSearchSchema.parse({ model: true }).model).toBe(true)
  })
})
