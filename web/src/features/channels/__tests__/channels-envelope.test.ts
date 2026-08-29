// metapi-go/features/channels — envelope parsing regression tests.
//
// GET /api/channels returns `{items,total,page,pageSize}`, not a bare array.
// These tests pin the parser to the real contract: a well-formed envelope
// yields its items, an empty list stays empty, and any malformed body throws
// (so the query fails explicitly instead of rendering an empty table).

import { describe, expect, it } from 'vitest'

import {
  parseChannelsEnvelope,
  parseChannelsErrorSummary,
  parseChannelsPageEnvelope,
} from '../api'

const sampleItem = { id: 1, name: 'channel-1' }

describe('parseChannelsEnvelope', () => {
  it('extracts items from a well-formed envelope', () => {
    const items = [sampleItem]
    const envelope = { items, total: 1, page: 1, pageSize: 50 }
    expect(parseChannelsEnvelope(envelope)).toBe(items)
  })

  it('returns an empty array for an empty items array', () => {
    expect(
      parseChannelsEnvelope({ items: [], total: 0, page: 1, pageSize: 50 })
    ).toEqual([])
  })

  it('throws when the items field is missing', () => {
    expect(() =>
      parseChannelsEnvelope({ total: 0, page: 1, pageSize: 50 })
    ).toThrow('Invalid channels response')
  })

  it('throws when items is not an array', () => {
    expect(() => parseChannelsEnvelope({ items: 'not-an-array' })).toThrow(
      'Invalid channels response'
    )
  })

  it('throws for a null or undefined body', () => {
    expect(() => parseChannelsEnvelope(null)).toThrow(
      'Invalid channels response'
    )
    expect(() => parseChannelsEnvelope(undefined)).toThrow(
      'Invalid channels response'
    )
  })

  it('throws for a primitive body', () => {
    expect(() => parseChannelsEnvelope('channels')).toThrow(
      'Invalid channels response'
    )
    expect(() => parseChannelsEnvelope(42)).toThrow('Invalid channels response')
  })
})

describe('parseChannelsPageEnvelope', () => {
  const sampleItem = { id: 11, name: 'channel-11' }

  it('extracts a page and its authoritative total', () => {
    expect(
      parseChannelsPageEnvelope({ items: [sampleItem], total: 42 })
    ).toEqual({ items: [sampleItem], total: 42 })
  })

  it('falls back to the page item count when total is absent', () => {
    expect(parseChannelsPageEnvelope({ items: [sampleItem] })).toEqual({
      items: [sampleItem],
      total: 1,
    })
  })

  it('rejects malformed page envelopes', () => {
    expect(() => parseChannelsPageEnvelope(null)).toThrow(
      'Invalid channels page response'
    )
    expect(() => parseChannelsPageEnvelope({ items: 'nope' })).toThrow(
      'Invalid channels page response'
    )
    expect(() =>
      parseChannelsPageEnvelope({ items: [], total: 'nope' })
    ).toThrow('Invalid channels page response')
  })
})

describe('parseChannelsErrorSummary', () => {
  it('parses the fleet-wide error summary contract', () => {
    const byStatus = {
      enabled: 10,
      cooldown: 2,
      breaker_open: 1,
      manually_disabled: 3,
    }
    expect(
      parseChannelsErrorSummary({ total: 16, errorCount: 3, byStatus })
    ).toEqual({ total: 16, errorCount: 3, byStatus })
  })

  it('rejects malformed error summaries', () => {
    expect(() => parseChannelsErrorSummary(null)).toThrow(
      'Invalid channels error summary response'
    )
    expect(() =>
      parseChannelsErrorSummary({ total: 1, errorCount: 0 })
    ).toThrow('Invalid channels error summary response')
    expect(() =>
      parseChannelsErrorSummary({ total: '1', errorCount: 0, byStatus: {} })
    ).toThrow('Invalid channels error summary response')
  })
})
