import { describe, expect, it } from 'vitest'

import { modelsSearchSchema } from '../lib/models-schema'

// ---------------------------------------------------------------------------
// brand / capability transforms
// ---------------------------------------------------------------------------

describe('modelsSearchSchema — brand transform', () => {
  it('normalizes a comma-separated list (trims + drops empties) to a string', () => {
    expect(modelsSearchSchema.parse({ brand: 'openai, anthropic' }).brand).toBe(
      'openai,anthropic'
    )
  })

  it('drops empty segments and surrounding whitespace', () => {
    expect(modelsSearchSchema.parse({ brand: ' openai ,,' }).brand).toBe(
      'openai'
    )
  })

  it('returns undefined for missing / empty input (no URL noise)', () => {
    expect(modelsSearchSchema.parse({}).brand).toBeUndefined()
    expect(modelsSearchSchema.parse({ brand: '' }).brand).toBeUndefined()
    expect(modelsSearchSchema.parse({ brand: '[]' }).brand).toBeUndefined()
  })
})

describe('modelsSearchSchema — capability transform', () => {
  it('is independent of brand', () => {
    const result = modelsSearchSchema.parse({
      brand: 'openai',
      capability: 'vision, ,code',
    })
    expect(result.brand).toBe('openai')
    expect(result.capability).toBe('vision,code')
  })
})

// ---------------------------------------------------------------------------
// sort transform (shares the proxy-logs shape)
// ---------------------------------------------------------------------------

describe('modelsSearchSchema — sort transform', () => {
  it('normalizes a comma-separated sort descriptor to a canonical string', () => {
    expect(
      modelsSearchSchema.parse({ sort: 'created:desc,model:asc' }).sort
    ).toBe('created:desc,model:asc')
  })

  it('returns undefined when sort is missing / empty (no URL noise)', () => {
    expect(modelsSearchSchema.parse({}).sort).toBeUndefined()
    expect(modelsSearchSchema.parse({ sort: '[]' }).sort).toBeUndefined()
  })
})

// ---------------------------------------------------------------------------
// numerics
// ---------------------------------------------------------------------------

describe('modelsSearchSchema — numerics', () => {
  it('coerces string numerics and accepts page 0', () => {
    const result = modelsSearchSchema.parse({ page: '0', pageSize: '50' })
    expect(result.page).toBe(0)
    expect(result.pageSize).toBe(50)
  })

  it.each([
    ['pageSize below 1', { pageSize: '0' }, { pageSize: 20 }],
    ['pageSize above 200', { pageSize: '201' }, { pageSize: 20 }],
    ['non-numeric page', { page: 'abc' }, { page: 0 }],
  ])('falls back instead of throwing for %s', (_label, input, fallback) => {
    const result = modelsSearchSchema.safeParse(input)
    expect(result.success).toBe(true)
    if (!result.success) return
    expect(result.data).toMatchObject(fallback)
  })

  it('tolerates router-parsed primitives without throwing', () => {
    const result = modelsSearchSchema.parse({
      q: 123,
      page: 0,
      sort: true,
      brand: 42,
      capability: [{ bad: true }],
    })
    expect(result.q).toBe(123)
    expect(result.page).toBe(0)
    expect(result.sort).toBeUndefined()
    expect(result.brand).toBeUndefined()
    expect(result.capability).toBeUndefined()
  })
})
