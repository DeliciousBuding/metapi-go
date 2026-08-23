// Unit tests for the attention target parser (the backend
// GET /api/stats/attention target contract -> typed router location).
import { describe, expect, it } from 'vitest'

import { resolveAttentionTarget } from '../attention-target'

describe('resolveAttentionTarget', () => {
  it('maps an expired/low-balance account target to the accounts location', () => {
    expect(resolveAttentionTarget('/accounts?accountId=42')).toEqual({
      to: '/accounts',
      search: { accountId: 42 },
    })
  })

  it('maps a disabled-site target to the sites edit location', () => {
    expect(resolveAttentionTarget('/sites?edit=7')).toEqual({
      to: '/sites',
      search: { edit: 7 },
    })
  })

  it('maps an event target to the settings section path', () => {
    expect(resolveAttentionTarget('/settings/operations/program-logs')).toEqual(
      {
        to: '/settings/$subarea/$section',
        params: { subarea: 'operations', section: 'program-logs' },
      }
    )
  })

  it('rejects malformed targets instead of emitting a dead link', () => {
    const malformed = [
      '',
      '/accounts',
      '/accounts?accountId=0',
      '/accounts?accountId=abc',
      '/accounts?accountId=1.5',
      '/sites?edit=',
      '/sites?edit=-3',
      '/sites',
      '/settings/operations',
      '/settings/operations/program-logs/',
      '/unknown/page',
      'https://example.com/accounts?accountId=1',
    ]
    for (const target of malformed) {
      expect(
        resolveAttentionTarget(target),
        `expected null for target ${JSON.stringify(target)}`
      ).toBeNull()
    }
  })
})
