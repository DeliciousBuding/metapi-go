import { describe, expect, it } from 'vitest'

import {
  TESTER_FORM_DEFAULT_VALUES,
  testerSchema,
  type TesterFormValues,
} from '../lib/tester-schema'

function validOverrides(): Partial<TesterFormValues> {
  return {
    model: 'gpt-4o',
    prompt: 'hello',
    targetFormat: 'claude',
    temperature: 0.7,
    topP: 1,
    maxTokens: 1024,
    stream: false,
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
    const result = testerSchema.parse({ ...validInput(), model: '  gpt-4o  ' })
    expect(result.model).toBe('gpt-4o')
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
  it('rejects an empty model with the modelRequired key', () => {
    const result = testerSchema.safeParse({ ...validInput(), model: '' })
    expect(result.success).toBe(false)
    if (result.success) return
    expect(result.error.issues[0]?.message).toBe(
      'modelTester.form.errors.modelRequired'
    )
  })

  it('rejects a whitespace-only model after trimming', () => {
    const result = testerSchema.safeParse({ ...validInput(), model: '   ' })
    expect(result.success).toBe(false)
  })

  it('rejects an empty prompt with the promptRequired key', () => {
    const result = testerSchema.safeParse({ ...validInput(), prompt: '' })
    expect(result.success).toBe(false)
    if (result.success) return
    expect(result.error.issues[0]?.message).toBe(
      'modelTester.form.errors.promptRequired'
    )
  })
})

// ---------------------------------------------------------------------------
// length bounds
// ---------------------------------------------------------------------------

describe('testerSchema — length bounds', () => {
  it('rejects a systemPrompt over 4000 chars', () => {
    const result = testerSchema.safeParse({
      ...validInput(),
      systemPrompt: 'x'.repeat(4001),
    })
    expect(result.success).toBe(false)
    if (result.success) return
    expect(result.error.issues[0]?.message).toBe(
      'modelTester.form.errors.systemTooLong'
    )
  })

  it('rejects a prompt over 16000 chars', () => {
    const result = testerSchema.safeParse({
      ...validInput(),
      prompt: 'x'.repeat(16001),
    })
    expect(result.success).toBe(false)
    if (result.success) return
    expect(result.error.issues[0]?.message).toBe(
      'modelTester.form.errors.promptTooLong'
    )
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
  it('rejects temperature above 2 with the temperatureMax key', () => {
    const result = testerSchema.safeParse({ ...validInput(), temperature: 2.5 })
    expect(result.success).toBe(false)
    if (result.success) return
    expect(result.error.issues[0]?.message).toBe(
      'modelTester.form.errors.temperatureMax'
    )
  })

  it('rejects temperature below 0 with the temperatureMin key', () => {
    const result = testerSchema.safeParse({
      ...validInput(),
      temperature: -0.1,
    })
    expect(result.success).toBe(false)
    if (result.success) return
    expect(result.error.issues[0]?.message).toBe(
      'modelTester.form.errors.temperatureMin'
    )
  })

  it('rejects topP above 1 with the topPMax key', () => {
    const result = testerSchema.safeParse({ ...validInput(), topP: 1.1 })
    expect(result.success).toBe(false)
    if (result.success) return
    expect(result.error.issues[0]?.message).toBe(
      'modelTester.form.errors.topPMax'
    )
  })

  it('rejects maxTokens above 128000 with the maxTokensMax key', () => {
    const result = testerSchema.safeParse({
      ...validInput(),
      maxTokens: 128001,
    })
    expect(result.success).toBe(false)
    if (result.success) return
    expect(result.error.issues[0]?.message).toBe(
      'modelTester.form.errors.maxTokensMax'
    )
  })

  it('rejects a non-integer maxTokens with the maxTokensInteger key', () => {
    const result = testerSchema.safeParse({ ...validInput(), maxTokens: 1.5 })
    expect(result.success).toBe(false)
    if (result.success) return
    expect(result.error.issues[0]?.message).toBe(
      'modelTester.form.errors.maxTokensInteger'
    )
  })

  it('rejects a negative maxTokens with the maxTokensMin key', () => {
    const result = testerSchema.safeParse({ ...validInput(), maxTokens: -1 })
    expect(result.success).toBe(false)
    if (result.success) return
    expect(result.error.issues[0]?.message).toBe(
      'modelTester.form.errors.maxTokensMin'
    )
  })
})

// ---------------------------------------------------------------------------
// no coercion / enum
// ---------------------------------------------------------------------------

describe('testerSchema — no coercion + enum', () => {
  it('rejects a string temperature (no coerce)', () => {
    const result = testerSchema.safeParse({
      ...validInput(),
      temperature: '0.7' as unknown as number,
    })
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

  it('rejects a string stream flag (no coerce)', () => {
    const result = testerSchema.safeParse({
      ...validInput(),
      stream: 'true' as unknown as boolean,
    })
    expect(result.success).toBe(false)
  })
})

// ---------------------------------------------------------------------------
// defaults
// ---------------------------------------------------------------------------

describe('TESTER_FORM_DEFAULT_VALUES', () => {
  it('exposes the canonical default form shape', () => {
    expect(TESTER_FORM_DEFAULT_VALUES).toEqual({
      model: '',
      systemPrompt: '',
      prompt: '',
      targetFormat: 'openai',
      temperature: 0.7,
      topP: 1,
      maxTokens: 1024,
      stream: false,
    })
  })

  it('fails schema validation (model + prompt empty) — defaults are a seed, not a valid submit', () => {
    expect(testerSchema.safeParse(TESTER_FORM_DEFAULT_VALUES).success).toBe(
      false
    )
  })
})
