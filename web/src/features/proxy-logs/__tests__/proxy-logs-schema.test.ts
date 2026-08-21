import { describe, expect, it } from 'vitest'

import {
  PROXY_LOG_STATUS_FILTER_OPTIONS,
  proxyLogsSearchSchema,
} from '../lib/proxy-logs-schema'

// ---------------------------------------------------------------------------
// sort transform
// ---------------------------------------------------------------------------

describe('proxyLogsSearchSchema — sort transform', () => {
  it('normalizes a multi-segment sort descriptor to a canonical string', () => {
    const result = proxyLogsSearchSchema.parse({
      sort: 'created:desc,model:asc',
    })
    expect(result.sort).toBe('created:desc,model:asc')
  })

  it('produces a single-segment string when only the id is present', () => {
    const result = proxyLogsSearchSchema.parse({ sort: 'model' })
    expect(result.sort).toBe('model:asc')
  })

  it('returns undefined when sort is missing or empty (no URL noise)', () => {
    expect(proxyLogsSearchSchema.parse({}).sort).toBeUndefined()
    expect(proxyLogsSearchSchema.parse({ sort: '' }).sort).toBeUndefined()
    expect(proxyLogsSearchSchema.parse({ sort: '[]' }).sort).toBeUndefined()
  })

  it('keeps a bare direction token as an empty id', () => {
    const result = proxyLogsSearchSchema.parse({ sort: ':desc' })
    expect(result.sort).toBe(':desc')
  })
})

// ---------------------------------------------------------------------------
// status enum
// ---------------------------------------------------------------------------

describe('proxyLogsSearchSchema — status enum', () => {
  it('accepts each documented status', () => {
    expect(proxyLogsSearchSchema.parse({ status: 'all' }).status).toBe('all')
    expect(proxyLogsSearchSchema.parse({ status: 'success' }).status).toBe(
      'success'
    )
    expect(proxyLogsSearchSchema.parse({ status: 'failed' }).status).toBe(
      'failed'
    )
  })

  it('falls back to "all" for an unknown or non-string status', () => {
    expect(proxyLogsSearchSchema.parse({ status: 'partial' }).status).toBe(
      'all'
    )
    expect(proxyLogsSearchSchema.parse({ status: true }).status).toBe('all')
  })
})

// ---------------------------------------------------------------------------
// numeric coercion + bounds
// ---------------------------------------------------------------------------

describe('proxyLogsSearchSchema — numerics', () => {
  it('coerces string numerics', () => {
    const result = proxyLogsSearchSchema.parse({
      page: '5',
      pageSize: '50',
      siteId: '7',
      channelId: '42',
      latencyMin: '10',
      latencyMax: '100',
    })
    expect(result.page).toBe(5)
    expect(result.pageSize).toBe(50)
    expect(result.siteId).toBe(7)
    expect(result.channelId).toBe(42)
    expect(result.latencyMin).toBe(10)
    expect(result.latencyMax).toBe(100)
  })

  it('accepts page 0 (min 0, unlike the checkin schema)', () => {
    expect(proxyLogsSearchSchema.parse({ page: '0' }).page).toBe(0)
  })

  it.each([
    ['pageSize below 1', { pageSize: '0' }, { pageSize: 20 }],
    ['pageSize above 200', { pageSize: '201' }, { pageSize: 20 }],
    ['latencyMin negative', { latencyMin: '-1' }, { latencyMin: undefined }],
    ['non-numeric page', { page: 'abc' }, { page: 0 }],
    // channelId is positive-only: the backend reads 0 as "no channel filter",
    // so a non-positive value must degrade to unset rather than be sent.
    ['channelId zero', { channelId: '0' }, { channelId: undefined }],
    ['channelId negative', { channelId: '-3' }, { channelId: undefined }],
    ['channelId non-numeric', { channelId: 'abc' }, { channelId: undefined }],
  ])('falls back instead of throwing for %s', (_label, input, fallback) => {
    const result = proxyLogsSearchSchema.safeParse(input)
    expect(result.success).toBe(true)
    if (!result.success) return
    expect(result.data).toMatchObject(fallback)
  })

  it('tolerates router-parsed primitives without throwing', () => {
    const result = proxyLogsSearchSchema.parse({
      q: 123,
      page: 0,
      pageSize: 'bogus',
      status: 'bogus',
      sort: 123,
      latencyMin: 'abc',
    })
    expect(result.q).toBe(123)
    expect(result.page).toBe(0)
    expect(result.pageSize).toBe(20)
    expect(result.status).toBe('all')
    expect(result.sort).toBeUndefined()
    expect(result.latencyMin).toBeUndefined()
  })

  it('tolerates a malformed sort array without throwing', () => {
    const result = proxyLogsSearchSchema.parse({
      sort: [{ id: 7, desc: 'yes' }],
    })
    expect(result.sort).toBeUndefined()
  })
})

// ---------------------------------------------------------------------------
// PROXY_LOG_STATUS_FILTER_OPTIONS
// ---------------------------------------------------------------------------

describe('PROXY_LOG_STATUS_FILTER_OPTIONS', () => {
  it('exposes all / success / failed in that order', () => {
    expect(PROXY_LOG_STATUS_FILTER_OPTIONS).toHaveLength(3)
    expect(PROXY_LOG_STATUS_FILTER_OPTIONS.map((o) => o.value)).toEqual([
      'all',
      'success',
      'failed',
    ])
  })
})
