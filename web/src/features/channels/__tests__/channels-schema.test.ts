import { describe, expect, it } from 'vitest'

import { channelsSearchSchema } from '../lib/channels-schema'

// ---------------------------------------------------------------------------
// channelsSearchSchema — tolerant URL search contract
// ---------------------------------------------------------------------------

describe('channelsSearchSchema', () => {
  it('normalizes a comma-separated sort descriptor to a canonical string', () => {
    expect(channelsSearchSchema.parse({ sort: 'name:desc,url:asc' }).sort).toBe(
      'name:desc,url:asc'
    )
  })

  it('returns undefined for empty sort (no URL noise)', () => {
    expect(channelsSearchSchema.parse({}).sort).toBeUndefined()
    expect(channelsSearchSchema.parse({ sort: '[]' }).sort).toBeUndefined()
  })

  it('tolerates router-parsed primitives without throwing', () => {
    const result = channelsSearchSchema.parse({
      q: 123,
      page: 0,
      pageSize: 'bogus',
      sort: true,
    })
    expect(result.q).toBe(123)
    expect(result.page).toBe(0)
    expect(result.pageSize).toBe(20)
    expect(result.sort).toBeUndefined()
  })

  it('falls back to page 0 / pageSize 20 for an empty input', () => {
    const result = channelsSearchSchema.parse({})
    expect(result.page).toBe(0)
    expect(result.pageSize).toBe(20)
  })
})
