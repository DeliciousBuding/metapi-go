// metapi-go/components/common — probe history envelope parsing (P0-2).
// The parser turns the backend `{limit, items}` envelope into the row-id map
// the table cells consume; a malformed body must throw (fail the query
// explicitly) instead of masquerading as an empty history.

import { describe, expect, it } from 'vitest'

import { parseProbeHistoryEnvelope } from '../use-probe-history'

const VALID_ENVELOPE = {
  limit: 20,
  items: [
    {
      channelId: 12,
      results: [
        {
          id: 401,
          status: 'success',
          latencyMs: 842.5,
          httpStatus: 200,
          errorText: null,
          modelName: 'gpt-4o',
          createdAt: '2026-08-28T02:00:00Z',
        },
      ],
    },
    { channelId: 13, results: [] },
  ],
}

describe('parseProbeHistoryEnvelope', () => {
  it('maps channel groups by id', () => {
    const map = parseProbeHistoryEnvelope(VALID_ENVELOPE, 'channelId')
    expect(Object.keys(map)).toEqual(['12', '13'])
    expect(map[12]?.[0]?.id).toBe(401)
    expect(map[12]?.[0]?.status).toBe('success')
    expect(map[12]?.[0]?.latencyMs).toBe(842.5)
    expect(map[13]).toEqual([])
  })

  it('maps account groups by accountId', () => {
    const map = parseProbeHistoryEnvelope(
      {
        limit: 20,
        items: [
          {
            accountId: 5,
            results: [
              {
                id: 402,
                status: 'failure',
                latencyMs: null,
                httpStatus: 401,
                errorText: 'unauthorized',
                modelName: 'gpt-4o',
                createdAt: '2026-08-28T02:00:00Z',
              },
            ],
          },
        ],
      },
      'accountId'
    )
    expect(map[5]?.[0]?.status).toBe('failure')
  })

  it.each([
    ['null', null],
    ['a non-object', 'probe-history'],
    ['an envelope without items', { limit: 20 }],
    ['an envelope with non-array items', { limit: 20, items: {} }],
  ])('throws on %s body', (_label, body) => {
    expect(() => parseProbeHistoryEnvelope(body, 'channelId')).toThrow(
      'Invalid probe history response'
    )
  })

  it('skips malformed group entries instead of crashing the table', () => {
    const map = parseProbeHistoryEnvelope(
      {
        limit: 20,
        items: [
          null,
          { channelId: 'not-a-number', results: [] },
          { channelId: 7 },
          { channelId: 8, results: [{ id: 403, status: 'success' }] },
        ],
      },
      'channelId'
    )
    expect(Object.keys(map)).toEqual(['8'])
  })
})
