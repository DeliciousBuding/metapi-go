import { describe, expect, it } from 'vitest'

import { resolveCreatedRouteId } from '../api'

describe('resolveCreatedRouteId', () => {
  it('reads the top-level id returned by route creation', () => {
    expect(resolveCreatedRouteId({ id: 91 })).toBe(91)
  })

  it('rejects missing and invalid route ids', () => {
    expect(resolveCreatedRouteId(undefined)).toBeUndefined()
    expect(resolveCreatedRouteId({ id: 0 })).toBeUndefined()
  })
})
