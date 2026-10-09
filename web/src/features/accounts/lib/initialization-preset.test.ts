import { describe, expect, it } from 'vitest'

import type { SiteInitializationPreset } from '@/lib/api/sites'

import { findAccountInitializationPreset } from './initialization-preset'

const preset: SiteInitializationPreset = {
  id: 'fixture-coding',
  label: 'Fixture Coding',
  providerLabel: 'Fixture',
  platform: 'openai',
  defaultUrl: 'https://coding.example.invalid/api/coding/',
  recommendedSkipModelFetch: true,
  recommendedModels: ['coding-model'],
  docsUrl: '',
}

describe('account initialization preset matching', () => {
  it('matches platform plus normalized URL without copying a provider catalog', () => {
    expect(
      findAccountInitializationPreset(
        {
          platform: ' OPENAI ',
          url: 'https://CODING.EXAMPLE.INVALID:443/api/coding/?a=1#fragment',
        },
        [preset]
      )
    ).toBe(preset)
    const root = { ...preset, defaultUrl: 'https://standard.example.invalid' }
    expect(
      findAccountInitializationPreset(
        { platform: 'openai', url: 'https://standard.example.invalid/v1/' },
        [root]
      )
    ).toBe(root)
  })
  it.each([
    { platform: 'claude', url: preset.defaultUrl },
    { platform: 'openai', url: 'https://coding.example.invalid/api/other' },
    {
      platform: 'openai',
      url: 'https://coding.example.invalid.evil/api/coding',
    },
    { platform: 'openai', url: 'http://coding.example.invalid/api/coding' },
  ])(
    'does not apply a preset across distinct connection identities: %j',
    (site) => {
      expect(findAccountInitializationPreset(site, [preset])).toBeUndefined()
    }
  )
  it('leaves missing presets and site data without a recommendation', () => {
    expect(findAccountInitializationPreset(undefined, [preset])).toBeUndefined()
    expect(
      findAccountInitializationPreset(
        { platform: 'openai', url: preset.defaultUrl },
        undefined
      )
    ).toBeUndefined()
    expect(
      findAccountInitializationPreset({ platform: 'openai', url: '' }, [
        { ...preset, defaultUrl: '' },
      ])
    ).toBeUndefined()
  })
})
