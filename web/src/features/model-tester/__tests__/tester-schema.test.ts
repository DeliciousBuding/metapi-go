import { describe, expect, it } from 'vitest'

import {
  TESTER_FORM_DEFAULT_VALUES,
  testerSchema,
  type TesterFormValues,
} from '../lib/tester-schema'

function validOverrides(): Partial<TesterFormValues> {
  return {
    model: 'gpt-5.5',
    prompt: 'hello',
    targetFormat: 'claude',
    temperature: 0.7,
    topP: 1,
    maxTokens: 1024,
  }
}

function validInput(): TesterFormValues {
  return { ...TESTER_FORM_DEFAULT_VALUES, ...validOverrides() }
}

// ---------------------------------------------------------------------------
// happy path
// ---------------------------------------------------------------------------

describe('testerSchema — happy path', () => {
  it('parses a fully valid form', () => {
    const result = testerSchema.safeParse(validInput())
    expect(result.success).toBe(true)
  })

  it('trims and accepts model with surrounding whitespace', () => {
    const result = testerSchema.parse({ ...validInput(), model: '  gpt-5.5  ' })
    expect(result.model).toBe('gpt-5.5')
  })

  it('accepts an empty systemPrompt (no min constraint)', () => {
    const result = testerSchema.safeParse({ ...validInput(), systemPrompt: '' })
    expect(result.success).toBe(true)
  })
})

// ---------------------------------------------------------------------------
// required fields
// ---------------------------------------------------------------------------

describe('testerSchema — required fields', () => {
  const requiredCases: Array<[string, Partial<TesterFormValues>, string | undefined]> = [
    ['an empty model', { model: '' }, 'modelTester.form.errors.modelRequired'],
    ['a whitespace-only model', { model: '   ' }, undefined],
    ['an empty prompt', { prompt: '' }, 'modelTester.form.errors.promptRequired'],
  ]

  it.each(requiredCases)(
    'rejects %s with a required-field error',
    (_label, overrides, message) => {
      const result = testerSchema.safeParse({ ...validInput(), ...overrides })
      expect(result.success).toBe(false)
      if (result.success) return
      if (message) expect(result.error.issues[0]?.message).toBe(message)
    }
  )
})

// ---------------------------------------------------------------------------
// length bounds
// ---------------------------------------------------------------------------

describe('testerSchema — length bounds', () => {
  const tooLongCases: Array<[string, Partial<TesterFormValues>, string]> = [
    ['a systemPrompt over 4000 chars', { systemPrompt: 'x'.repeat(4001) }, 'modelTester.form.errors.systemTooLong'],
    ['a prompt over 16000 chars', { prompt: 'x'.repeat(16001) }, 'modelTester.form.errors.promptTooLong'],
  ]

  it.each(tooLongCases)('rejects %s', (_label, overrides, message) => {
    const result = testerSchema.safeParse({ ...validInput(), ...overrides })
    expect(result.success).toBe(false)
    if (result.success) return
    expect(result.error.issues[0]?.message).toBe(message)
  })

  it('accepts exactly 4000-char systemPrompt and 16000-char prompt', () => {
    expect(
      testerSchema.safeParse({
        ...validInput(),
        systemPrompt: 'x'.repeat(4000),
      }).success
    ).toBe(true)
    expect(
      testerSchema.safeParse({ ...validInput(), prompt: 'x'.repeat(16000) })
        .success
    ).toBe(true)
  })
})

// ---------------------------------------------------------------------------
// numeric bounds
// ---------------------------------------------------------------------------

describe('testerSchema — numeric bounds', () => {
  const numericBoundCases: Array<[string, Partial<TesterFormValues>, string]> = [
    ['temperature above 2', { temperature: 2.5 }, 'modelTester.form.errors.temperatureMax'],
    ['temperature below 0', { temperature: -0.1 }, 'modelTester.form.errors.temperatureMin'],
    ['topP above 1', { topP: 1.1 }, 'modelTester.form.errors.topPMax'],
    ['maxTokens above 128000', { maxTokens: 128001 }, 'modelTester.form.errors.maxTokensMax'],
    ['a non-integer maxTokens', { maxTokens: 1.5 }, 'modelTester.form.errors.maxTokensInteger'],
    ['a negative maxTokens', { maxTokens: -1 }, 'modelTester.form.errors.maxTokensMin'],
  ]

  it.each(numericBoundCases)('rejects %s', (_label, overrides, message) => {
    const result = testerSchema.safeParse({ ...validInput(), ...overrides })
    expect(result.success).toBe(false)
    if (result.success) return
    expect(result.error.issues[0]?.message).toBe(message)
  })
})

// ---------------------------------------------------------------------------
// no coercion / enum
// ---------------------------------------------------------------------------

describe('testerSchema — no coercion + enum', () => {
  const noCoerceCases: Array<[string, Partial<TesterFormValues>]> = [
    ['a string temperature (no coerce)', { temperature: '0.7' as unknown as number }],
    ['a non-integer channelId (no coerce)', { channelId: '42' as unknown as number }],
  ]

  it.each(noCoerceCases)('rejects %s', (_label, overrides) => {
    const result = testerSchema.safeParse({ ...validInput(), ...overrides })
    expect(result.success).toBe(false)
  })

  it('rejects an unknown targetFormat with an enum error', () => {
    const result = testerSchema.safeParse({
      ...validInput(),
      targetFormat: 'unknown' as unknown as TesterFormValues['targetFormat'],
    })
    expect(result.success).toBe(false)
    if (result.success) return
    expect(result.error.issues[0]?.message).toContain('Invalid option')
  })

  it('accepts an omitted channelId (optional channel targeting)', () => {
    const result = testerSchema.safeParse({
      ...validInput(),
      channelId: undefined,
    })
    expect(result.success).toBe(true)
  })
})

// ---------------------------------------------------------------------------
// channel comparison
// ---------------------------------------------------------------------------

describe('testerSchema — channel comparison', () => {
  it('accepts compare mode with at least two channels', () => {
    const result = testerSchema.safeParse({
      ...validInput(),
      compareChannels: true,
      channelIds: [1, 2],
    })
    expect(result.success).toBe(true)
  })

  it('rejects compare mode with fewer than two channels', () => {
    const result = testerSchema.safeParse({
      ...validInput(),
      compareChannels: true,
      channelIds: [1],
    })
    expect(result.success).toBe(false)
    if (result.success) return
    expect(result.error.issues[0]?.message).toBe(
      'modelTester.form.errors.compareMinChannels'
    )
  })

  it('accepts empty channelIds when compare mode is off', () => {
    const result = testerSchema.safeParse({
      ...validInput(),
      compareChannels: false,
      channelIds: [],
    })
    expect(result.success).toBe(true)
  })
})

// ---------------------------------------------------------------------------
// defaults
// ---------------------------------------------------------------------------

describe('TESTER_FORM_DEFAULT_VALUES', () => {
  it('exposes the canonical default form shape', () => {
    expect(TESTER_FORM_DEFAULT_VALUES).toEqual({
      model: '',
      channelId: undefined,
      compareChannels: false,
      channelIds: [],
      systemPrompt: '',
      prompt: '',
      targetFormat: 'openai',
      temperature: 0.7,
      topP: 1,
      maxTokens: 1024,
    })
  })

  it('fails schema validation (model + prompt empty) — defaults are a seed, not a valid submit', () => {
    expect(testerSchema.safeParse(TESTER_FORM_DEFAULT_VALUES).success).toBe(
      false
    )
  })
})
