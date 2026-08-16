import { describe, expect, it } from 'vitest'

import { resolveCreatedAccountId } from '../api'

describe('resolveCreatedAccountId', () => {
  it('reads the top-level id returned by single account creation', () => {
    expect(resolveCreatedAccountId({ id: 42 })).toBe(42)
  })

  it('reads the first successful item returned by batch API-key creation', () => {
    expect(
      resolveCreatedAccountId({
        items: [
          { status: 'failed' },
          { status: 'created', id: 73 },
          { status: 'created', id: 74 },
        ],
      })
    ).toBe(73)
  })

  it('does not invent an id when creation returned no usable account', () => {
    expect(
      resolveCreatedAccountId({ items: [{ status: 'failed', id: 12 }] })
    ).toBeUndefined()
  })
})
