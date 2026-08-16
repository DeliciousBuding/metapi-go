import { describe, expect, it } from 'vitest'

import {
  filterEnabledComparisonChannels,
  retainEnabledComparisonChannelIds,
} from '../lib/comparison-channels'

const channels = [
  { id: 1, enabled: true, name: 'primary' },
  { id: 2, enabled: false, name: 'disabled' },
  { id: 3, enabled: true, name: 'secondary' },
]

describe('comparison channel eligibility', () => {
  it('exposes only enabled channels in the comparison selector', () => {
    expect(filterEnabledComparisonChannels(channels)).toEqual([
      channels[0],
      channels[2],
    ])
  })

  it('removes disabled or stale channel ids before probing', () => {
    expect(retainEnabledComparisonChannelIds([1, 2, 99, 3], channels)).toEqual([
      1, 3,
    ])
  })
})
