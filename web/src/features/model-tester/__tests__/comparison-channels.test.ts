import { describe, expect, it } from 'vitest'

import {
  filterEnabledComparisonChannels,
  formatChannelLabel,
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

describe('formatChannelLabel', () => {
  it('disambiguates same-name channels with model and channel id', () => {
    expect(
      formatChannelLabel({
        id: 1,
        name: 'svc-batch-01',
        site: { name: 'NewAPI' },
        models: 'gpt-5.5',
      })
    ).toBe('svc-batch-01 · gpt-5.5 · NewAPI (#1)')
  })

  it('omits the model segment when the channel has no model list', () => {
    expect(
      formatChannelLabel({
        id: 9,
        name: 'svc-oneapi-02',
        site: { name: 'OneAPI' },
        models: '',
      })
    ).toBe('svc-oneapi-02 · OneAPI (#9)')
  })
})
