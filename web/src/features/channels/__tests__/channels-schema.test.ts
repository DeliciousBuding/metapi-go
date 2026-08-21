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

// ---------------------------------------------------------------------------
// status facet param — comma-separated routing status vocabulary
// ---------------------------------------------------------------------------

describe('channelsSearchSchema — status facet', () => {
  it('accepts the page-written URL shape with a multi-select status list', () => {
    const result = channelsSearchSchema.parse({
      q: 'gpt',
      page: '2',
      pageSize: '50',
      sort: 'name:asc,status:desc',
      status: 'cooldown,breaker_open',
    })
    expect(result.q).toBe('gpt')
    expect(result.page).toBe(2)
    expect(result.pageSize).toBe(50)
    expect(result.sort).toBe('name:asc,status:desc')
    expect(result.status).toBe('cooldown,breaker_open')
  })

  it('round-trips each real routing status value verbatim', () => {
    for (const status of [
      'enabled',
      'cooldown',
      'breaker_open',
      'manually_disabled',
    ]) {
      expect(channelsSearchSchema.parse({ status }).status).toBe(status)
    }
  })

  it('leaves status undefined when the param is absent', () => {
    expect(channelsSearchSchema.parse({}).status).toBeUndefined()
  })
})
