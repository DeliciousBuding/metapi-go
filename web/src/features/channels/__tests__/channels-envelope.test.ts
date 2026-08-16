// metapi-go/features/channels — envelope parsing regression tests.
//
// GET /api/channels returns `{items,total,page,pageSize}`, not a bare array.
// These tests pin the parser to the real contract: a well-formed envelope
// yields its items, an empty list stays empty, and any malformed body throws
// (so the query fails explicitly instead of rendering an empty table).

import { describe, expect, it } from 'vitest'

import { parseChannelsEnvelope } from '../api'

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
