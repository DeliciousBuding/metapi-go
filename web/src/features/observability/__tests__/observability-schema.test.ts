import { describe, expect, it } from 'vitest'

import { observabilitySearchSchema } from '../lib/observability-schema'

// ---------------------------------------------------------------------------
// observabilitySearchSchema — tolerant section param
// ---------------------------------------------------------------------------

describe('observabilitySearchSchema', () => {
  it('accepts each registered section', () => {
    expect(
      observabilitySearchSchema.parse({ section: 'overview' }).section
    ).toBe('overview')
    expect(observabilitySearchSchema.parse({ section: 'health' }).section).toBe(
      'health'
    )
    expect(
      observabilitySearchSchema.parse({ section: 'proxy-logs' }).section
    ).toBe('proxy-logs')
  })

  it('falls back to overview for an absent section', () => {
    expect(observabilitySearchSchema.parse({}).section).toBe('overview')
  })

  it('falls back instead of throwing for unknown / non-string sections', () => {
    expect(observabilitySearchSchema.parse({ section: 'bogus' }).section).toBe(
      'overview'
    )
    expect(observabilitySearchSchema.parse({ section: 123 }).section).toBe(
      'overview'
    )
    expect(observabilitySearchSchema.parse({ section: true }).section).toBe(
      'overview'
    )
  })
})
