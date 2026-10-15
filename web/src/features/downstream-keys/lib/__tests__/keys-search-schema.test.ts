import { describe, expect, it } from 'vitest'

import { downstreamKeysSearchSchema } from '../keys-search-schema'

describe('downstreamKeysSearchSchema', () => {
  it('accepts the diagnostic edit deep link', () => {
    expect(
      downstreamKeysSearchSchema.parse({
        edit: '7',
        locateField: 'accessPolicy',
      })
    ).toEqual({ edit: 7, locateField: 'accessPolicy' })
  })

  it('degrades invalid one-shot values instead of rejecting the route', () => {
    expect(downstreamKeysSearchSchema.parse({ edit: '0' })).toEqual({})
    expect(downstreamKeysSearchSchema.parse({ edit: null })).toEqual({})
  })
})
